package cli

import (
	"testing"

	solaiconfig "github.com/melodyogonna/solai/solai-agent/config"
)

func defaultTestConfig() *solaiconfig.SolaiConfig {
	cfg := solaiconfig.DefaultConfig()
	cfg.Model.Provider = "google"
	cfg.Model.Name = "gemini-2.5-pro"
	cfg.Providers["google"] = "test-key"
	cfg.UserGoals = "Monitor SOL"
	return cfg
}

// ---- buildAgentBwrapArgs ----------------------------------------------------

func TestBuildAgentBwrapArgs_StartsWithUnshareAll(t *testing.T) {
	cfg := defaultTestConfig()
	args := buildAgentBwrapArgs(cfg, "/usr/bin/solai", "/etc/solai/config.json", "/tools")
	if len(args) == 0 || args[0] != "--unshare-all" {
		t.Errorf("expected --unshare-all as first arg, got %v", args)
	}
}

func TestBuildAgentBwrapArgs_ShareNet_True(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Sandbox.ShareNet = true
	args := buildAgentBwrapArgs(cfg, "/usr/bin/solai", "/etc/solai/config.json", "/tools")

	found := false
	for _, a := range args {
		if a == "--share-net" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --share-net when ShareNet=true, got %v", args)
	}
}

func TestBuildAgentBwrapArgs_ShareNet_False(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Sandbox.ShareNet = false
	args := buildAgentBwrapArgs(cfg, "/usr/bin/solai", "/etc/solai/config.json", "/tools")

	for _, a := range args {
		if a == "--share-net" {
			t.Errorf("unexpected --share-net when ShareNet=false: %v", args)
			return
		}
	}
}

func TestBuildAgentBwrapArgs_BindsConfigJSON(t *testing.T) {
	cfg := defaultTestConfig()
	configPath := "/home/user/.solai/config.json"
	args := buildAgentBwrapArgs(cfg, "/usr/bin/solai", configPath, "/tools")

	// Look for --ro-bind configPath /etc/solai/config.json
	found := false
	for i := 0; i+2 < len(args); i++ {
		if args[i] == "--ro-bind" && args[i+1] == configPath && args[i+2] == "/etc/solai/config.json" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --ro-bind %s /etc/solai/config.json in args: %v", configPath, args)
	}
}

func TestBuildAgentBwrapArgs_BindsToolsDir(t *testing.T) {
	cfg := defaultTestConfig()
	toolsDir := "/home/user/.solai/tools"
	args := buildAgentBwrapArgs(cfg, "/usr/bin/solai", "/etc/solai/config.json", toolsDir)

	found := false
	for i := 0; i+2 < len(args); i++ {
		if args[i] == "--ro-bind" && args[i+1] == toolsDir && args[i+2] == "/tools" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --ro-bind %s /tools in args: %v", toolsDir, args)
	}
}

func TestBuildAgentBwrapArgs_BindsSelfExe(t *testing.T) {
	cfg := defaultTestConfig()
	selfExe := "/usr/local/bin/solai"
	args := buildAgentBwrapArgs(cfg, selfExe, "/etc/solai/config.json", "/tools")

	found := false
	for i := 0; i+2 < len(args); i++ {
		if args[i] == "--ro-bind" && args[i+1] == selfExe && args[i+2] == "/solai" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --ro-bind %s /solai in args: %v", selfExe, args)
	}
}

func TestBuildAgentBwrapArgs_HasDieWithParent(t *testing.T) {
	cfg := defaultTestConfig()
	args := buildAgentBwrapArgs(cfg, "/usr/bin/solai", "/etc/solai/config.json", "/tools")

	found := false
	for _, a := range args {
		if a == "--die-with-parent" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --die-with-parent in args: %v", args)
	}
}

func TestBuildAgentBwrapArgs_EndsWithDoubleDash(t *testing.T) {
	cfg := defaultTestConfig()
	args := buildAgentBwrapArgs(cfg, "/usr/bin/solai", "/etc/solai/config.json", "/tools")

	if len(args) == 0 || args[len(args)-1] != "--" {
		t.Errorf("expected -- as last arg, got %v", args)
	}
}

func TestBuildAgentBwrapArgs_ExtraBinds_ReadOnly(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Sandbox.ExtraBinds = []solaiconfig.FSBind{
		{Path: "/data/readonly", ReadOnly: true},
	}
	args := buildAgentBwrapArgs(cfg, "/usr/bin/solai", "/etc/solai/config.json", "/tools")

	found := false
	for i := 0; i+2 < len(args); i++ {
		if args[i] == "--ro-bind" && args[i+1] == "/data/readonly" && args[i+2] == "/data/readonly" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --ro-bind /data/readonly /data/readonly in args: %v", args)
	}
}

func TestBuildAgentBwrapArgs_ExtraBinds_ReadWrite(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Sandbox.ExtraBinds = []solaiconfig.FSBind{
		{Path: "/data/writable", ReadOnly: false},
	}
	args := buildAgentBwrapArgs(cfg, "/usr/bin/solai", "/etc/solai/config.json", "/tools")

	found := false
	for i := 0; i+2 < len(args); i++ {
		if args[i] == "--bind" && args[i+1] == "/data/writable" && args[i+2] == "/data/writable" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --bind /data/writable /data/writable in args: %v", args)
	}
}

func TestBuildAgentBwrapArgs_HasProcDevTmpfs(t *testing.T) {
	cfg := defaultTestConfig()
	args := buildAgentBwrapArgs(cfg, "/usr/bin/solai", "/etc/solai/config.json", "/tools")

	required := map[string]bool{
		"--proc":  false,
		"--dev":   false,
		"--tmpfs": false,
	}
	for _, a := range args {
		if _, ok := required[a]; ok {
			required[a] = true
		}
	}
	for flag, found := range required {
		if !found {
			t.Errorf("expected %q in args: %v", flag, args)
		}
	}
}
