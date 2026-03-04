package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useTempHome redirects ~/.solai to a temp directory for the duration of the test.
func useTempHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

// ---- DefaultConfig --------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.CycleInterval != "5m" {
		t.Errorf("CycleInterval: got %q, want %q", cfg.CycleInterval, "5m")
	}
	if !cfg.Sandbox.ShareNet {
		t.Error("Sandbox.ShareNet: expected true by default")
	}
	if cfg.Sandbox.ExtraBinds == nil {
		t.Error("Sandbox.ExtraBinds: expected non-nil slice")
	}
	if cfg.Providers == nil {
		t.Error("Providers: expected non-nil map")
	}
	if cfg.Model.Provider != "" || cfg.Model.Name != "" {
		t.Error("Model: expected empty by default")
	}
}

// ---- Set ------------------------------------------------------------------

func TestSet_ModelProvider(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Set("model.provider", "openai"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model.Provider != "openai" {
		t.Errorf("got %q, want %q", cfg.Model.Provider, "openai")
	}
}

func TestSet_ModelName(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Set("model.name", "gpt-4o"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model.Name != "gpt-4o" {
		t.Errorf("got %q, want %q", cfg.Model.Name, "gpt-4o")
	}
}

func TestSet_Providers(t *testing.T) {
	tests := []struct {
		key      string
		provider string
	}{
		{"provider.google", "google"},
		{"provider.openai", "openai"},
		{"provider.anthropic", "anthropic"},
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			cfg := DefaultConfig()
			if err := cfg.Set(tc.key, "mykey"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Providers[tc.provider] != "mykey" {
				t.Errorf("Providers[%q] = %q, want %q", tc.provider, cfg.Providers[tc.provider], "mykey")
			}
		})
	}
}

func TestSet_WalletSeed(t *testing.T) {
	cfg := DefaultConfig()
	_ = cfg.Set("wallet-seed", "word1 word2")
	if cfg.WalletSeed != "word1 word2" {
		t.Errorf("got %q", cfg.WalletSeed)
	}
}

func TestSet_CycleInterval(t *testing.T) {
	cfg := DefaultConfig()
	_ = cfg.Set("cycle-interval", "10m")
	if cfg.CycleInterval != "10m" {
		t.Errorf("got %q", cfg.CycleInterval)
	}
}

func TestSet_UserGoals(t *testing.T) {
	cfg := DefaultConfig()
	_ = cfg.Set("user-goals", "monitor price")
	if cfg.UserGoals != "monitor price" {
		t.Errorf("got %q", cfg.UserGoals)
	}
}

func TestSet_SandboxShareNet(t *testing.T) {
	tests := []struct {
		value string
		want  bool
		err   bool
	}{
		{"true", true, false},
		{"false", false, false},
		{"1", true, false},
		{"0", false, false},
		{"yes", true, false},
		{"no", false, false},
		{"TRUE", true, false},
		{"FALSE", false, false},
		{"maybe", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			cfg := DefaultConfig()
			err := cfg.Set("sandbox.share-net", tc.value)
			if tc.err {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Sandbox.ShareNet != tc.want {
				t.Errorf("ShareNet: got %v, want %v", cfg.Sandbox.ShareNet, tc.want)
			}
		})
	}
}

func TestSet_UnknownKey(t *testing.T) {
	cfg := DefaultConfig()
	err := cfg.Set("does-not-exist", "value")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unknown config key") {
		t.Errorf("error should mention unknown key: %v", err)
	}
}

// ---- Get ------------------------------------------------------------------

