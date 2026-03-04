package agent

import (
	"testing"
	"time"

	solaiconfig "github.com/melodyogonna/solai/solai-agent/config"
)

// ---- parseDuration ----------------------------------------------------------

func TestParseDuration_ValidString(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"5m", 5 * time.Minute},
		{"1h", time.Hour},
		{"30s", 30 * time.Second},
		{"2h30m", 2*time.Hour + 30*time.Minute},
		{"100ms", 100 * time.Millisecond},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := parseDuration(tc.input, time.Second)
			if got != tc.want {
				t.Errorf("parseDuration(%q): got %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseDuration_EmptyString_ReturnsDefault(t *testing.T) {
	defaultVal := 5 * time.Minute
	if got := parseDuration("", defaultVal); got != defaultVal {
		t.Errorf("got %v, want %v", got, defaultVal)
	}
}

func TestParseDuration_InvalidString_ReturnsDefault(t *testing.T) {
	defaultVal := 10 * time.Minute
	for _, invalid := range []string{"notaduration", "5", "2x", "forever"} {
		if got := parseDuration(invalid, defaultVal); got != defaultVal {
			t.Errorf("parseDuration(%q): got %v, want default %v", invalid, got, defaultVal)
		}
	}
}

// ---- LoadConfigFrom validation ----------------------------------------------

func defaultSolaiConfig() *solaiconfig.SolaiConfig {
	cfg := solaiconfig.DefaultConfig()
	cfg.Model.Provider = "google"
	cfg.Model.Name = "gemini-2.5-pro"
	cfg.Providers["google"] = "test-api-key"
	cfg.UserGoals = "Monitor SOL price"
	return cfg
}

func TestLoadConfigFrom_ValidConfig(t *testing.T) {
	cfg := defaultSolaiConfig()
	agentCfg, err := LoadConfigFrom(cfg, t.TempDir(), "system prompt")
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}
	if agentCfg.SystemPrompt != "system prompt" {
		t.Errorf("SystemPrompt: got %q", agentCfg.SystemPrompt)
	}
	if agentCfg.UserGoals != "Monitor SOL price" {
		t.Errorf("UserGoals: got %q", agentCfg.UserGoals)
	}
	if agentCfg.Wallet == nil {
		t.Error("expected Wallet to be non-nil")
	}
	if agentCfg.LLMProvider == nil {
		t.Error("expected LLMProvider to be non-nil")
	}
	if agentCfg.SystemManager == nil {
		t.Error("expected SystemManager to be non-nil")
	}
}

func TestLoadConfigFrom_EmptyModelProvider_Error(t *testing.T) {
	cfg := defaultSolaiConfig()
	cfg.Model.Provider = ""
	_, err := LoadConfigFrom(cfg, t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error when model.provider is empty")
	}
}

func TestLoadConfigFrom_EmptyModelName_Error(t *testing.T) {
	cfg := defaultSolaiConfig()
	cfg.Model.Name = ""
	_, err := LoadConfigFrom(cfg, t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error when model.name is empty")
	}
}

func TestLoadConfigFrom_MissingProviderKey_Error(t *testing.T) {
	cfg := defaultSolaiConfig()
	cfg.Providers["google"] = "" // clear the api key
	_, err := LoadConfigFrom(cfg, t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error when provider API key is empty")
	}
}

func TestLoadConfigFrom_MismatchedProvider_Error(t *testing.T) {
	cfg := defaultSolaiConfig()
	cfg.Model.Provider = "openai"
	// google key is set but openai is not.
	_, err := LoadConfigFrom(cfg, t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error when selected provider has no key")
	}
}

func TestLoadConfigFrom_CycleInterval_Parsed(t *testing.T) {
	cfg := defaultSolaiConfig()
	cfg.CycleInterval = "10m"
	agentCfg, err := LoadConfigFrom(cfg, t.TempDir(), "")
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}
	if agentCfg.CycleInterval != 10*time.Minute {
		t.Errorf("CycleInterval: got %v, want 10m", agentCfg.CycleInterval)
	}
}

func TestLoadConfigFrom_CycleInterval_DefaultWhenEmpty(t *testing.T) {
	cfg := defaultSolaiConfig()
	cfg.CycleInterval = ""
	agentCfg, err := LoadConfigFrom(cfg, t.TempDir(), "")
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}
	if agentCfg.CycleInterval != 5*time.Minute {
		t.Errorf("CycleInterval: got %v, want default 5m", agentCfg.CycleInterval)
	}
}

func TestLoadConfigFrom_WalletSeed_GeneratesWallet(t *testing.T) {
	cfg := defaultSolaiConfig()
	cfg.WalletSeed = "" // empty → generate new wallet
	agentCfg, err := LoadConfigFrom(cfg, t.TempDir(), "")
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}
	if agentCfg.Wallet == nil {
		t.Fatal("expected wallet to be generated")
	}
	pubKey := agentCfg.Wallet.Base58PubKey()
	if pubKey == "" {
		t.Error("expected non-empty public key from generated wallet")
	}
}

func TestLoadConfigFrom_ToolsDirInjected(t *testing.T) {
	cfg := defaultSolaiConfig()
	toolsDir := t.TempDir()
	agentCfg, err := LoadConfigFrom(cfg, toolsDir, "")
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}
	if agentCfg.ToolsDir != toolsDir {
		t.Errorf("ToolsDir: got %q, want %q", agentCfg.ToolsDir, toolsDir)
	}
}

func TestLoadConfigFrom_LLMProviderReflectsCredentials(t *testing.T) {
	cfg := defaultSolaiConfig()
	cfg.Providers["openai"] = "okey"
	agentCfg, err := LoadConfigFrom(cfg, t.TempDir(), "")
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}
	if !agentCfg.LLMProvider.IsConfigured("google") {
		t.Error("expected google to be configured in LLMProvider")
	}
	if !agentCfg.LLMProvider.IsConfigured("openai") {
		t.Error("expected openai to be configured in LLMProvider")
	}
}
