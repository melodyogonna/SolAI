package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/melodyogonna/solai/ratelimit"
	"github.com/melodyogonna/solai/solai-agent/capability"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/schema"
	lctools "github.com/tmc/langchaingo/tools"
)

// cycleRetry is the retry strategy used for each agent cycle.
// It retries up to 3 times with exponential backoff starting at 2s, capped at 30s.
var cycleRetry = ratelimit.NewExponentialRetry(3, 2*time.Second, 30*time.Second, 2.0)

// Run is the main entry point for the autonomous agent loop.
// It blocks until ctx is cancelled, running one agent cycle per CycleInterval.
//
// On startup it loads agentic tools from cfg.ToolsDir, then enters a loop where
// each iteration runs a full ReAct cycle using the configured LLM and tools.
func Run(ctx context.Context, cfg Config, capManager *capability.CapabilityManager) {
	warnings, err := cfg.SystemManager.Setup(capManager)
	if err != nil {
		slog.Error("system manager setup failed", "err", err)
		return
	}
	for _, w := range warnings {
		slog.Warn("tool setup warning", "err", w)
	}
	go cfg.SystemManager.Start(ctx)

	agentTools := append(capManager.GetAgentTools(), cfg.SystemManager.GetTools()...)
	if len(agentTools) == 0 {
		slog.Warn("no tools loaded — agent will report it cannot accomplish goals")
	} else {
		slog.Info("tools loaded", "count", len(agentTools))
		for _, t := range agentTools {
			slog.Info("tool available", "name", t.Name())
		}
	}

	windowMem := memory.NewConversationWindowBuffer(3)

	var cycleNum int
	for {
		select {
		case <-ctx.Done():
			slog.Info("agent shutting down")
			return
		default:
		}

		cycleNum++
		slog.Info("starting agent cycle", "cycle", cycleNum)
		cycleCtx, cancel := context.WithTimeout(ctx, cfg.CycleTimeout)
		cyclePrompt := buildCyclePrompt(cfg, capManager, cfg.SystemManager.GetTools())
		slog.Debug("cycle prompt", "cycle", cycleNum, "prompt", cyclePrompt)
		answer, err := runCycle(cycleCtx, cfg, agentTools, cyclePrompt, windowMem)
		cancel()

		if err != nil {
			handleCycleError(cycleNum, err)
		} else {
			slog.Info("cycle complete", "cycle", cycleNum, "answer", answer)
		}
	}
}

// runCycle executes one complete ReAct cycle with exponential retry on transient errors.
// It creates a fresh agent each attempt but shares the window memory across retries and cycles.
// Non-transient errors (ErrNotFinished, context cancellation/timeout) are not retried.
func runCycle(ctx context.Context, cfg Config, agentTools []lctools.Tool, prompt string, mem schema.Memory) (string, error) {
	var answer string
	attempt := 0
	err := cycleRetry.Execute(ctx, func(ctx context.Context) error {
		attempt++
		if attempt > 1 {
			slog.Info("retrying cycle", "attempt", attempt)
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		a := agents.NewOneShotAgent(
			cfg.LLM,
			agentTools,
			agents.WithMaxIterations(10),
			agents.WithPromptPrefix(cfg.SystemPrompt),
		)
		executor := agents.NewExecutor(a, agents.WithMemory(mem))
		var err error
		answer, err = chains.Run(ctx, executor, prompt)
		if errors.Is(err, agents.ErrNotFinished) {
			// Not a transient error — stop retrying immediately.
			return &noRetryError{err}
		}
		return err
	})
	var nre *noRetryError
	if errors.As(err, &nre) {
		return "", nre.err
	}
	return answer, err
}

// noRetryError wraps an error to signal that cycleRetry should not attempt further retries.
type noRetryError struct{ err error }

func (e *noRetryError) Error() string { return e.err.Error() }

// buildCyclePrompt assembles the input string passed to chains.Run each cycle.
// It lists ALL available tools (capabilities + agentic tools) so the agent has
// a complete, consistent view before seeing the langchaingo ReAct tool list.
// It also injects any MemorySectionProvider sections (e.g. MemoryCapability).
// This is the "Question" the ReAct agent receives; the system prompt is the "Prefix".
func buildCyclePrompt(cfg Config, capManager *capability.CapabilityManager, agenticTools []lctools.Tool) string {
	var sections []string
	var toolSection strings.Builder
	toolSection.WriteString("## Available Tools\n")

	if capSection := capManager.BuildCapabilityPromptSection(); capSection != "" {
		toolSection.WriteString("\n### Built-in Capabilities\n")
		toolSection.WriteString(capSection)
	}
	if len(agenticTools) > 0 {
		toolSection.WriteString("\n### Agentic Tools\n")
		for _, t := range agenticTools {
			fmt.Fprintf(&toolSection, "- **%s**: %s\n", t.Name(), t.Description())
		}
	}
	if toolSection.Len() > len("## Available Tools\n") {
		sections = append(sections, strings.TrimRight(toolSection.String(), "\n"))
	}

	for _, c := range capManager.GetAll() {
		if mp, ok := c.(capability.MemorySectionProvider); ok {
			if sec := mp.BuildMemorySection(); sec != "" {
				sections = append(sections, sec)
			}
		}
	}

	sections = append(sections, "## Your Goals\n"+cfg.UserGoals)
	return strings.Join(sections, "\n\n")
}

// handleCycleError logs cycle errors with appropriate severity.
// The agent loop always continues after an error — it never crashes.
func handleCycleError(cycleNum int, err error) {
	switch {
	case errors.Is(err, agents.ErrNotFinished):
		slog.Warn("cycle did not finish within max iterations — tools may be insufficient for the specified goals", "cycle", cycleNum)
	case errors.Is(err, context.DeadlineExceeded):
		slog.Warn("cycle timed out", "cycle", cycleNum)
	default:
		slog.Error("unexpected cycle error", "cycle", cycleNum, "err", err)
	}
}
