package tool

import (
	"testing"
	"time"

	"github.com/melodyogonna/solai/solai-agent/capability"
)

func makeManifest(name, desc, executable, timeout string) Manifest {
	return Manifest{
		Name:        name,
		Description: desc,
		Executable:  executable,
		Timeout:     timeout,
	}
}

// ---- NewAgenticTool — timeout parsing ---------------------------------------

func TestNewAgenticTool_DefaultTimeout_WhenEmpty(t *testing.T) {
	m := makeManifest("my-tool", "desc", "./bin/my-tool", "")
	at := NewAgenticTool(m, "/tools/my-tool", nil, SandboxPolicy{}, nil, capability.NewCommunicationCapability(), "")
	if at.timeout != DefaultToolTimeout {
		t.Errorf("timeout: got %v, want %v", at.timeout, DefaultToolTimeout)
	}
}

func TestNewAgenticTool_ParsesValidTimeout(t *testing.T) {
	m := makeManifest("my-tool", "desc", "./bin/my-tool", "90s")
	at := NewAgenticTool(m, "/tools/my-tool", nil, SandboxPolicy{}, nil, capability.NewCommunicationCapability(), "")
	if at.timeout != 90*time.Second {
		t.Errorf("timeout: got %v, want 90s", at.timeout)
	}
}

func TestNewAgenticTool_DefaultTimeout_WhenInvalid(t *testing.T) {
	m := makeManifest("my-tool", "desc", "./bin/my-tool", "notaduration")
	at := NewAgenticTool(m, "/tools/my-tool", nil, SandboxPolicy{}, nil, capability.NewCommunicationCapability(), "")
	if at.timeout != DefaultToolTimeout {
		t.Errorf("timeout: got %v, want default %v", at.timeout, DefaultToolTimeout)
	}
}

func TestNewAgenticTool_DefaultTimeout_WhenZero(t *testing.T) {
	m := makeManifest("my-tool", "desc", "./bin/my-tool", "0s")
	at := NewAgenticTool(m, "/tools/my-tool", nil, SandboxPolicy{}, nil, capability.NewCommunicationCapability(), "")
	if at.timeout != DefaultToolTimeout {
		t.Errorf("timeout: got %v, want default %v (zero duration should fall back)", at.timeout, DefaultToolTimeout)
	}
}

func TestNewAgenticTool_ParsesMinuteTimeout(t *testing.T) {
	m := makeManifest("my-tool", "desc", "./bin/my-tool", "2m")
	at := NewAgenticTool(m, "/tools/my-tool", nil, SandboxPolicy{}, nil, capability.NewCommunicationCapability(), "")
	if at.timeout != 2*time.Minute {
		t.Errorf("timeout: got %v, want 2m", at.timeout)
	}
}

// ---- Name / Description / llmCfg --------------------------------------------

func TestAgenticTool_Name(t *testing.T) {
	m := makeManifest("token-price", "desc", "./bin/token-price", "")
	at := NewAgenticTool(m, "/tools/token-price", nil, SandboxPolicy{}, nil, capability.NewCommunicationCapability(), "")
	if at.Name() != "token-price" {
		t.Errorf("Name: got %q, want token-price", at.Name())
	}
}

func TestAgenticTool_Description(t *testing.T) {
	m := makeManifest("token-price", "Fetches USD prices for Solana tokens.", "./bin/token-price", "")
	at := NewAgenticTool(m, "/tools/token-price", nil, SandboxPolicy{}, nil, capability.NewCommunicationCapability(), "")
	if at.Description() != "Fetches USD prices for Solana tokens." {
		t.Errorf("Description: got %q", at.Description())
	}
}

func TestAgenticTool_WithLLMConfig(t *testing.T) {
	m := makeManifest("my-tool", "desc", "./bin/my-tool", "")
	llmCfg := &capability.LLMConfig{Provider: "google", Model: "gemini-2.5-pro", APIKey: "key"}
	at := NewAgenticTool(m, "/tools/my-tool", llmCfg, SandboxPolicy{}, nil, capability.NewCommunicationCapability(), "")
	if at.llmCfg == nil {
		t.Fatal("expected llmCfg to be set")
	}
	if at.llmCfg.Provider != "google" {
		t.Errorf("llmCfg.Provider: got %q", at.llmCfg.Provider)
	}
}

func TestAgenticTool_NilLLMConfig(t *testing.T) {
	m := makeManifest("my-tool", "desc", "./bin/my-tool", "")
	at := NewAgenticTool(m, "/tools/my-tool", nil, SandboxPolicy{}, nil, capability.NewCommunicationCapability(), "")
	if at.llmCfg != nil {
		t.Error("expected llmCfg to be nil")
	}
}

// ---- DefaultToolTimeout value -----------------------------------------------

func TestDefaultToolTimeout_IsAtLeastOneMinute(t *testing.T) {
	if DefaultToolTimeout < time.Minute {
		t.Errorf("DefaultToolTimeout is %v, expected at least 1 minute for LLM subagent calls", DefaultToolTimeout)
	}
}
