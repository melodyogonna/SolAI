package tool

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/melodyogonna/solai/solai-agent/capability"
	solaiconfig "github.com/melodyogonna/solai/solai-agent/config"
)

// fakeChecker is a CapabilityChecker used by buildSandboxPolicy tests directly.
type fakeChecker struct {
	available map[string]bool
}

func newChecker(names ...string) *fakeChecker {
	m := make(map[string]bool)
	for _, n := range names {
		m[n] = true
	}
	return &fakeChecker{available: m}
}

func (f *fakeChecker) IsRegularCapabilityAvailable(name string) bool {
	return f.available[name]
}

// newCapManager builds a *capability.CapabilityManager with the named capabilities.
// Supported names: "network-manager". Unknown names are silently ignored.
func newCapManager(names ...string) *capability.CapabilityManager {
	for _, n := range names {
		switch n {
		case "network-manager":
			capability.Register(n, func() capability.Capability {
				return capability.NewNetworkManagerCapability()
			})
		}
	}
	return capability.SetUp(names)
}

// setupTool creates a fake tool directory with a manifest and stub executable.
func setupTool(t *testing.T, toolsDir, name, manifestContent string) {
	t.Helper()
	toolDir := filepath.Join(toolsDir, name)
	binDir := filepath.Join(toolDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(toolDir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}
	// Write a stub executable.
	execPath := filepath.Join(binDir, name)
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho '{\"type\":\"success\",\"output\":null}'"), 0755); err != nil {
		t.Fatal(err)
	}
}

func validManifestJSON(name string) string {
	return `{"name":"` + name + `","description":"A test tool","version":"1.0.0","executable":"./bin/` + name + `"}`
}

// ---- LoadTools basic cases --------------------------------------------------

func TestLoadTools_EmptyDir(t *testing.T) {
	toolsDir := t.TempDir()
	provider := capability.NewLLMProviderFromMap(nil)
	tools, warnings, err := LoadTools(toolsDir, provider, newCapManager(), "", nil)
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(warnings))
	}
}

func TestLoadTools_InvalidDir(t *testing.T) {
	provider := capability.NewLLMProviderFromMap(nil)
	_, _, err := LoadTools("/nonexistent/tools/dir", provider, newCapManager(), "", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func TestLoadTools_ValidTool_NoLLM(t *testing.T) {
	toolsDir := t.TempDir()
	setupTool(t, toolsDir, "my-tool", validManifestJSON("my-tool"))

	provider := capability.NewLLMProviderFromMap(nil)
	tools, warnings, err := LoadTools(toolsDir, provider, newCapManager(), "", nil)
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name() != "my-tool" {
		t.Errorf("tool name: got %q, want my-tool", tools[0].Name())
	}
}

func TestLoadTools_MultipleTools(t *testing.T) {
	toolsDir := t.TempDir()
	setupTool(t, toolsDir, "tool-a", validManifestJSON("tool-a"))
	setupTool(t, toolsDir, "tool-b", validManifestJSON("tool-b"))

	provider := capability.NewLLMProviderFromMap(nil)
	tools, _, err := LoadTools(toolsDir, provider, newCapManager(), "", nil)
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

func TestLoadTools_MissingManifest_Warning(t *testing.T) {
	toolsDir := t.TempDir()
	// Create a directory without a manifest.
	if err := os.MkdirAll(filepath.Join(toolsDir, "no-manifest"), 0755); err != nil {
		t.Fatal(err)
	}

	provider := capability.NewLLMProviderFromMap(nil)
	tools, warnings, err := LoadTools(toolsDir, provider, newCapManager(), "", nil)
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning for missing manifest, got %d: %v", len(warnings), warnings)
	}
}

func TestLoadTools_InvalidManifest_Warning(t *testing.T) {
	toolsDir := t.TempDir()
	toolDir := filepath.Join(toolsDir, "bad-tool")
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "manifest.json"), []byte("{invalid json"), 0644); err != nil {
		t.Fatal(err)
	}

	provider := capability.NewLLMProviderFromMap(nil)
	_, warnings, err := LoadTools(toolsDir, provider, newCapManager(), "", nil)
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning for invalid manifest, got %d", len(warnings))
	}
}

