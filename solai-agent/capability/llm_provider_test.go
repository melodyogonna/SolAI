package capability

import (
	"strings"
	"testing"
)

// ---- NewLLMProviderFromMap --------------------------------------------------

func TestNewLLMProviderFromMap_KnownProviders(t *testing.T) {
	p := NewLLMProviderFromMap(map[string]string{
		"google":    "gkey",
		"openai":    "okey",
		"anthropic": "akey",
	})

	for _, name := range []string{"google", "openai", "anthropic"} {
		if !p.IsConfigured(name) {
			t.Errorf("expected %q to be configured", name)
		}
	}
}

func TestNewLLMProviderFromMap_UnknownProviderIgnored(t *testing.T) {
	p := NewLLMProviderFromMap(map[string]string{
		"unknown-provider": "key",
		"google":           "gkey",
	})
	// Only google should be present; "unknown-provider" is silently dropped.
	if !p.IsConfigured("google") {
		t.Error("google should be configured")
	}
	if p.IsConfigured("unknown-provider") {
		t.Error("unknown-provider should be ignored")
	}
}

func TestNewLLMProviderFromMap_EmptyValueIgnored(t *testing.T) {
	p := NewLLMProviderFromMap(map[string]string{
		"google": "",
		"openai": "okey",
	})
	if p.IsConfigured("google") {
		t.Error("google with empty key should not be configured")
	}
	if !p.IsConfigured("openai") {
		t.Error("openai should be configured")
	}
}

func TestNewLLMProviderFromMap_NilMap(t *testing.T) {
	p := NewLLMProviderFromMap(nil)
	for _, name := range []string{"google", "openai", "anthropic"} {
		if p.IsConfigured(name) {
			t.Errorf("%q should not be configured with nil map", name)
		}
	}
}

// ---- IsConfigured -----------------------------------------------------------

func TestIsConfigured_CaseInsensitive(t *testing.T) {
	p := NewLLMProviderFromMap(map[string]string{"google": "gkey"})
	// knownProviders uses lowercase internally; IsConfigured normalises input.
	if !p.IsConfigured("google") {
		t.Error("expected google to be configured")
	}
	if !p.IsConfigured("Google") {
		t.Error("expected Google (mixed case) to match")
	}
}

func TestIsConfigured_Unconfigured(t *testing.T) {
	p := NewLLMProviderFromMap(nil)
	if p.IsConfigured("anthropic") {
		t.Error("expected anthropic not configured")
	}
}

// ---- ResolveForTool ---------------------------------------------------------

func TestResolveForTool_PrimaryPreferred(t *testing.T) {
	p := NewLLMProviderFromMap(map[string]string{
		"google": "gkey",
		"openai": "okey",
	})
	opts := []LLMModelOption{
		{Model: "gemini-2.5-pro", Provider: "google"},
		{Model: "gpt-4o", Provider: "openai"},
	}
	cfg := p.ResolveForTool("gemini-2.5-pro", opts)
	if cfg == nil {
		t.Fatal("expected non-nil LLMConfig")
	}
	if cfg.Provider != "google" {
		t.Errorf("expected primary provider google, got %q", cfg.Provider)
	}
	if cfg.Model != "gemini-2.5-pro" {
		t.Errorf("expected primary model gemini-2.5-pro, got %q", cfg.Model)
	}
	if cfg.APIKey != "gkey" {
		t.Errorf("expected gkey, got %q", cfg.APIKey)
	}
}

func TestResolveForTool_FallsBackToSupported(t *testing.T) {
	// Only openai is configured; primary is google.
	p := NewLLMProviderFromMap(map[string]string{
		"openai": "okey",
	})
	opts := []LLMModelOption{
		{Model: "gemini-2.5-pro", Provider: "google"},
		{Model: "gpt-4o", Provider: "openai"},
	}
	cfg := p.ResolveForTool("gemini-2.5-pro", opts)
	if cfg == nil {
		t.Fatal("expected non-nil LLMConfig from fallback")
	}
	if cfg.Provider != "openai" {
		t.Errorf("expected fallback provider openai, got %q", cfg.Provider)
	}
}

