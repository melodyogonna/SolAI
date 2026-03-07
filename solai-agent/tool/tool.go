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
	// capManager provides capability dispatch for runtime requests from the tool.
	capManager *capability.CapabilityManager
}

// NewAgenticTool constructs an AgenticTool from a manifest, its directory,
// an optional LLM config, sandbox policy, resolved env vars, and the
// capability manager used to dispatch capability requests at runtime.
func NewAgenticTool(manifest Manifest, dir string, llmCfg *capability.LLMConfig, policy SandboxPolicy, toolEnv []string, capManager *capability.CapabilityManager) *AgenticTool {
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
		capManager:    capManager,
	}
}

func (t *AgenticTool) Name() string        { return t.manifest.Name }
func (t *AgenticTool) Description() string { return t.manifest.Description }

// Call implements tools.Tool. It is invoked by langchaingo when the LLM selects
// this tool during the ReAct loop.
func (t *AgenticTool) Call(ctx context.Context, input string) (string, error) {
	taskInput := parseTaskInput(input)
	taskInput.Type = "input"
	if t.capManager != nil {
		taskInput.AvailableCapabilities = t.capManager.BuildToolCapabilitySection()
	}

	extraEnv := append([]string(nil), t.toolEnv...)
	if t.llmCfg != nil {
		extraEnv = append(extraEnv, t.llmCfg.Env()...)
	}

	handler := t.buildRequestHandler(ctx)

	output, err := RunTool(ctx, t.dir, t.manifest.Executable, taskInput, t.timeout, extraEnv, t.sandboxPolicy, handler)
	if err != nil {
		slog.Error("tool infrastructure error", "tool", t.manifest.Name, "err", err)
		msg := fmt.Sprintf("Tool infrastructure error: %v", err)
		return msg, err
	}

	if output.Type == "error" {
		var errMsg string
		if err := json.Unmarshal(output.Output, &errMsg); err != nil {
			errMsg = string(output.Output)
		}
		return fmt.Sprintf("Tool error: %s", errMsg), nil
	}

	return string(output.Output), nil
}

// buildRequestHandler returns a RequestHandler that dispatches capability
// requests from the tool to the coordinator's Regular capabilities.
func (t *AgenticTool) buildRequestHandler(ctx context.Context) RequestHandler {
	if t.capManager == nil {
		return nil
	}
	return func(ctx context.Context, capName, action, input string) (string, error) {
		c := t.capManager.GetByName(capName)
		if c == nil {
			return "", fmt.Errorf("unknown capability %q", capName)
		}
		if c.Class() != capability.Regular {
			return "", fmt.Errorf("capability %q is not accessible to agentic tools", capName)
		}
		req, _ := json.Marshal(map[string]string{"action": action, "input": input})
		return c.Execute(ctx, string(req))
	}
}
