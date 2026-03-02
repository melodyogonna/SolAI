package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ToolInput is the JSON structure written to a tool's stdin.
type ToolInput struct {
	// Overview is a one-sentence description of the task.
	Overview string `json:"overview"`

	// Tasks is a list of discrete sub-tasks for the tool to accomplish, in order.
	Tasks []string `json:"tasks"`
}

// ToolOutput is the JSON structure read from a tool's stdout.
type ToolOutput struct {
	// Type is one of: "success", "error".
	Type string `json:"type"`

	// Output holds the tool's result. For "success" it may be any JSON value.
	// For "error" it is typically a human-readable string.
	Output json.RawMessage `json:"output"`
}

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
// toolDir is the directory containing the tool's files (mounted read-only at /app).
// executable is the path relative to toolDir that will be run as /app/<executable>.
func buildBwrapArgs(policy SandboxPolicy, toolDir, executable string) []string {
	// Strip leading "./" from executable so we can join it cleanly.
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

// RunTool spawns the given executable in dir, writes input as JSON to its stdin,
// waits for the process to finish, and parses ToolOutput from stdout.
//
// When policy.BwrapPath is non-empty the tool runs inside a bubblewrap
// sandbox according to the policy. When empty, the tool runs as a plain
// subprocess (unsandboxed, e.g. when go generate has not been run).
//
// extraEnv is an optional list of "KEY=VALUE" strings appended to the subprocess
// environment. Pass nil for no extra variables (the subprocess inherits the parent env).
// The process is killed if ctx is cancelled or timeout elapses.
// stderr from the subprocess is captured and included in error messages.
func RunTool(ctx context.Context, dir, executable string, input ToolInput, timeout time.Duration, extraEnv []string, policy SandboxPolicy) (ToolOutput, error) {
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
		// Sandboxed tools run with a clean environment; only inject extraEnv.
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

	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return ToolOutput{}, fmt.Errorf("tool process failed: %w\nstderr: %s", err, stderrStr)
		}
		return ToolOutput{}, fmt.Errorf("tool process failed: %w", err)
	}

	return parseToolOutput(stdout.Bytes())
}

// parseToolOutput unmarshals raw bytes into ToolOutput.
func parseToolOutput(raw []byte) (ToolOutput, error) {
	var out ToolOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return ToolOutput{}, fmt.Errorf("parsing tool output: %w\nraw output: %s", err, string(raw))
	}
	if out.Type == "" {
		return ToolOutput{}, fmt.Errorf("tool output missing required field 'type'\nraw output: %s", string(raw))
	}
	return out, nil
}

// parseTaskInput converts the LLM's action input string into a ToolInput.
// The LLM may produce either a JSON object or a plain English string.
// Both forms are handled so tool execution is robust to LLM output variation.
func parseTaskInput(input string) ToolInput {
	var ti ToolInput
	if err := json.Unmarshal([]byte(input), &ti); err == nil && ti.Overview != "" {
		return ti
	}
	// Fallback: treat the entire input as the overview.
	return ToolInput{
		Overview: input,
		Tasks:    []string{input},
	}
}