func TestLoadTools_MissingExecutable_Warning(t *testing.T) {
	toolsDir := t.TempDir()
	toolDir := filepath.Join(toolsDir, "no-exec")
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Manifest points to an executable that doesn't exist.
	manifest := `{"name":"no-exec","description":"d","version":"1.0.0","executable":"./bin/no-exec"}`
	if err := os.WriteFile(filepath.Join(toolDir, "manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	provider := capability.NewLLMProviderFromMap(nil)
	tools, warnings, err := LoadTools(toolsDir, provider, newCapManager(), "", nil)
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools when executable missing, got %d", len(tools))
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
}

// ---- LLM options resolution -------------------------------------------------

func TestLoadTools_LLMOptions_ProviderConfigured(t *testing.T) {
	toolsDir := t.TempDir()
	manifest := `{
		"name": "llm-tool",
		"description": "requires llm",
		"version": "1.0.0",
		"executable": "./bin/llm-tool",
		"llm_options": {
			"primary": "gemini-2.5-pro",
			"supported": [{"model": "gemini-2.5-pro", "provider": "google"}]
		}
	}`
	setupTool(t, toolsDir, "llm-tool", manifest)

	provider := capability.NewLLMProviderFromMap(map[string]string{"google": "gkey"})
	tools, warnings, err := LoadTools(toolsDir, provider, newCapManager(), "", nil)
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
}

func TestLoadTools_LLMOptions_NoProviderConfigured_Warning(t *testing.T) {
	toolsDir := t.TempDir()
	manifest := `{
		"name": "llm-tool",
		"description": "requires llm",
		"version": "1.0.0",
		"executable": "./bin/llm-tool",
		"llm_options": {
			"primary": "gemini-2.5-pro",
			"supported": [{"model": "gemini-2.5-pro", "provider": "google"}]
		}
	}`
	setupTool(t, toolsDir, "llm-tool", manifest)

	provider := capability.NewLLMProviderFromMap(nil) // no providers configured
	tools, warnings, err := LoadTools(toolsDir, provider, newCapManager(), "", nil)
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected tool to be disabled, got %d tools", len(tools))
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning for missing LLM provider, got %d: %v", len(warnings), warnings)
	}
}

// ---- Required capabilities --------------------------------------------------

func TestLoadTools_RequiredCapability_Available(t *testing.T) {
	toolsDir := t.TempDir()
	manifest := `{
		"name": "net-tool",
		"description": "needs network",
		"version": "1.0.0",
		"executable": "./bin/net-tool",
		"required_capabilities": ["network-manager"]
	}`
	setupTool(t, toolsDir, "net-tool", manifest)

	provider := capability.NewLLMProviderFromMap(nil)
	capMgr := newCapManager("network-manager")
	tools, warnings, err := LoadTools(toolsDir, provider, capMgr, "", nil)
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
}

func TestLoadTools_RequiredCapability_Missing_Warning(t *testing.T) {
	toolsDir := t.TempDir()
	manifest := `{
		"name": "net-tool",
		"description": "needs network",
		"version": "1.0.0",
		"executable": "./bin/net-tool",
		"required_capabilities": ["network-manager"]
	}`
	setupTool(t, toolsDir, "net-tool", manifest)

	provider := capability.NewLLMProviderFromMap(nil)
	capMgr := newCapManager() // network-manager not available
	tools, warnings, err := LoadTools(toolsDir, provider, capMgr, "", nil)
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected tool to be disabled, got %d tools", len(tools))
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning for missing capability, got %d: %v", len(warnings), warnings)
	}
}

// ---- buildSandboxPolicy -----------------------------------------------------

