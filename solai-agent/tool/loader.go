package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/melodyogonna/solai/solai-agent/capability"
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
// Returns:
//   - []tools.Tool: successfully loaded tools (may be empty if none found)
//   - []error: one warning per tool that failed to load or was disabled
//   - error: fatal only if toolsDir itself cannot be read
func LoadTools(toolsDir string, provider *capability.LLMProvider) ([]tools.Tool, []error, error) {
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return nil, nil, fmt.Errorf("reading tools directory %s: %w", toolsDir, err)
	}

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

		loaded = append(loaded, NewAgenticTool(manifest, toolDir, llmCfg))
	}

	return loaded, warnings, nil
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
