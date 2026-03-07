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
	// walletAddress is pre-resolved at construction and injected into every
	// tool invocation via ToolInput.Capabilities["wallet_address"].
	walletAddress string
}

// NewAgenticTool constructs an AgenticTool from a manifest, its directory,
// an optional LLM config, sandbox policy, resolved env vars, a communication
// capability for IPC directory management, and an optional pre-resolved wallet
// address to inject into tool inputs.
func NewAgenticTool(manifest Manifest, dir string, llmCfg *capability.LLMConfig, policy SandboxPolicy, toolEnv []string, commCap *capability.CommunicationCapability, walletAddress string) *AgenticTool {
	timeout := DefaultToolTimeout
	if manifest.Timeout != "" {
		if d, err := time.ParseDuration(manifest.Timeout); err == nil && d > 0 {
			timeout = d
		}
	}
	return &AgenticTool{
		manifest:      manifest,
		dir:           dir,
		timeout:       timeout,
		llmCfg:        llmCfg,
		sandboxPolicy: policy,
		toolEnv:       toolEnv,
		commCap:       commCap,
		walletAddress: walletAddress,
	}
}

func (t *AgenticTool) Name() string        { return t.manifest.Name }
func (t *AgenticTool) Description() string { return t.manifest.Description }

// Call implements tools.Tool. It is invoked by langchaingo when the LLM selects
// this tool during the ReAct loop.
func (t *AgenticTool) Call(ctx context.Context, input string) (string, error) {
	taskInput := parseTaskInput(input)
	taskInput.Type = "input"
	if t.walletAddress != "" {
		taskInput.Capabilities = map[string]string{
			"wallet_address": t.walletAddress,
		}
	}

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
	if policy.BwrapPath != "" {
		policy.IPCDir = ipcDir
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

	switch output.Type {
	case "error":
		var errMsg string
		if err := json.Unmarshal(output.Payload, &errMsg); err != nil {
			errMsg = string(output.Payload)
		}
		return fmt.Sprintf("Tool error: %s", errMsg), nil
	case "request":
		return string(output.Payload), nil
	}

	return string(output.Payload), nil
}
