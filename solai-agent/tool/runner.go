package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ToolInput is the JSON structure written to a tool's stdin to start execution.
type ToolInput struct {
	// Type is always "input". Included so tools can distinguish coordinator
	// messages from capability responses using the same type field.
	Type string `json:"type"`

	// Overview is a one-sentence description of the task.
	Overview string `json:"overview"`

	// Tasks is a list of discrete sub-tasks for the tool to accomplish, in order.
	Tasks []string `json:"tasks"`

	// AvailableCapabilities is the generated documentation block describing
	// which Regular capabilities the tool can request, their actions, and
	// input/output formats. Empty when no requestable capabilities are registered.
	AvailableCapabilities string `json:"available_capabilities,omitempty"`
}

// ToolOutput is the final JSON structure read from a tool's stdout.
type ToolOutput struct {
	// Type is one of: "success", "error".
	Type string `json:"type"`

	// Output holds the tool's result. For "success" it may be any JSON value.
	// For "error" it is a human-readable string.
	Output json.RawMessage `json:"output"`
}

// ToolRequest is written by a tool to stdout to request a coordinator capability.
// After writing this, the tool must block reading stdin for a ToolResponse.
type ToolRequest struct {
	// Type is always "request".
	Type string `json:"type"`

	// Capability is the name of the Regular capability to invoke (e.g. "wallet").
	Capability string `json:"capability"`

	// Action is the specific action to perform (e.g. "sign", "address").
	Action string `json:"action"`

	// Input is the action's input payload (e.g. base64-encoded bytes to sign).
	Input string `json:"input"`
}

// ToolResponse is written by the coordinator to the tool's stdin in reply to a ToolRequest.
type ToolResponse struct {
	// Type is always "response".
	Type string `json:"type"`

	// Output is the capability's result on success.
	Output string `json:"output,omitempty"`

	// Error is a human-readable message when the capability request failed.
	Error string `json:"error,omitempty"`
}

// RequestHandler dispatches a capability request from a tool to the coordinator.
// capability is the name of the Regular capability; action and input correspond
// to ToolRequest.Action and ToolRequest.Input.
type RequestHandler func(ctx context.Context, capability, action, input string) (string, error)

// FSBind is a filesystem path bind-mounted into the sandbox.
type FSBind struct {
	// Path is the host path to bind-mount. It becomes available at the same
	// path inside the sandbox.
	Path string

	// ReadOnly, when true, mounts the path read-only (--ro-bind).
	// When false, the path is writable (--bind).
	ReadOnly bool
}

// SandboxPolicy defines the isolation constraints applied when running a tool.
// The zero value represents the minimal (deny-all network, no extra mounts) policy.
type SandboxPolicy struct {
	// BwrapPath is the path to the bwrap binary to use. Empty means sandbox
	// is unavailable and the tool runs as a plain subprocess (unsandboxed).
	BwrapPath string

	// ShareNet, when true, passes --share-net to bwrap, granting the tool
	// access to the host network stack. Corresponds to the "network-manager"
	// capability.
	ShareNet bool

	// FSBinds is an ordered list of extra filesystem paths to expose inside
	// the sandbox. Corresponds to the "file-manager" capability (future).
	FSBinds []FSBind
}

// buildBwrapArgs constructs the bwrap argument list for the given policy.
func buildBwrapArgs(policy SandboxPolicy, toolDir, executable string) []string {
	exec := filepath.Base(executable)

	args := []string{
		"--unshare-all",
		"--ro-bind", toolDir, "/app",
		"--tmpfs", "/tmp",
		"--proc", "/proc",
		"--dev", "/dev",
		"--die-with-parent",
	}

	if policy.ShareNet {
		args = append(args, "--share-net")
	}

	for _, bind := range policy.FSBinds {
		if bind.ReadOnly {
			args = append(args, "--ro-bind", bind.Path, bind.Path)
		} else {
			args = append(args, "--bind", bind.Path, bind.Path)
		}
	}

	args = append(args, "--", "/app/"+exec)
	return args
}

