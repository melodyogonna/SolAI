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

// newLLM creates a langchaingo LLM from the SOLAI_LLM_* env vars injected
// by the coordinator.
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

// ---- Internal agent system prompt ----------------------------------------

const agentSystemPrompt = `You are a Solana token price assistant. Your only job is to fetch current
USD prices for tokens the user asks about, using the jupiter-price tool.

Rules:
- Always use the jupiter-price tool to get prices — never guess or make up prices.
- If the user asks about "all tokens" or "major tokens", fetch: SOL, USDC, USDT, JUP, BONK.
- Only look up tokens that are explicitly requested or clearly implied.
- Report prices clearly with the token symbol, USD price, and 24h change.`
