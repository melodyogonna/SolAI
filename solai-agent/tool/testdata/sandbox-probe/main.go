// sandbox-probe is a minimal tool binary used by sandbox_test.go.
// It reports whether /etc/passwd is readable and whether an external
// TCP connection succeeds, so the test can verify sandbox isolation.
//
// Build with CGO_ENABLED=0 to produce a statically linked binary that
// runs inside the sandbox (which has no libc mounts).
package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"time"
)

type output struct {
	PasswdReadable   bool `json:"passwd_readable"`
	NetworkReachable bool `json:"network_reachable"`
}

type toolOutput struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func main() {
	ipcDir := os.Getenv("SOLAI_IPC_DIR")

	result := output{}

	// Check whether /etc/passwd is readable.
	if _, err := os.ReadFile("/etc/passwd"); err == nil {
		result.PasswdReadable = true
	}

	// Check whether an external TCP connection is possible.
	// Use a short timeout so the test doesn't hang long when blocked.
	conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 2*time.Second)
	if err == nil {
		conn.Close()
		result.NetworkReachable = true
	}

	out, _ := json.Marshal(result)
	data, _ := json.Marshal(toolOutput{Type: "success", Payload: json.RawMessage(out)})
	os.WriteFile(filepath.Join(ipcDir, "output.json"), data, 0600) //nolint:errcheck
}