func TestBuildSandboxPolicy_NoCapabilities(t *testing.T) {
	m := Manifest{Name: "t", Description: "d", Executable: "./bin/t"}
	policy, err := buildSandboxPolicy(m, newChecker(), "/usr/bin/bwrap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if policy.ShareNet {
		t.Error("expected ShareNet=false for no capabilities")
	}
	if policy.BwrapPath != "/usr/bin/bwrap" {
		t.Errorf("BwrapPath: got %q", policy.BwrapPath)
	}
}

func TestBuildSandboxPolicy_NetworkManager_SetsShareNet(t *testing.T) {
	m := Manifest{
		Name:                 "t",
		Description:          "d",
		Executable:           "./bin/t",
		RequiredCapabilities: []string{"network-manager"},
	}
	checker := newChecker("network-manager")
	policy, err := buildSandboxPolicy(m, checker, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !policy.ShareNet {
		t.Error("expected ShareNet=true for network-manager capability")
	}
}

func TestBuildSandboxPolicy_MissingCapability_Error(t *testing.T) {
	m := Manifest{
		Name:                 "t",
		Description:          "d",
		Executable:           "./bin/t",
		RequiredCapabilities: []string{"file-manager"},
	}
	_, err := buildSandboxPolicy(m, newChecker(), "")
	if err == nil {
		t.Fatal("expected error for missing required capability")
	}
}

// ---- Non-directory entries skipped ------------------------------------------

func TestLoadTools_SkipsFiles(t *testing.T) {
	toolsDir := t.TempDir()
	// Create a file (not a directory) at the top level — should be skipped.
	if err := os.WriteFile(filepath.Join(toolsDir, "not-a-dir"), []byte("file"), 0644); err != nil {
		t.Fatal(err)
	}
	// Add a valid tool directory.
	setupTool(t, toolsDir, "real-tool", validManifestJSON("real-tool"))

	provider := capability.NewLLMProviderFromMap(nil)
	tools, _, err := LoadTools(toolsDir, provider, newCapManager(), "", nil)
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if len(tools) != 1 {
		t.Errorf("expected 1 tool (file skipped), got %d", len(tools))
	}
}

// ---- GOARCH in binary name (informational) ----------------------------------

func TestCurrentArch(t *testing.T) {
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		t.Logf("running on arch %q — tool binary names use this arch suffix", arch)
	}
}

// ---- resolveToolEnv ---------------------------------------------------------

func makeEnvManifest(required, sensitive bool, names ...string) Manifest {
	vars := make([]EnvVar, len(names))
	for i, n := range names {
		vars[i] = EnvVar{Name: n, Required: required, Sensitive: sensitive}
	}
	return Manifest{Name: "my-tool", Description: "d", Version: "1.0.0", Executable: "./bin/my-tool", Env: vars}
}

func cfgWithToolEnv(toolName string, kvs map[string]string) *solaiconfig.SolaiConfig {
	cfg := solaiconfig.DefaultConfig()
	for k, v := range kvs {
		_ = cfg.Set("tool-env."+toolName+"."+k, v)
	}
	return cfg
}

