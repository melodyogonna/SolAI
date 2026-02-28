package main

import (
	"context"
	"fmt"
	"log"

	"github.com/melodyogonna/solai/solai-agent/wallet"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/tools"
)

type agentConfig struct {
	model        llms.Model
	systemPrompt []byte
	wallet       *wallet.SolKeyPair
}

func run(ctx context.Context, config agentConfig) {
	for {
		agent := agents.NewOneShotAgent(config.model, []tools.Tool{}, agents.WithMaxIterations(3))
		executor := agents.NewExecutor(agent)
		answer, err := chains.Run(ctx, executor, string(config.systemPrompt))
		if err != nil {
			log.Print(err)
		}
		fmt.Print(answer)
	}
}
