package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/melodyogonna/solai/solai-agent/capability"
	"github.com/melodyogonna/solai/solai-agent/tool"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	lctools "github.com/tmc/langchaingo/tools"
)

// Run is the main entry point for the autonomous agent loop.
// It blocks until ctx is cancelled, running one agent cycle per CycleInterval.
//
// On startup it loads agentic tools from cfg.ToolsDir, then enters a loop where
// each iteration runs a full ReAct cycle using the configured LLM and tools.
func Run(ctx context.Context, cfg Config, capManager *capability.CapabilityManager) {
	agentTools, warnings, err := tool.LoadTools(cfg.ToolsDir)
	if err != nil {
		slog.Error("cannot read tools directory, agent cannot start", "dir", cfg.ToolsDir, "err", err)
		return
	}
	for _, w := range warnings {
		slog.Warn("tool load warning", "err", w)
	}
	if len(agentTools) == 0 {
		slog.Warn("no agentic tools loaded — agent will report it cannot accomplish goals")
	} else {
		slog.Info("agentic tools loaded", "count", len(agentTools))
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
		// Give each cycle twice the interval as a hard deadline to prevent
		// a runaway cycle from blocking the next one indefinitely.
		cycleCtx, cancel := context.WithTimeout(ctx, cfg.CycleInterval*2)
		answer, err := runCycle(cycleCtx, cfg, agentTools, cyclePrompt)
		cancel()

		if err != nil {
			handleCycleError(err)
		} else {
			slog.Info("cycle complete", "answer", answer)
		}

		// Sleep for the cycle interval, but wake immediately if ctx is cancelled.
		select {
		case <-ctx.Done():
			slog.Info("agent shutting down")
			return
		case <-time.After(cfg.CycleInterval):
		}
	}
}

// runCycle executes one complete ReAct cycle.
// It creates a fresh agent and executor each cycle to avoid state bleed
// across cycles. Returns the agent's final answer or an error.
func runCycle(ctx context.Context, cfg Config, agentTools []lctools.Tool, prompt string) (string, error) {
	a := agents.NewOneShotAgent(
		cfg.LLM,
		agentTools,
		agents.WithMaxIterations(10),
		agents.WithPromptPrefix(cfg.SystemPrompt),
	)
	executor := agents.NewExecutor(a)
	return chains.Run(ctx, executor, prompt)
}

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
