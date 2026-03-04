package capability

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	lctools "github.com/tmc/langchaingo/tools"
)

// noopChecker is a CapabilityChecker that always returns false.
type noopChecker struct{}

func (noopChecker) IsRegularCapabilityAvailable(_ string) bool { return false }

// allChecker is a CapabilityChecker that always returns true.
type allChecker struct{}

func (allChecker) IsRegularCapabilityAvailable(_ string) bool { return true }

// stubTool implements lctools.Tool for testing.
type stubTool struct{ name string }

func (s *stubTool) Name() string                                      { return s.name }
func (s *stubTool) Description() string                               { return s.name }
func (s *stubTool) Call(_ context.Context, _ string) (string, error) { return "", nil }

// ---- Name / Class / Description ---------------------------------------------

func TestSystemManager_Metadata(t *testing.T) {
	sm := NewSystemManager(nil, NewLLMProviderFromMap(nil))
	if sm.Name() != "system-manager" {
		t.Errorf("Name: got %q, want %q", sm.Name(), "system-manager")
	}
	if sm.Class() != Core {
		t.Errorf("Class: got %v, want Core", sm.Class())
	}
	if sm.Description() != "" {
		t.Errorf("Description: expected empty for Core capability, got %q", sm.Description())
	}
}

// ---- BwrapPath --------------------------------------------------------------

func TestSystemManager_BwrapPathInitiallyEmpty(t *testing.T) {
	sm := NewSystemManager(nil, NewLLMProviderFromMap(nil))
	if sm.BwrapPath() != "" {
		t.Errorf("expected empty bwrap path before Setup, got %q", sm.BwrapPath())
	}
}

// ---- GetTools panics before Setup -------------------------------------------

func TestGetTools_PanicsIfSetupNotCalled(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when GetTools called before Setup")
		}
	}()
	sm := NewSystemManager(nil, NewLLMProviderFromMap(nil))
	sm.GetTools() // must panic
}

// ---- Execute ----------------------------------------------------------------

func TestExecute_BeforeSetup_ReturnsEmptyTools(t *testing.T) {
	sm := NewSystemManager(nil, NewLLMProviderFromMap(nil))
	result, err := sm.Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(result), &status); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if status["tools_loaded"].(float64) != 0 {
		t.Errorf("expected 0 tools loaded, got %v", status["tools_loaded"])
	}
}

func TestExecute_AfterSetup_ReflectsLoadedTools(t *testing.T) {
	tools := []lctools.Tool{
		&stubTool{name: "tool-a"},
		&stubTool{name: "tool-b"},
	}
	loader := func(_ string, _ CapabilityChecker) ([]lctools.Tool, []error, error) {
		return tools, nil, nil
	}
	sm := NewSystemManager(loader, NewLLMProviderFromMap(nil))
	_, err := sm.Setup(noopChecker{})
	if err != nil {
		// Setup may fail due to sandbox extraction; that's acceptable in tests.
		t.Logf("Setup warning (sandbox unavailable): %v", err)
	}

	result, err := sm.Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(result), &status); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	if int(status["tools_loaded"].(float64)) != len(tools) {
		t.Errorf("tools_loaded: got %v, want %d", status["tools_loaded"], len(tools))
	}
}

func TestExecute_SandboxAvailableField(t *testing.T) {
	sm := NewSystemManager(
		func(_ string, _ CapabilityChecker) ([]lctools.Tool, []error, error) {
			return nil, nil, nil
		},
		NewLLMProviderFromMap(nil),
	)
	result, err := sm.Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(result), &status); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if _, ok := status["sandbox_available"]; !ok {
		t.Error("expected sandbox_available field in Execute output")
	}
}

// ---- Setup ------------------------------------------------------------------

func TestSetup_LoaderFatalError_Propagated(t *testing.T) {
	loader := func(_ string, _ CapabilityChecker) ([]lctools.Tool, []error, error) {
		return nil, nil, errors.New("fatal loader error")
	}
	sm := NewSystemManager(loader, NewLLMProviderFromMap(nil))
	_, err := sm.Setup(noopChecker{})
	if err == nil {
		t.Fatal("expected fatal error from loader to be propagated")
	}
}

func TestSetup_LoaderWarnings_Returned(t *testing.T) {
	loader := func(_ string, _ CapabilityChecker) ([]lctools.Tool, []error, error) {
		return nil, []error{errors.New("tool-x disabled")}, nil
	}
	sm := NewSystemManager(loader, NewLLMProviderFromMap(nil))
	warnings, err := sm.Setup(noopChecker{})
	if err != nil {
		t.Logf("Setup error (may be sandbox): %v", err)
		return
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(warnings))
	}
}

func TestSetup_GetToolsAfterSetup(t *testing.T) {
	expected := []lctools.Tool{&stubTool{name: "my-tool"}}
	loader := func(_ string, _ CapabilityChecker) ([]lctools.Tool, []error, error) {
		return expected, nil, nil
	}
	sm := NewSystemManager(loader, NewLLMProviderFromMap(nil))
	if _, err := sm.Setup(noopChecker{}); err != nil {
		t.Logf("Setup error (may be sandbox): %v", err)
	}
	tools := sm.GetTools()
	if len(tools) != len(expected) {
		t.Errorf("GetTools: got %d tools, want %d", len(tools), len(expected))
	}
}

// ---- RegisterJob / Start ----------------------------------------------------

func TestRegisterJob_RunsOnTick(t *testing.T) {
	sm := NewSystemManager(nil, NewLLMProviderFromMap(nil))

	ran := make(chan struct{}, 1)
	sm.RegisterJob(CleanupJob{
		Name:     "test-job",
		Interval: 10 * time.Millisecond,
		Fn: func(_ context.Context) error {
			select {
			case ran <- struct{}{}:
			default:
			}
			return nil
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go sm.Start(ctx)

	select {
	case <-ran:
		// job ran at least once
	case <-ctx.Done():
		t.Error("job did not run within timeout")
	}
}

func TestRegisterJob_StopsOnContextCancel(t *testing.T) {
	sm := NewSystemManager(nil, NewLLMProviderFromMap(nil))

	count := 0
	sm.RegisterJob(CleanupJob{
		Name:     "count-job",
		Interval: 10 * time.Millisecond,
		Fn: func(_ context.Context) error {
			count++
			return nil
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Start blocks until context is done.
	sm.Start(ctx)
	// After Start returns, no more increments should happen.
	before := count
	time.Sleep(20 * time.Millisecond)
	if count != before {
		t.Errorf("job ran after context was cancelled: count changed from %d to %d", before, count)
	}
}
