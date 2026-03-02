package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/melodyogonna/solai/solai-agent/sandbox"
	lctools "github.com/tmc/langchaingo/tools"
)

// ToolLoaderFunc loads agentic tools. Injected into SystemManager to avoid
// circular imports between capability and tool packages.
//
// bwrapPath is the path to the extracted bwrap binary, or empty when sandboxing
// is unavailable. checker allows the loader to validate required_capabilities
// declared by each tool against the agent's registered Regular capabilities.
type ToolLoaderFunc func(bwrapPath string, checker CapabilityChecker) ([]lctools.Tool, []error, error)

// CleanupJob is a periodic administrative task run by SystemManager in the background.
type CleanupJob struct {
	Name     string
	Interval time.Duration
	Fn       func(ctx context.Context) error
}

// SystemManager is a Core capability that owns the agent's operational environment:
// tool loading, LLM provider logging, sandbox extraction, and cleanup job scheduling.
type SystemManager struct {
	loader   ToolLoaderFunc
	provider *LLMProvider
	tools    []lctools.Tool
	jobs     []CleanupJob
	ready    bool
	bwrapPath string // extracted bwrap binary path; empty if sandbox unavailable
}

// NewSystemManager creates a SystemManager with the given tool loader and LLM provider.
// The provider is used only for startup logging in Setup.
// Call Setup before GetTools, and optionally RegisterJob before Start.
func NewSystemManager(loader ToolLoaderFunc, provider *LLMProvider) *SystemManager {
	return &SystemManager{loader: loader, provider: provider}
}

// Name implements Capability.
func (m *SystemManager) Name() string { return "system-manager" }

// Class implements Capability — Core class, never injected into prompts.
func (m *SystemManager) Class() CapabilityClass { return Core }

// Description implements Capability — empty because Core capabilities are invisible.
func (m *SystemManager) Description() string { return "" }

// Execute returns a JSON status report of loaded tools and registered jobs.
// Useful for future diagnostic tooling; safe to call before or after Setup.
func (m *SystemManager) Execute(_ context.Context, _ string) (string, error) {
	toolNames := make([]string, len(m.tools))
	for i, t := range m.tools {
		toolNames[i] = t.Name()
	}

	jobNames := make([]string, len(m.jobs))
	for i, j := range m.jobs {
		jobNames[i] = j.Name
	}

	status := map[string]any{
		"tools_loaded":      len(m.tools),
		"tools":             toolNames,
		"jobs_registered":   len(m.jobs),
		"jobs":              jobNames,
		"sandbox_available": m.bwrapPath != "",
	}
	data, err := json.Marshal(status)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Setup prepares the agent's operational environment:
//  1. Logs configured LLM providers.
//  2. Extracts the embedded bwrap sandbox binary.
//  3. Loads tools via the injected loader, passing the bwrap path and checker.
//
// checker is used to validate required_capabilities in tool manifests; pass the
// CapabilityManager built in main.go. checker must not be nil.
//
// Returns (warnings, fatal_error). Must be called before GetTools.
//
// If the SANDBOX environment variable is set to "required", Setup returns a
// fatal error when the bwrap binary cannot be extracted. Otherwise the agent
// falls back to running tools unsandboxed with a warning.
func (m *SystemManager) Setup(checker CapabilityChecker) ([]error, error) {
	if providers := m.provider.ConfiguredProviders(); len(providers) > 0 {
		slog.Info("LLM providers configured", "providers", providers)
	} else {
		slog.Warn("no LLM providers configured — tools requiring an LLM will be disabled")
	}

	bwrapPath, err := sandbox.Extract()
	if err != nil {
		if os.Getenv("SANDBOX") == "required" {
			return nil, fmt.Errorf("sandbox extraction failed (SANDBOX=required): %w", err)
		}
		slog.Warn("sandbox not available; tools will run unsandboxed", "err", err)
	} else {
		m.bwrapPath = bwrapPath
		slog.Info("sandbox binary extracted", "path", bwrapPath, "version", sandbox.BwrapVersion)
		// Clean up the temp binary when the agent exits via a registered job
		// that fires once on first tick. A more direct approach is to register
		// the cleanup with the context; we do it here via a one-shot job so
		// the temp file is removed when Start(ctx) returns.
	}

	tools, warnings, err := m.loader(m.bwrapPath, checker)
	if err != nil {
		return nil, err
	}
	m.tools = tools
	m.ready = true
	return warnings, nil
}

// GetTools returns tools loaded by Setup. Panics if Setup was not called first.
func (m *SystemManager) GetTools() []lctools.Tool {
	if !m.ready {
		panic("system_manager: GetTools called before Setup")
	}
	return m.tools
}

// BwrapPath returns the path to the extracted sandbox binary, or empty string
// if the sandbox is not available. Useful for diagnostics.
func (m *SystemManager) BwrapPath() string { return m.bwrapPath }

// RegisterJob appends a cleanup job. Must be called before Start.
func (m *SystemManager) RegisterJob(job CleanupJob) {
	m.jobs = append(m.jobs, job)
}

// Start launches one goroutine per registered job and blocks until ctx is cancelled,
// then waits for all goroutines to exit. Designed to be called as go sm.Start(ctx).
// When ctx is cancelled, any extracted sandbox binary temp file is removed.
func (m *SystemManager) Start(ctx context.Context) {
	var wg sync.WaitGroup
	for _, job := range m.jobs {
		wg.Add(1)
		go func(j CleanupJob) {
			defer wg.Done()
			m.runJob(ctx, j)
		}(job)
	}
	wg.Wait()

	// Remove the extracted bwrap temp file on shutdown.
	if m.bwrapPath != "" {
		if err := os.Remove(m.bwrapPath); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove sandbox binary temp file", "path", m.bwrapPath, "err", err)
		} else {
			slog.Debug("sandbox binary temp file removed", "path", m.bwrapPath)
		}
	}
}

// runJob runs a single cleanup job on its interval until ctx is cancelled.
// Errors from the job are logged as warnings and the job keeps running.
func (m *SystemManager) runJob(ctx context.Context, job CleanupJob) {
	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := job.Fn(ctx); err != nil {
				slog.Warn("cleanup job error", "job", job.Name, "err", err)
			}
		}
	}
}
