package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/melodyogonna/solai/ratelimit"
	"github.com/melodyogonna/solai/solai-agent/capability"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
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

	cyclePrompt := buildCyclePrompt(cfg, capManager)

	for {
		select {
		case <-ctx.Done():
			slog.Info("agent shutting down")
			return
		default:
		}

		slog.Info("starting agent cycle")
		cycleCtx, cancel := context.WithTimeout(ctx, cfg.CycleTimeout)
		answer, err := runCycle(cycleCtx, cfg, agentTools, cyclePrompt)
		cancel()

		if err != nil {
			handleCycleError(err)
		} else {
			slog.Info("cycle complete", "answer", answer)
		}
	}
}

// runCycle executes one complete ReAct cycle with exponential retry on transient errors.
// It creates a fresh agent and executor each attempt to avoid state bleed.
// Non-transient errors (ErrNotFinished, context cancellation/timeout) are not retried.
func runCycle(ctx context.Context, cfg Config, agentTools []lctools.Tool, prompt string) (string, error) {
	var answer string
	err := cycleRetry.Execute(ctx, func(ctx context.Context) error {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		a := agents.NewOneShotAgent(
			cfg.LLM,
			agentTools,
			agents.WithMaxIterations(10),
			agents.WithPromptPrefix(cfg.SystemPrompt),
		)
		executor := agents.NewExecutor(a)
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
// It combines the capability section (e.g. wallet address) with the user's goals.
// This is the "Question" the ReAct agent receives; the system prompt is the "Prefix".
func buildCyclePrompt(cfg Config, capManager *capability.CapabilityManager) string {
	capSection := capManager.BuildCapabilityPromptSection()
	if capSection != "" {
		return fmt.Sprintf("## System Capabilities\n%s\n\n## Your Goals\n%s", capSection, cfg.UserGoals)
	}
	return fmt.Sprintf("## Your Goals\n%s", cfg.UserGoals)
}

// handleCycleError logs cycle errors with appropriate severity.
// The agent loop always continues after an error — it never crashes.
func handleCycleError(err error) {
	switch {
	case errors.Is(err, agents.ErrNotFinished):
		slog.Warn("cycle did not finish within max iterations — tools may be insufficient for the specified goals")
	case errors.Is(err, context.DeadlineExceeded):
		slog.Warn("cycle timed out")
	default:
		slog.Error("unexpected cycle error", "err", err)
	}
}
