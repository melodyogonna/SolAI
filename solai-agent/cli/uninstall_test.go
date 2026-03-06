package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// runUninstallWithDir calls runUninstall after temporarily pointing ToolsDir at dir.
// We do this by setting up the tool directory structure and invoking cobra's RunE directly.

func TestUninstall_Success(t *testing.T) {
	toolsDir := t.TempDir()
	toolDir := filepath.Join(toolsDir, "my-tool")
	if err := os.MkdirAll(filepath.Join(toolDir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "manifest.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := uninstallCmd
	cmd.SetArgs([]string{"my-tool"})

	// Patch config.ToolsDir by temporarily overriding via env var that config.ToolsDir reads.
	t.Setenv("HOME", toolsDir)

	// Directly invoke the handler with a fake toolsDir to avoid filesystem side-effects.
	err := removeToolDir(toolsDir, "my-tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(toolDir); !os.IsNotExist(err) {
		t.Error("expected tool directory to be removed")
	}
}

func TestUninstall_NotInstalled(t *testing.T) {
	toolsDir := t.TempDir()
	err := removeToolDir(toolsDir, "no-such-tool")
	if err == nil {
		t.Fatal("expected error for non-existent tool")
	}
}

func TestUninstall_PathTraversal(t *testing.T) {
	toolsDir := t.TempDir()
	for _, name := range []string{"../evil", "..", ".", "", "a/b", "/absolute"} {
		if err := removeToolDir(toolsDir, name); err == nil {
			t.Errorf("expected error for unsafe name %q, got nil", name)
		}
	}
}

func TestUninstall_RemovesOnlyTargetTool(t *testing.T) {
	toolsDir := t.TempDir()
	for _, name := range []string{"tool-a", "tool-b"} {
		dir := filepath.Join(toolsDir, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := removeToolDir(toolsDir, "tool-a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(toolsDir, "tool-a")); !os.IsNotExist(err) {
		t.Error("tool-a should be removed")
	}
	if _, err := os.Stat(filepath.Join(toolsDir, "tool-b")); err != nil {
		t.Error("tool-b should still exist")
	}
}
