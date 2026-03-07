package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/openai"
)

func newLLM(ctx context.Context) (llms.Model, error) {
	provider := os.Getenv("SOLAI_LLM_PROVIDER")
	model := os.Getenv("SOLAI_LLM_MODEL")
	apiKey := os.Getenv("SOLAI_LLM_API_KEY")

	if provider == "" || apiKey == "" {
		return nil, fmt.Errorf("SOLAI_LLM_PROVIDER and SOLAI_LLM_API_KEY must be set")
	}

	switch provider {
	case "google":
		opts := []googleai.Option{googleai.WithAPIKey(apiKey)}
		if model != "" {
			opts = append(opts, googleai.WithDefaultModel(model))
		}
		return googleai.New(ctx, opts...)

	case "openai":
		opts := []openai.Option{openai.WithToken(apiKey)}
		if model != "" {
			opts = append(opts, openai.WithModel(model))
		}
		return openai.New(opts...)

	case "anthropic":
		opts := []anthropic.Option{anthropic.WithToken(apiKey)}
		if model != "" {
			opts = append(opts, anthropic.WithModel(model))
		}
		return anthropic.New(opts...)

	default:
		return nil, fmt.Errorf("unsupported LLM provider %q (supported: google, openai, anthropic)", provider)
	}
}

const agentSystemPrompt = `You are a Raydium DEX analyst for Solana. Help users understand liquidity pools, yield farming, and trading volumes.

Tools available:
- raydium-top-pools: fetch the top Raydium pools sorted by TVL; accepts an optional token symbol/address filter
- raydium-search: search for specific pools by token pair, name, or mint address

Guidelines:
- For "best yield" or "top pools" queries, use raydium-top-pools.
- For questions about a specific token pair, use raydium-search.
- Always report: TVL, 24h volume, APR (fee + reward), and whether active farms exist.
- APR = fee_apr + any reward APR from ongoing farms.
- Summarise clearly — don't dump raw numbers, give context (e.g. "high TVL indicates deep liquidity").`
