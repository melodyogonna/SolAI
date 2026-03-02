package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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

// RunTool spawns the given executable in dir, writes input as JSON to its stdin,
// waits for the process to finish, and parses ToolOutput from stdout.
//
// The process is killed if ctx is cancelled or timeout elapses.
// stderr from the subprocess is captured and included in error messages.
func RunTool(ctx context.Context, dir, executable string, input ToolInput, timeout time.Duration) (ToolOutput, error) {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return ToolOutput{}, fmt.Errorf("marshalling tool input: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, executable)
	cmd.Dir = dir
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
