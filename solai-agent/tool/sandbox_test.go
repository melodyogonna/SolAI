package tool_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/melodyogonna/solai/solai-agent/sandbox"
	"github.com/melodyogonna/solai/solai-agent/tool"
)

// buildProbe compiles the sandbox-probe binary into a temp directory and
// returns the directory path and the executable filename.
// The binary is built with CGO_ENABLED=0 so it is statically linked and
// runs without libc inside the sandbox.
func buildProbe(t *testing.T) (dir, exe string) {
	t.Helper()
	tmpDir := t.TempDir()
	exePath := filepath.Join(tmpDir, "probe")

	src := filepath.Join("testdata", "sandbox-probe")
	cmd := exec.Command("go", "build", "-o", exePath, ".")
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("building probe: %v\n%s", err, out)
	}
	return tmpDir, "./probe"
}

// probeOutput is the JSON the probe writes inside "output".
type probeOutput struct {
	PasswdReadable   bool `json:"passwd_readable"`
	NetworkReachable bool `json:"network_reachable"`
}

// runProbe runs the probe through RunTool with the given policy and returns
// the decoded probeOutput.
func runProbe(t *testing.T, dir, exe string, policy tool.SandboxPolicy) probeOutput {
	t.Helper()

	// Create an IPC directory for the probe to read input and write output.
	ipcDir := t.TempDir()
	policy.IPCDir = ipcDir

	// Set SOLAI_IPC_DIR to the in-process path; inside bwrap it is bound at /run/solai.
	var ipcEnvVal string
	if policy.BwrapPath != "" {
		ipcEnvVal = "/run/solai"
	} else {
		ipcEnvVal = ipcDir
	}
	extraEnv := []string{"SOLAI_IPC_DIR=" + ipcEnvVal}

	input := tool.ToolInput{Prompt: "probe", Tasks: []string{"probe"}}
	out, err := tool.RunTool(context.Background(), dir, exe, input, 10*time.Second, extraEnv, policy)
	if err != nil {
		t.Fatalf("RunTool: %v", err)
	}
	if out.Type != "success" {
		t.Fatalf("probe returned type %q: %s", out.Type, out.Payload)
	}
	var p probeOutput
	if err := json.Unmarshal(out.Payload, &p); err != nil {
		t.Fatalf("decoding probe output: %v (raw: %s)", err, out.Payload)
	}
	return p
}

// TestSandboxFilesystemIsolation verifies that the sandbox does not expose the
// host filesystem to the tool. /etc/passwd must not be readable.
func TestSandboxFilesystemIsolation(t *testing.T) {
	bwrapPath, err := sandbox.Extract()
	if err != nil {
		t.Skipf("sandbox binary not available: %v", err)
	}
	t.Cleanup(func() { os.Remove(bwrapPath) })

	dir, exe := buildProbe(t)
	policy := tool.SandboxPolicy{BwrapPath: bwrapPath}

	result := runProbe(t, dir, exe, policy)

	if result.PasswdReadable {
		t.Error("sandbox FAILED: tool could read /etc/passwd (filesystem not isolated)")
	} else {
		t.Log("filesystem isolation OK: /etc/passwd not readable inside sandbox")
	}
}

// TestSandboxNetworkIsolation verifies that a tool without network-manager
// cannot reach external hosts.
func TestSandboxNetworkIsolation(t *testing.T) {
	bwrapPath, err := sandbox.Extract()
	if err != nil {
		t.Skipf("sandbox binary not available: %v", err)
	}
	t.Cleanup(func() { os.Remove(bwrapPath) })

	dir, exe := buildProbe(t)
	policy := tool.SandboxPolicy{BwrapPath: bwrapPath, ShareNet: false}

	result := runProbe(t, dir, exe, policy)

	if result.NetworkReachable {
		t.Error("sandbox FAILED: tool reached the network without network-manager capability")
	} else {
		t.Log("network isolation OK: external TCP blocked inside sandbox")
	}
}

// TestSandboxNetworkWithCapability verifies that a tool with ShareNet: true
// can reach external hosts.
func TestSandboxNetworkWithCapability(t *testing.T) {
	bwrapPath, err := sandbox.Extract()
	if err != nil {
		t.Skipf("sandbox binary not available: %v", err)
	}
	t.Cleanup(func() { os.Remove(bwrapPath) })

	dir, exe := buildProbe(t)
	policy := tool.SandboxPolicy{BwrapPath: bwrapPath, ShareNet: true}

	result := runProbe(t, dir, exe, policy)

	if !result.NetworkReachable {
		t.Error("sandbox FAILED: tool could not reach network even with network-manager granted")
	} else {
		t.Log("network capability OK: external TCP reachable with --share-net")
	}
}