// RunTool spawns the tool, writes the ToolInput to its stdin, and runs the
// bidirectional request/response loop until the tool writes a "success" or
// "error" message.
//
// When a tool writes a "request" message, handler is called to dispatch the
// capability action and the result is written back to the tool's stdin. Pass
// nil for handler if the tool is known not to use capability requests.
//
// The process is killed if ctx is cancelled or the per-tool timeout elapses.
func RunTool(ctx context.Context, dir, executable string, input ToolInput, timeout time.Duration, extraEnv []string, policy SandboxPolicy, handler RequestHandler) (ToolOutput, error) {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return ToolOutput{}, fmt.Errorf("marshalling tool input: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if policy.BwrapPath != "" {
		bwrapArgs := buildBwrapArgs(policy, dir, executable)
		cmd = exec.CommandContext(ctx, policy.BwrapPath, bwrapArgs...)
		if len(extraEnv) > 0 {
			cmd.Env = extraEnv
		}
	} else {
		cmd = exec.CommandContext(ctx, executable)
		cmd.Dir = dir
		if len(extraEnv) > 0 {
			cmd.Env = append(os.Environ(), extraEnv...)
		}
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return ToolOutput{}, fmt.Errorf("creating stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return ToolOutput{}, fmt.Errorf("creating stdout pipe: %w", err)
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return ToolOutput{}, fmt.Errorf("starting tool: %w", err)
	}

	// Write the initial input and keep stdin open for capability responses.
	if _, err := fmt.Fprintf(stdinPipe, "%s\n", inputJSON); err != nil {
		_ = cmd.Process.Kill()
		return ToolOutput{}, fmt.Errorf("writing tool input: %w", err)
	}

	// Process messages until success/error or an error.
	result, loopErr := processMessages(ctx, stdoutPipe, stdinPipe, handler)

	// Close stdin (signals EOF to the tool) and drain any remaining stdout
	// to prevent the subprocess from blocking on a full pipe buffer.
	stdinPipe.Close()
	io.Copy(io.Discard, stdoutPipe) //nolint:errcheck

	waitErr := cmd.Wait()

	if loopErr != nil {
		return ToolOutput{}, loopErr
	}
	if result.Type == "" {
		// No output received — report the process exit error if available.
		if waitErr != nil {
			if ctx.Err() != nil {
				return ToolOutput{}, ctx.Err()
			}
			stderrStr := stderrBuf.String()
			if stderrStr != "" {
				return ToolOutput{}, fmt.Errorf("tool process failed: %w\nstderr: %s", waitErr, stderrStr)
			}
			return ToolOutput{}, fmt.Errorf("tool process failed: %w", waitErr)
		}
		return ToolOutput{}, fmt.Errorf("tool produced no output")
	}
	return result, nil
}

// processMessages reads newline-delimited JSON messages from stdout.
// On "request" it dispatches via handler and writes the response to stdin.
// On "success" or "error" it returns the final ToolOutput.
func processMessages(ctx context.Context, stdout io.Reader, stdin io.Writer, handler RequestHandler) (ToolOutput, error) {
	dec := json.NewDecoder(stdout)
	for {
		var msg struct {
			Type       string          `json:"type"`
			Capability string          `json:"capability"`
			Action     string          `json:"action"`
			Input      string          `json:"input"`
			Output     json.RawMessage `json:"output"`
		}
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				return ToolOutput{}, nil
			}
			return ToolOutput{}, fmt.Errorf("reading tool output: %w", err)
		}

		switch msg.Type {
		case "request":
			resp := dispatchCapabilityRequest(ctx, msg.Capability, msg.Action, msg.Input, handler)
			respJSON, _ := json.Marshal(resp)
			if _, err := fmt.Fprintf(stdin, "%s\n", respJSON); err != nil {
				return ToolOutput{}, fmt.Errorf("writing capability response: %w", err)
			}

		case "success", "error":
			return ToolOutput{Type: msg.Type, Output: msg.Output}, nil
		}
	}
}

// dispatchCapabilityRequest calls the handler and wraps the result in a ToolResponse.
func dispatchCapabilityRequest(ctx context.Context, capName, action, input string, handler RequestHandler) ToolResponse {
	if handler == nil {
		return ToolResponse{Type: "response", Error: "capability requests are not supported"}
	}
	output, err := handler(ctx, capName, action, input)
	if err != nil {
		return ToolResponse{Type: "response", Error: err.Error()}
	}
	return ToolResponse{Type: "response", Output: output}
}

// parseTaskInput converts the LLM's action input string into a ToolInput.
// The LLM may produce either a JSON object or a plain English string.
func parseTaskInput(input string) ToolInput {
	var ti ToolInput
	if err := json.Unmarshal([]byte(input), &ti); err == nil && ti.Overview != "" {
		return ti
	}
	return ToolInput{
		Overview: input,
		Tasks:    []string{input},
	}
}
