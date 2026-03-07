package tool

import (
	"testing"
)

// ---- parseTaskInput ---------------------------------------------------------

func TestParseTaskInput_ValidJSON(t *testing.T) {
	input := `{"overview":"fetch prices","tasks":["get SOL","get USDC"]}`
	got := parseTaskInput(input)
	if got.Overview != "fetch prices" {
		t.Errorf("Overview: got %q, want %q", got.Overview, "fetch prices")
	}
	if len(got.Tasks) != 2 {
		t.Errorf("Tasks: got %d, want 2", len(got.Tasks))
	}
	if got.Tasks[0] != "get SOL" {
		t.Errorf("Tasks[0]: got %q", got.Tasks[0])
	}
}

func TestParseTaskInput_PlainString(t *testing.T) {
	input := "get the current SOL price"
	got := parseTaskInput(input)
	if got.Overview != input {
		t.Errorf("Overview: got %q, want %q", got.Overview, input)
	}
	if len(got.Tasks) != 1 || got.Tasks[0] != input {
		t.Errorf("Tasks: got %v, want [%q]", got.Tasks, input)
	}
}

func TestParseTaskInput_JSONWithNoOverview_FallsBack(t *testing.T) {
	// Valid JSON but no overview field — fall back to plain-string path.
	input := `{"tasks":["step 1"]}`
	got := parseTaskInput(input)
	if got.Overview != input {
		t.Errorf("expected fallback to plain string, got Overview=%q", got.Overview)
	}
}

func TestParseTaskInput_InvalidJSON_FallsBack(t *testing.T) {
	input := "{broken json"
	got := parseTaskInput(input)
	if got.Overview != input {
		t.Errorf("expected fallback, got Overview=%q", got.Overview)
	}
	if len(got.Tasks) != 1 || got.Tasks[0] != input {
		t.Errorf("unexpected Tasks: %v", got.Tasks)
	}
}

func TestParseTaskInput_EmptyString(t *testing.T) {
	got := parseTaskInput("")
	// Empty string is not valid JSON with overview; falls back.
	if got.Overview != "" {
		t.Errorf("Overview: got %q, want empty", got.Overview)
	}
}

func TestParseTaskInput_JSONWithTasksPreserved(t *testing.T) {
	input := `{"overview":"do thing","tasks":["a","b","c"]}`
	got := parseTaskInput(input)
	if len(got.Tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(got.Tasks))
	}
}

// ---- buildBwrapArgs ---------------------------------------------------------

func TestBuildBwrapArgs_MinimalPolicy(t *testing.T) {
	policy := SandboxPolicy{BwrapPath: "/usr/bin/bwrap"}
	args := buildBwrapArgs(policy, "/tools/my-tool", "./bin/my-tool")

	assertContains := func(args []string, s string) {
		t.Helper()
		for _, a := range args {
			if a == s {
				return
			}
		}
		t.Errorf("expected %q in args %v", s, args)
	}

	assertContains(args, "--unshare-all")
	assertContains(args, "--die-with-parent")
	assertContains(args, "/tools/my-tool")
	assertContains(args, "/app")
	assertContains(args, "--tmpfs")
	assertContains(args, "--proc")
	assertContains(args, "--dev")

	// No --share-net for minimal policy.
	for _, a := range args {
		if a == "--share-net" {
			t.Error("unexpected --share-net for minimal policy")
		}
	}

	// Last argument should be the executable inside /app, preserving any
	// subdirectory structure from the manifest's executable field.
	last := args[len(args)-1]
	if last != "/app/bin/my-tool" {
		t.Errorf("last arg: got %q, want /app/bin/my-tool", last)
	}
}

func TestBuildBwrapArgs_ShareNet(t *testing.T) {
	policy := SandboxPolicy{BwrapPath: "/usr/bin/bwrap", ShareNet: true}
	args := buildBwrapArgs(policy, "/tools/my-tool", "./bin/my-tool")

	found := false
	for _, a := range args {
		if a == "--share-net" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --share-net in args: %v", args)
	}
}

func TestBuildBwrapArgs_FSBinds(t *testing.T) {
	policy := SandboxPolicy{
		BwrapPath: "/usr/bin/bwrap",
		FSBinds: []FSBind{
			{Path: "/data/ro", ReadOnly: true},
			{Path: "/data/rw", ReadOnly: false},
		},
	}
	args := buildBwrapArgs(policy, "/tools/my-tool", "./bin/my-tool")

	// Check ro-bind for read-only.
	roIdx := -1
	for i, a := range args {
		if a == "--ro-bind" && i+1 < len(args) && args[i+1] == "/data/ro" {
			roIdx = i
			break
		}
	}
	if roIdx == -1 {
		t.Errorf("expected --ro-bind /data/ro in args: %v", args)
	}

	// Check bind for read-write.
	rwIdx := -1
	for i, a := range args {
		if a == "--bind" && i+1 < len(args) && args[i+1] == "/data/rw" {
			rwIdx = i
			break
		}
	}
	if rwIdx == -1 {
		t.Errorf("expected --bind /data/rw in args: %v", args)
	}
}

func TestBuildBwrapArgs_ExecutablePreservesSubdir(t *testing.T) {
	// "./bin/token-price" must map to "/app/bin/token-price" inside the sandbox,
	// not "/app/token-price" — the bin/ subdirectory must be preserved.
	policy := SandboxPolicy{}
	args := buildBwrapArgs(policy, "/tools/my-tool", "./bin/token-price")
	last := args[len(args)-1]
	if last != "/app/bin/token-price" {
		t.Errorf("last arg: got %q, want /app/bin/token-price", last)
	}
}
