package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/joho/godotenv/autoload"
	"github.com/melodyogonna/solai/solai-agent/agent"
	"github.com/melodyogonna/solai/solai-agent/capability"
	"github.com/tmc/langchaingo/llms/googleai"
)

func main() {
	cfg, err := agent.LoadConfig()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	ctx := context.Background()
	llm, err := googleai.New(ctx,
		googleai.WithAPIKey(os.Getenv("API_KEY")),
		googleai.WithDefaultModel("gemini-2.5-pro"),
	)
	if err != nil {
		log.Fatalf("failed to initialize LLM: %v", err)
	}
	cfg.LLM = llm

	capability.Register("wallet", func() capability.Capability {
		return capability.NewWalletCapability(cfg.Wallet)
	})
	capability.Register("network-manager", func() capability.Capability {
		return capability.NewNetworkManagerCapability()
	})
	capManager := capability.SetUp([]string{"wallet", "network-manager"})

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("SolAI agent starting",
		"model", "gemini-2.5-pro",
		"toolsDir", cfg.ToolsDir,
		"cycleInterval", cfg.CycleInterval,
		"wallet", cfg.Wallet.Base58PubKey(),
	)

	agent.Run(ctx, cfg, capManager)

	slog.Info("SolAI agent stopped")
}