func TestResolveForTool_NoConfiguredProvider(t *testing.T) {
	p := NewLLMProviderFromMap(nil)
	opts := []LLMModelOption{
		{Model: "gemini-2.5-pro", Provider: "google"},
	}
	if got := p.ResolveForTool("gemini-2.5-pro", opts); got != nil {
		t.Errorf("expected nil when no provider configured, got %+v", got)
	}
}

func TestResolveForTool_PrimaryNotInSupported(t *testing.T) {
	// Primary is not in the supported list — falls back to first configured supported.
	p := NewLLMProviderFromMap(map[string]string{"openai": "okey"})
	opts := []LLMModelOption{
		{Model: "gpt-4o", Provider: "openai"},
	}
	cfg := p.ResolveForTool("non-existent-model", opts)
	if cfg == nil {
		t.Fatal("expected fallback to openai")
	}
	if cfg.Provider != "openai" {
		t.Errorf("expected openai, got %q", cfg.Provider)
	}
}

func TestResolveForTool_EmptySupportedList(t *testing.T) {
	p := NewLLMProviderFromMap(map[string]string{"google": "gkey"})
	cfg := p.ResolveForTool("gemini-2.5-pro", nil)
	if cfg != nil {
		t.Errorf("expected nil for empty supported list, got %+v", cfg)
	}
}

// ---- LLMConfig.Env ----------------------------------------------------------

func TestLLMConfigEnv(t *testing.T) {
	cfg := &LLMConfig{
		Provider: "google",
		Model:    "gemini-2.5-pro",
		APIKey:   "test-key",
	}
	env := cfg.Env()
	if len(env) != 3 {
		t.Fatalf("expected 3 env vars, got %d", len(env))
	}

	wantVars := map[string]string{
		"SOLAI_LLM_PROVIDER": "google",
		"SOLAI_LLM_MODEL":    "gemini-2.5-pro",
		"SOLAI_LLM_API_KEY":  "test-key",
	}
	for _, entry := range env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			t.Errorf("malformed env entry %q", entry)
			continue
		}
		want, ok := wantVars[parts[0]]
		if !ok {
			t.Errorf("unexpected env key %q", parts[0])
			continue
		}
		if parts[1] != want {
			t.Errorf("env %s: got %q, want %q", parts[0], parts[1], want)
		}
	}
}

// ---- ConfiguredProviders ----------------------------------------------------

func TestConfiguredProviders(t *testing.T) {
	p := NewLLMProviderFromMap(map[string]string{
		"google": "gkey",
		"openai": "okey",
	})
	names := p.ConfiguredProviders()
	if len(names) != 2 {
		t.Errorf("expected 2 providers, got %d", len(names))
	}
	found := make(map[string]bool)
	for _, n := range names {
		found[n] = true
	}
	if !found["google"] || !found["openai"] {
		t.Errorf("expected google and openai in ConfiguredProviders, got %v", names)
	}
}

func TestConfiguredProviders_Empty(t *testing.T) {
	p := NewLLMProviderFromMap(nil)
	if names := p.ConfiguredProviders(); len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}
}

// ---- SupportedProviderList --------------------------------------------------

func TestSupportedProviderList_Basic(t *testing.T) {
	opts := []LLMModelOption{
		{Model: "gemini-2.5-pro", Provider: "google"},
		{Model: "gpt-4o", Provider: "openai"},
	}
	got := SupportedProviderList(opts)
	if !strings.Contains(got, "gemini-2.5-pro (google)") {
		t.Errorf("expected gemini-2.5-pro (google) in %q", got)
	}
	if !strings.Contains(got, "gpt-4o (openai)") {
		t.Errorf("expected gpt-4o (openai) in %q", got)
	}
}

func TestSupportedProviderList_DeduplicatesProvider(t *testing.T) {
	opts := []LLMModelOption{
		{Model: "gemini-2.5-pro", Provider: "google"},
		{Model: "gemini-flash", Provider: "google"},
	}
	got := SupportedProviderList(opts)
	// google appears only once.
	if strings.Count(got, "google") != 1 {
		t.Errorf("expected google once, got %q", got)
	}
}

func TestSupportedProviderList_Empty(t *testing.T) {
	if got := SupportedProviderList(nil); got != "" {
		t.Errorf("expected empty string for nil input, got %q", got)
	}
}
