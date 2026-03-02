package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/melodyogonna/solai/solai-agent/agent"
	"github.com/melodyogonna/solai/solai-agent/capability"
	solaiconfig "github.com/melodyogonna/solai/solai-agent/config"
	"github.com/melodyogonna/solai/solai-agent/prompts"
	"github.com/spf13/cobra"
	"github.com/tmc/langchaingo/llms/googleai"
)

// agentRunCmd is the hidden subcommand invoked inside the bwrap sandbox by
// "solai start". It is not shown in help output.
var agentRunCmd = &cobra.Command{
	Use:    "__agent-run",
	Hidden: true,
	RunE:   runAgentRunCmd,
}

func init() {
	rootCmd.AddCommand(agentRunCmd)
}

// runAgentRunCmd is the cobra handler for __agent-run.
// Inside the bwrap sandbox, config is at /etc/solai/config.json and tools at /tools/.
// SOLAI_CONFIG_PATH overrides the config path (useful for --no-sandbox debug path).
func runAgentRunCmd(cmd *cobra.Command, args []string) error {
	configPath := "/etc/solai/config.json"
	if v := os.Getenv("SOLAI_CONFIG_PATH"); v != "" {
		configPath = v
	}
	toolsDir := "/tools"

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading config from %s: %w", configPath, err)
	}
	cfg := solaiconfig.DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	return agentRun(cmd.Context(), cfg, toolsDir)
}

// agentRun initializes and runs the autonomous agent loop.
// It is called by the __agent-run subcommand (inside bwrap) and by
// "solai start --no-sandbox" (directly on the host).
func agentRun(ctx context.Context, cfg *solaiconfig.SolaiConfig, toolsDir string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	llm, err := googleai.New(ctx,
		googleai.WithAPIKey(cfg.APIKey),
		googleai.WithDefaultModel("gemini-2.5-pro"),
	)
	if err != nil {
		return fmt.Errorf("initializing LLM: %w", err)
	}

	agentCfg, err := agent.LoadConfigFrom(cfg, toolsDir, prompts.SystemPrompt)
	if err != nil {
		return fmt.Errorf("building agent config: %w", err)
	}
	agentCfg.LLM = llm

	capability.Register("wallet", func() capability.Capability {
		return capability.NewWalletCapability(agentCfg.Wallet)
	})
	capability.Register("network-manager", func() capability.Capability {
		return capability.NewNetworkManagerCapability()
	})
	capManager := capability.SetUp([]string{"wallet", "network-manager"})

	slog.Info("SolAI agent starting",
		"model", "gemini-2.5-pro",
		"toolsDir", toolsDir,
		"cycleInterval", agentCfg.CycleInterval,
		"wallet", agentCfg.Wallet.Base58PubKey(),
	)

	agent.Run(ctx, agentCfg, capManager)
	slog.Info("SolAI agent stopped")
	return nil
}
