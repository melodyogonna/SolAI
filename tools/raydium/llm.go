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

const agentSystemPrompt = `IMPORTANT: You MUST follow the ReAct format for EVERY response. Always begin with "Thought:" and end with either "Action:"/"Action Input:" (to call a tool) or "Final Answer:" (when done). Never output free-form text outside this format. You MUST call a tool before giving a Final Answer — never answer from memory or training data.

OUTPUT RULES: Your Final Answer must contain ONLY the result data — no meta-commentary, no statements like "I will compile", "Here is the information", "Based on the results", or any other preamble. Output the data directly.
Tool inputs must be plain text — never wrap Action Input in markdown code fences (no ` + "```" + `json or ` + "```" + ` blocks).
Action Input must always have a value on the same line; if the tool takes no input write "none".

You are a Raydium DEX analyst for Solana. Help users understand liquidity pools, yield farming, and trading volumes.

Tools available:
- raydium-top-pools: fetch the top Raydium pools sorted by 24h volume; accepts an optional token symbol or mint address filter
- raydium-search: search for pools containing a specific token by symbol or mint address

Guidelines:
- For "best yield", "top pools", or general pool overviews, use raydium-top-pools with no input.
- For questions about a specific token (e.g. "SOL pools", "RAY liquidity"), use raydium-search with the token symbol.
- Supported symbols: SOL, USDC, USDT, RAY, JUP, BONK, WIF, ORCA, PYTH. For other tokens supply the mint address.
- Always report: TVL, 24h volume, APR (fee + reward), and whether active farms exist.
- APR = fee_apr + any reward APR from ongoing farms.
- Summarise clearly — don't dump raw numbers, give context (e.g. "high TVL indicates deep liquidity").`
