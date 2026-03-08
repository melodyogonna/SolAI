package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/melodyogonna/solai/solai-agent/capability"
	solaiconfig "github.com/melodyogonna/solai/solai-agent/config"
	"github.com/tmc/langchaingo/tools"
)

// LoadTools walks toolsDir and discovers all agentic tools.
//
// A tool is discovered when a subdirectory contains a valid manifest.json
// whose declared Executable exists on disk.
//
// If a tool declares llm_options, provider is consulted to resolve which model
// and credentials to inject. Tools with no matching configured provider are
// skipped with a warning rather than causing a fatal error.
//
// If a tool declares required_capabilities, checker is consulted for each entry.
// Missing Regular capabilities cause the tool to be disabled with a warning.
//
// bwrapPath is the path to the extracted bwrap binary (empty when sandbox is
// unavailable). A SandboxPolicy is built for each tool from its declared
// capabilities and injected so calls run inside bubblewrap.
//
// Returns:
//   - []tools.Tool: successfully loaded tools (may be empty if none found)
//   - []error: one warning per tool that failed to load or was disabled
//   - error: fatal only if toolsDir itself cannot be read
func LoadTools(toolsDir string, provider *capability.LLMProvider, capManager *capability.CapabilityManager, bwrapPath string, cfg *solaiconfig.SolaiConfig) ([]tools.Tool, []error, error) {
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return nil, nil, fmt.Errorf("reading tools directory %s: %w", toolsDir, err)
	}

	// Extract the communication capability for IPC directory management.
	var commCap *capability.CommunicationCapability
	if c := capManager.GetByName("communication"); c != nil {
		commCap, _ = c.(*capability.CommunicationCapability)
	}
	if commCap == nil {
		// Fallback: create a standalone instance when running outside the normal
		// capability setup (e.g. in tests or legacy env-var path).
		commCap = capability.NewCommunicationCapability()
	}

	// Build the capability section once; it's the same for all tools in this agent.
	capabilitySection := capManager.BuildToolRequestSection()

	var loaded []tools.Tool
	var warnings []error

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		toolDir := filepath.Join(toolsDir, entry.Name())
		manifestPath := filepath.Join(toolDir, "manifest.json")

		manifest, err := LoadManifest(manifestPath)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("tool %q: %w", entry.Name(), err))
			continue
		}

		if err := manifest.Validate(); err != nil {
			warnings = append(warnings, fmt.Errorf("tool %q: %w", entry.Name(), err))
			continue
		}

		execPath := filepath.Join(toolDir, manifest.Executable)
		if _, err := os.Stat(execPath); err != nil {
			warnings = append(warnings, fmt.Errorf("tool %q: executable not found at %s: %w",
				manifest.Name, execPath, err))
			continue
		}

		var llmCfg *capability.LLMConfig
		if opts := manifest.LLMOptions; opts != nil {
			supported := make([]capability.LLMModelOption, len(opts.Supported))
			for i, m := range opts.Supported {
				supported[i] = capability.LLMModelOption{Model: m.Model, Provider: m.Provider}
			}
			llmCfg = provider.ResolveForTool(opts.Primary, supported)
			if llmCfg == nil {
				warnings = append(warnings, fmt.Errorf(
					"tool %q disabled: no supported LLM provider configured (needs one of: %s)",
					manifest.Name, joinProviders(opts.Supported)))
				continue
			}
		}

		// Validate required capabilities and build the sandbox policy.
		policy, capWarning := buildSandboxPolicy(manifest, capManager, bwrapPath)
		if capWarning != nil {
			warnings = append(warnings, fmt.Errorf("tool %q disabled: %w", manifest.Name, capWarning))
			continue
		}

		// Resolve and validate declared env vars.
		toolEnv, envWarning := resolveToolEnv(manifest, cfg)
		if envWarning != nil {
			warnings = append(warnings, fmt.Errorf("tool %q disabled: %w", manifest.Name, envWarning))
			continue
		}

		autoPayloads := resolveAutoPayloads(manifest, capManager)
		loaded = append(loaded, NewAgenticTool(manifest, toolDir, llmCfg, policy, toolEnv, commCap, autoPayloads, capabilitySection))
	}

	return loaded, warnings, nil
}

// resolveAutoPayloads resolves all manifest payload entries that have a Source
// field by calling the named capability's Execute method. Returns nil if no
// auto-payloads are declared or none can be resolved.
func resolveAutoPayloads(manifest Manifest, capManager *capability.CapabilityManager) map[string]string {
	result := make(map[string]string)
	for _, p := range manifest.Payloads {
		if p.Source == "" {
			continue
		}
		cap := capManager.GetByName(p.Source)
		if cap == nil {
			continue
		}
		if val, err := cap.Execute(context.Background(), ""); err == nil && val != "" {
			result[p.Name] = val
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// buildSandboxPolicy resolves the SandboxPolicy for a tool from its manifest.
// Returns (policy, nil) on success, or (zero, error) if a required capability
// is declared but not registered — the tool should be disabled in that case.
func buildSandboxPolicy(manifest Manifest, checker capability.CapabilityChecker, bwrapPath string) (SandboxPolicy, error) {
	policy := SandboxPolicy{BwrapPath: bwrapPath}

	for _, name := range manifest.RequiredCapabilities {
		if !checker.IsRegularCapabilityAvailable(name) {
			return SandboxPolicy{}, fmt.Errorf(
				"required capability %q is not registered (add it to the agent's capability list)", name)
		}
		switch name {
		case "network-manager":
			policy.ShareNet = true
		// "file-manager" support is reserved for future implementation.
		}
	}

	return policy, nil
}

// resolveToolEnv builds the "KEY=VALUE" env slice for a tool from the agent config.
// Returns an error if any required env var has no configured value.
func resolveToolEnv(manifest Manifest, cfg *solaiconfig.SolaiConfig) ([]string, error) {
	if len(manifest.Env) == 0 {
		return nil, nil
	}
	var configured map[string]string
	if cfg != nil && cfg.ToolEnv != nil {
		configured = cfg.ToolEnv[manifest.Name]
	}
	var env []string
	for _, decl := range manifest.Env {
		val := configured[decl.Name]
		if val == "" && decl.Required {
			return nil, fmt.Errorf(
				"required env var %q is not set (run: solai config set tool-env.%s.%s <value>)",
				decl.Name, manifest.Name, decl.Name)
		}
		if val != "" {
			env = append(env, decl.Name+"="+val)
		}
	}
	return env, nil
}

// joinProviders formats a ManifestLLMModel slice as a readable list of
// "model (provider)" entries for use in warning messages.
func joinProviders(models []ManifestLLMModel) string {
	parts := make([]string, 0, len(models))
	seen := make(map[string]bool)
	for _, m := range models {
		if !seen[m.Provider] {
			seen[m.Provider] = true
			parts = append(parts, fmt.Sprintf("%s (%s)", m.Model, m.Provider))
		}
	}
	return strings.Join(parts, ", ")
}