func TestGet_AllKeys(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Model.Provider = "google"
	cfg.Model.Name = "gemini-2.5-pro"
	cfg.Providers["google"] = "gkey"
	cfg.Providers["openai"] = "okey"
	cfg.Providers["anthropic"] = "akey"
	cfg.WalletSeed = "seed phrase"
	cfg.CycleInterval = "15m"
	cfg.UserGoals = "do stuff"
	cfg.Sandbox.ShareNet = false

	tests := []struct{ key, want string }{
		{"model.provider", "google"},
		{"model.name", "gemini-2.5-pro"},
		{"provider.google", "gkey"},
		{"provider.openai", "okey"},
		{"provider.anthropic", "akey"},
		{"wallet-seed", "seed phrase"},
		{"cycle-interval", "15m"},
		{"user-goals", "do stuff"},
		{"sandbox.share-net", "false"},
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			got, err := cfg.Get(tc.key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGet_SandboxShareNet_True(t *testing.T) {
	cfg := DefaultConfig() // ShareNet defaults to true
	got, err := cfg.Get("sandbox.share-net")
	if err != nil {
		t.Fatal(err)
	}
	if got != "true" {
		t.Errorf("got %q, want %q", got, "true")
	}
}

func TestGet_UnknownKey(t *testing.T) {
	cfg := DefaultConfig()
	_, err := cfg.Get("no-such-key")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

// ---- Save / Load ----------------------------------------------------------

func TestSave_Load_RoundTrip(t *testing.T) {
	useTempHome(t)

	cfg := DefaultConfig()
	cfg.Model.Provider = "anthropic"
	cfg.Model.Name = "claude-opus-4-6"
	cfg.Providers["anthropic"] = "sk-ant-test"
	cfg.UserGoals = "analyze Solana DeFi"
	cfg.CycleInterval = "30m"
	cfg.Sandbox.ShareNet = false

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Model.Provider != cfg.Model.Provider {
		t.Errorf("Model.Provider: got %q, want %q", loaded.Model.Provider, cfg.Model.Provider)
	}
	if loaded.Model.Name != cfg.Model.Name {
		t.Errorf("Model.Name: got %q, want %q", loaded.Model.Name, cfg.Model.Name)
	}
	if loaded.Providers["anthropic"] != "sk-ant-test" {
		t.Errorf("Providers[anthropic]: got %q", loaded.Providers["anthropic"])
	}
	if loaded.UserGoals != cfg.UserGoals {
		t.Errorf("UserGoals: got %q, want %q", loaded.UserGoals, cfg.UserGoals)
	}
	if loaded.CycleInterval != "30m" {
		t.Errorf("CycleInterval: got %q", loaded.CycleInterval)
	}
	if loaded.Sandbox.ShareNet {
		t.Error("Sandbox.ShareNet: expected false")
	}
}

func TestLoad_CreatesDefaultWhenAbsent(t *testing.T) {
	useTempHome(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Defaults should be in place.
	if cfg.CycleInterval != "5m" {
		t.Errorf("CycleInterval: got %q", cfg.CycleInterval)
	}
	if !cfg.Sandbox.ShareNet {
		t.Error("Sandbox.ShareNet: expected true")
	}

	// config.json should now exist on disk.
	if _, err := os.Stat(ConfigPath()); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}

func TestLoad_ReturnsDefaultsForMissingFields(t *testing.T) {
	useTempHome(t)

	// Write a minimal config missing optional fields.
	partial := `{"model":{"provider":"google","name":"gemini-2.5-pro"}}`
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ConfigPath(), []byte(partial), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Model.Provider != "google" {
		t.Errorf("Model.Provider: got %q", cfg.Model.Provider)
	}
	// Defaults from DefaultConfig should apply for missing fields.
	if cfg.CycleInterval != "5m" {
		t.Errorf("CycleInterval: got %q, want 5m", cfg.CycleInterval)
	}
}

func TestLoad_RejectsInvalidJSON(t *testing.T) {
	useTempHome(t)

	if err := os.MkdirAll(Dir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ConfigPath(), []byte("{not valid json"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSave_WritesValidJSON(t *testing.T) {
	useTempHome(t)

	cfg := DefaultConfig()
	cfg.Model.Provider = "openai"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Errorf("saved config is not valid JSON: %v", err)
	}
}

// ---- Directory helpers ----------------------------------------------------

func TestDir_ContainsDotSolai(t *testing.T) {
	useTempHome(t)
	d := Dir()
	if !strings.HasSuffix(d, ".solai") {
		t.Errorf("Dir() = %q, expected to end with .solai", d)
	}
}

func TestToolsDir_UnderDir(t *testing.T) {
	useTempHome(t)
	if filepath.Dir(ToolsDir()) != Dir() {
		t.Errorf("ToolsDir() %q is not under Dir() %q", ToolsDir(), Dir())
	}
	if filepath.Base(ToolsDir()) != "tools" {
		t.Errorf("ToolsDir() base: got %q, want tools", filepath.Base(ToolsDir()))
	}
}

func TestConfigPath_IsJSONFile(t *testing.T) {
	useTempHome(t)
	if !strings.HasSuffix(ConfigPath(), ".json") {
		t.Errorf("ConfigPath() = %q, expected .json suffix", ConfigPath())
	}
	if filepath.Dir(ConfigPath()) != Dir() {
		t.Error("ConfigPath() should be inside Dir()")
	}
}