func TestResolveToolEnv_NoEnvDeclared(t *testing.T) {
	m := Manifest{Name: "my-tool", Description: "d", Version: "1.0.0", Executable: "./bin/my-tool"}
	env, err := resolveToolEnv(m, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env != nil {
		t.Errorf("expected nil, got %v", env)
	}
}

func TestResolveToolEnv_RequiredVar_Configured(t *testing.T) {
	m := makeEnvManifest(true, true, "API_KEY")
	cfg := cfgWithToolEnv("my-tool", map[string]string{"API_KEY": "secret"})
	env, err := resolveToolEnv(m, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env) != 1 || env[0] != "API_KEY=secret" {
		t.Errorf("got %v, want [API_KEY=secret]", env)
	}
}

func TestResolveToolEnv_RequiredVar_Missing_Error(t *testing.T) {
	m := makeEnvManifest(true, true, "API_KEY")
	_, err := resolveToolEnv(m, solaiconfig.DefaultConfig())
	if err == nil {
		t.Fatal("expected error for missing required var")
	}
}

func TestResolveToolEnv_RequiredVar_NilConfig_Error(t *testing.T) {
	m := makeEnvManifest(true, false, "API_KEY")
	_, err := resolveToolEnv(m, nil)
	if err == nil {
		t.Fatal("expected error when config is nil and var is required")
	}
}

func TestResolveToolEnv_OptionalVar_Missing_NoError(t *testing.T) {
	m := makeEnvManifest(false, false, "OPTIONAL_VAR")
	env, err := resolveToolEnv(m, solaiconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env) != 0 {
		t.Errorf("expected empty env, got %v", env)
	}
}

func TestResolveToolEnv_SensitiveVar_Included(t *testing.T) {
	// sensitive=true is metadata only; the value should still be injected
	m := makeEnvManifest(false, true, "SECRET_KEY")
	cfg := cfgWithToolEnv("my-tool", map[string]string{"SECRET_KEY": "topsecret"})
	env, err := resolveToolEnv(m, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env) != 1 || env[0] != "SECRET_KEY=topsecret" {
		t.Errorf("got %v", env)
	}
}

func TestResolveToolEnv_MultipleVars_MixedRequiredness(t *testing.T) {
	m := Manifest{
		Name: "my-tool", Description: "d", Version: "1.0.0", Executable: "./bin/my-tool",
		Env: []EnvVar{
			{Name: "REQUIRED_KEY", Required: true},
			{Name: "OPTIONAL_KEY", Required: false},
		},
	}
	cfg := cfgWithToolEnv("my-tool", map[string]string{"REQUIRED_KEY": "req-val"})
	env, err := resolveToolEnv(m, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env) != 1 || env[0] != "REQUIRED_KEY=req-val" {
		t.Errorf("got %v, want [REQUIRED_KEY=req-val]", env)
	}
}

// ---- LoadTools env integration ----------------------------------------------

func TestLoadTools_RequiredEnvVar_Missing_Warning(t *testing.T) {
	toolsDir := t.TempDir()
	manifest := `{"name":"api-tool","description":"d","version":"1.0.0","executable":"./bin/api-tool","env":[{"name":"API_KEY","required":true,"sensitive":true}]}`
	setupTool(t, toolsDir, "api-tool", manifest)

	provider := capability.NewLLMProviderFromMap(nil)
	tools, warnings, err := LoadTools(toolsDir, provider, newCapManager(), "", solaiconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools (disabled), got %d", len(tools))
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
}

func TestLoadTools_RequiredEnvVar_Configured_Loads(t *testing.T) {
	toolsDir := t.TempDir()
	manifest := `{"name":"api-tool","description":"d","version":"1.0.0","executable":"./bin/api-tool","env":[{"name":"API_KEY","required":true,"sensitive":true}]}`
	setupTool(t, toolsDir, "api-tool", manifest)

	cfg := cfgWithToolEnv("api-tool", map[string]string{"API_KEY": "secret"})
	provider := capability.NewLLMProviderFromMap(nil)
	tools, warnings, err := LoadTools(toolsDir, provider, newCapManager(), "", cfg)
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
}

func TestLoadTools_OptionalEnvVar_Missing_Loads(t *testing.T) {
	toolsDir := t.TempDir()
	manifest := `{"name":"opt-tool","description":"d","version":"1.0.0","executable":"./bin/opt-tool","env":[{"name":"OPT_KEY","required":false}]}`
	setupTool(t, toolsDir, "opt-tool", manifest)

	provider := capability.NewLLMProviderFromMap(nil)
	tools, warnings, err := LoadTools(toolsDir, provider, newCapManager(), "", solaiconfig.DefaultConfig())
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
}
