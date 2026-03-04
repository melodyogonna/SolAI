package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/melodyogonna/solai/solai-agent/capability"
)

// DefaultToolTimeout is the maximum time a single tool execution may run.
// LLM subagents (the expected tool pattern) need time for at least one LLM
// round-trip plus their external API calls, so the default is generous.
const DefaultToolTimeout = 2 * time.Minute

// AgenticTool implements langchaingo's tools.Tool interface.
// Each instance corresponds to one discovered tool directory.
type AgenticTool struct {
	manifest      Manifest
	dir           string
	timeout       time.Duration
	// llmCfg holds the resolved LLM credentials to inject when spawning the tool.
	// Nil for tools that do not declare llm_options in their manifest.
	llmCfg        *capability.LLMConfig
	// sandboxPolicy is the resolved sandbox isolation policy for this tool,
	// built from required_capabilities in the manifest at load time.
	sandboxPolicy SandboxPolicy
}

// NewAgenticTool constructs an AgenticTool from a manifest, its directory,
// an optional resolved LLM config (nil for tools that do not need an LLM),
// and the sandbox policy resolved from the tool's required_capabilities.
//
// The per-tool timeout is taken from manifest.Timeout if set and parseable;
// otherwise DefaultToolTimeout is used.
func NewAgenticTool(manifest Manifest, dir string, llmCfg *capability.LLMConfig, policy SandboxPolicy) *AgenticTool {
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
	}
}

// Name returns the tool's name as declared in manifest.json.
// This is the identifier langchaingo and the LLM use to invoke the tool.
func (t *AgenticTool) Name() string {
	return t.manifest.Name
}

// Description returns the tool's description from manifest.json.
// This is shown verbatim to the LLM to help it decide when to use the tool.
func (t *AgenticTool) Description() string {
	return t.manifest.Description
}

// Call implements tools.Tool. It is invoked by langchaingo when the LLM selects
// this tool during the ReAct loop.
//
// input is the LLM's action input — either a JSON object with "overview"/"tasks"
// fields, or a plain string. Both forms are handled by parseTaskInput.
//
// Tool-level errors (non-zero exit, "error" output type) are returned as strings
// so the LLM reads them as Observations and can adapt its reasoning.
// Only infrastructure errors (binary cannot be spawned) return a Go error.
func (t *AgenticTool) Call(ctx context.Context, input string) (string, error) {
	taskInput := parseTaskInput(input)

	var extraEnv []string
	if t.llmCfg != nil {
		extraEnv = t.llmCfg.Env()
	}
	output, err := RunTool(ctx, t.dir, t.manifest.Executable, taskInput, t.timeout, extraEnv, t.sandboxPolicy)
	if err != nil {
		// Infrastructure failure — the process could not run at all.
		// Return as both a string (so the LLM can observe it) and a Go error
		// (so the executor can decide whether to stop iteration).
		msg := fmt.Sprintf("Tool infrastructure error: %v", err)
		return msg, err
	}

	if output.Type == "error" {
		// The tool ran and reported a failure. Return as an observation string
		// so the LLM can adapt — not as a Go error, since retry may succeed.
		var errMsg string
		if err := json.Unmarshal(output.Output, &errMsg); err != nil {
			errMsg = string(output.Output)
		}
		return fmt.Sprintf("Tool error: %s", errMsg), nil
	}

	// Success — return the raw JSON output for the LLM to read.
	return string(output.Output), nil
}
