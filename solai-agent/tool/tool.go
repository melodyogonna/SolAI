package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/melodyogonna/solai/solai-agent/capability"
)

// DefaultToolTimeout is the maximum time a single tool execution may run.
const DefaultToolTimeout = 2 * time.Minute

// AgenticTool implements langchaingo's tools.Tool interface.
// Each instance corresponds to one discovered tool directory.
type AgenticTool struct {
	manifest      Manifest
	dir           string
	timeout       time.Duration
	llmCfg        *capability.LLMConfig
	sandboxPolicy SandboxPolicy
	toolEnv       []string
	// commCap manages per-invocation IPC directories.
	commCap *capability.CommunicationCapability
	// autoPayloads holds payload values pre-resolved at construction time
	// (e.g. wallet_address from the wallet capability). They are merged into
	// every tool invocation's ToolInput.Payloads map.
	autoPayloads map[string]string
	// capabilitySection is appended to every prompt so the inner tool LLM
	// knows what coordinator capabilities it can request and how to do so.
	capabilitySection string
}

// NewAgenticTool constructs an AgenticTool from a manifest, its directory,
// an optional LLM config, sandbox policy, resolved env vars, a communication
// capability for IPC directory management, pre-resolved auto payloads, and a
// capability section to inject into every tool prompt.
func NewAgenticTool(manifest Manifest, dir string, llmCfg *capability.LLMConfig, policy SandboxPolicy, toolEnv []string, commCap *capability.CommunicationCapability, autoPayloads map[string]string, capabilitySection string) *AgenticTool {
	timeout := DefaultToolTimeout
	if manifest.Timeout != "" {
		if d, err := time.ParseDuration(manifest.Timeout); err == nil && d > 0 {
			timeout = d
		}
	}
	return &AgenticTool{
		manifest:          manifest,
		dir:               dir,
		timeout:           timeout,
		llmCfg:            llmCfg,
		sandboxPolicy:     policy,
		toolEnv:           toolEnv,
		commCap:           commCap,
		autoPayloads:      autoPayloads,
		capabilitySection: capabilitySection,
	}
}

func (t *AgenticTool) Name() string { return t.manifest.Name }

// Description returns the manifest description with payload documentation
// appended so the coordinator LLM knows what payloads to pass.
func (t *AgenticTool) Description() string {
	desc := t.manifest.Description
	if len(t.manifest.Payloads) == 0 {
		return desc
	}
	desc += "\n\nPayloads:"
	for _, p := range t.manifest.Payloads {
		line := fmt.Sprintf("\n- %s: %s", p.Name, p.Description)
		if p.Source != "" {
			line += " (auto-injected)"
		}
		desc += line
	}
	return desc
}

// Call implements tools.Tool. It is invoked by langchaingo when the LLM selects
// this tool during the ReAct loop.
func (t *AgenticTool) Call(ctx context.Context, input string) (string, error) {
	taskInput := parseTaskInput(input)

	// Append requestable capabilities so the inner tool LLM can generate
	// capability requests dynamically rather than relying on hardcoded logic.
	if t.capabilitySection != "" {
		taskInput.Prompt += "\n\n" + t.capabilitySection
	}

	if len(t.autoPayloads) > 0 {
		if taskInput.Payload == nil {
			taskInput.Payload = make(map[string]string)
		}
		for k, v := range t.autoPayloads {
			if _, exists := taskInput.Payload[k]; !exists {
				taskInput.Payload[k] = v
			}
		}
	}

	slog.Info("invoking tool", "tool", t.manifest.Name, "prompt", taskInput.Prompt)

	ipcDir, err := t.commCap.Allocate()
	if err != nil {
		return "", fmt.Errorf("allocating IPC directory: %w", err)
	}
	defer t.commCap.Release(ipcDir)

	extraEnv := append([]string(nil), t.toolEnv...)
	if t.llmCfg != nil {
		extraEnv = append(extraEnv, t.llmCfg.Env()...)
	}

	// Set SOLAI_IPC_DIR to the in-process path of the IPC directory.
	// In the sandbox, the host ipcDir is bind-mounted at /run/solai.
	policy := t.sandboxPolicy
	policy.IPCDir = ipcDir
	if policy.BwrapPath != "" {
		extraEnv = append(extraEnv, "SOLAI_IPC_DIR=/run/solai")
	} else {
		extraEnv = append(extraEnv, "SOLAI_IPC_DIR="+ipcDir)
	}

	output, err := RunTool(ctx, t.dir, t.manifest.Executable, taskInput, t.timeout, extraEnv, policy)
	if err != nil {
		slog.Error("tool infrastructure error", "tool", t.manifest.Name, "err", err)
		msg := fmt.Sprintf("Tool infrastructure error: %v", err)
		return msg, err
	}

	slog.Info("tool exited", "tool", t.manifest.Name, "output_type", output.Type)

	switch output.Type {
	case "error":
		var errMsg string
		if err := json.Unmarshal(output.Payload, &errMsg); err != nil {
			errMsg = string(output.Payload)
		}
		slog.Warn("tool returned error", "tool", t.manifest.Name, "error", errMsg)
		return fmt.Sprintf("Tool error: %s", errMsg), nil
	case "request":
		slog.Info("tool requested capability", "tool", t.manifest.Name, "payload", string(output.Payload))
		return string(output.Payload), nil
	}

	slog.Debug("tool success payload", "tool", t.manifest.Name, "payload", string(output.Payload))
	return string(output.Payload), nil
}
