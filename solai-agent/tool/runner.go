package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ToolInput is the JSON structure written to input.json in the IPC directory
// before starting a tool.
type ToolInput struct {
	// Type is always "input".
	Type string `json:"type"`

	// Prompt describes what to do. Always present.
	Prompt string `json:"prompt"`

	// Payload carries raw data on re-invocation (e.g. a base64-encoded signed
	// transaction returned by a capability). Empty on the initial invocation.
	Payload string `json:"payload,omitempty"`

	// Tasks is a list of discrete sub-tasks for the tool to accomplish, in order.
	// May be empty on re-invocation.
	Tasks []string `json:"tasks,omitempty"`

	// Capabilities holds pre-injected capability values for the tool.
	// Currently contains "wallet_address" when the wallet capability is active.
	Capabilities map[string]string `json:"capabilities,omitempty"`

	// ErrorDetails is non-empty when the re-invocation follows a failed
	// capability request; it carries the error message from that failure.
	ErrorDetails string `json:"error_details,omitempty"`
}

// ToolOutput is the JSON structure read from output.json after the tool exits.
type ToolOutput struct {
	// Type is one of: "success", "error", "request".
	Type string `json:"type"`

	// Payload holds the tool's result. For "success" it may be any JSON value.
	// For "error" it is a human-readable string.
	// For "request" it is {"capability":"...","action":"...","input":"..."}.
	Payload json.RawMessage `json:"payload"`
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

	// IPCDir is the host path of the per-invocation IPC directory. When
	// non-empty and BwrapPath is set, it is bind-mounted at /run/solai inside
	// the sandbox. The tool reads input.json and writes output.json there.
	IPCDir string
}

// systemLibDirs lists host paths that dynamically-linked tool binaries need
// inside the sandbox. Each present path is bind-mounted read-only.
var systemLibDirs = []string{
	"/lib",
	"/lib64",
	"/usr/lib",
	"/usr/lib64",
	"/etc/ld.so.cache",
	"/etc/ld.so.conf",
	"/etc/ld.so.conf.d",
}

// buildBwrapArgs constructs the bwrap argument list for the given policy.
func buildBwrapArgs(policy SandboxPolicy, toolDir, executable string) []string {
	// Clean the executable path to resolve "./" prefixes while preserving any
	// subdirectory structure (e.g. "./bin/token-price" → "bin/token-price").
	// The tool directory is bind-mounted at /app, so the in-sandbox path is
	// /app/<cleaned-relative-path>.
	rel := filepath.Clean(executable)

	args := []string{
		"--unshare-all",
		"--ro-bind", toolDir, "/app",
		"--tmpfs", "/tmp",
		"--proc", "/proc",
		"--dev", "/dev",
		"--die-with-parent",
	}

	// Bind system library directories so dynamically-linked tool binaries
	// can find their ELF interpreter and shared libraries.
	for _, p := range systemLibDirs {
		if _, err := os.Stat(p); err == nil {
			args = append(args, "--ro-bind", p, p)
		}
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

	// Bind the IPC directory so the tool can read input.json and write output.json.
	if policy.IPCDir != "" {
		args = append(args, "--bind", policy.IPCDir, "/run/solai")
	}

	args = append(args, "--", "/app/"+rel)
	return args
}

// RunTool writes input.json to the IPC directory, spawns the tool subprocess,
// waits for it to exit, then reads and returns output.json.
//
// The tool reads $SOLAI_IPC_DIR/input.json on startup and writes
// $SOLAI_IPC_DIR/output.json before exiting. The IPC directory must already
// exist with its host path set in policy.IPCDir; SOLAI_IPC_DIR in extraEnv
// must point to the in-process view of that directory.
//
// The process is killed if ctx is cancelled or the per-tool timeout elapses.
func RunTool(ctx context.Context, dir, executable string, input ToolInput, timeout time.Duration, extraEnv []string, policy SandboxPolicy) (ToolOutput, error) {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return ToolOutput{}, fmt.Errorf("marshalling tool input: %w", err)
	}
	if err := os.WriteFile(filepath.Join(policy.IPCDir, "input.json"), inputJSON, 0600); err != nil {
		return ToolOutput{}, fmt.Errorf("writing input.json: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if policy.BwrapPath != "" {
		bwrapArgs := buildBwrapArgs(policy, dir, executable)
		slog.Debug("running tool in sandbox", "bwrap", policy.BwrapPath, "args", strings.Join(bwrapArgs, " "))
		cmd = exec.CommandContext(ctx, policy.BwrapPath, bwrapArgs...)
		if len(extraEnv) > 0 {
			cmd.Env = extraEnv
		}
	} else {
		// Resolve to an absolute path so exec finds the binary regardless of
		// the agent's current working directory (cmd.Dir only affects the child
		// process's working directory, not the path lookup for the executable).
		absExec := filepath.Join(dir, executable)
		cmd = exec.CommandContext(ctx, absExec)
		cmd.Dir = dir
		if len(extraEnv) > 0 {
			cmd.Env = append(os.Environ(), extraEnv...)
		}
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return ToolOutput{}, fmt.Errorf("starting tool: %w", err)
	}

	waitErr := cmd.Wait()
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

	data, err := os.ReadFile(filepath.Join(policy.IPCDir, "output.json"))
	if err != nil {
		return ToolOutput{}, fmt.Errorf("reading output.json: %w", err)
	}
	var output ToolOutput
	if err := json.Unmarshal(data, &output); err != nil {
		return ToolOutput{}, fmt.Errorf("parsing output.json: %w", err)
	}
	if output.Type == "" {
		return ToolOutput{}, fmt.Errorf("tool produced no output (output.json has empty type)")
	}
	return output, nil
}

// parseTaskInput converts the LLM's action input string into a ToolInput.
// The LLM may produce either a JSON object or a plain English string.
func parseTaskInput(input string) ToolInput {
	var ti ToolInput
	if err := json.Unmarshal([]byte(input), &ti); err == nil && ti.Prompt != "" {
		return ti
	}
	return ToolInput{
		Prompt: input,
		Tasks:  []string{input},
	}
}
