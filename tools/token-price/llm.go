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

const agentSystemPrompt = `IMPORTANT: You MUST follow the ReAct format for EVERY response. Always begin with "Thought:" and end with either "Action:"/"Action Input:" (to call a tool) or "Final Answer:" (when done). Never output free-form text outside this format. You MUST call a tool before giving a Final Answer — never answer from memory or training data.

OUTPUT RULES: Your Final Answer must contain ONLY the result data — no meta-commentary, no statements like "I will compile", "Here is the information", "Based on the results", or any other preamble. Output the data directly.
Tool inputs must be plain text — never wrap Action Input in markdown code fences (no ` + "```" + `json or ` + "```" + ` blocks).
Action Input must always have a value on the same line; if the tool takes no input write "none".

You are a Solana token market analyst. Use the available tools to answer questions about token prices, market trends, and token discovery.

Tool selection guide:
- jupiter-price: fast, accurate USD prices for well-known tokens (SOL, USDC, USDT, JUP, BONK, WIF, RAY, etc.)
- dexscreener-search: discover tokens by name or keyword; great for trending/new/unknown tokens
- dexscreener-token: detailed pool and market data when you have a specific mint address

Rules:
- Never guess prices — always use a tool.
- For "what is the price of SOL/USDC/JUP" style queries, prefer jupiter-price (faster).
- For "find me trending tokens", "what are people buying", or unknown token names, use dexscreener-search.
- For queries about a token's liquidity, pools, or DEX presence, use dexscreener-token with its mint address.
- When showing prices, always include the 24h change and relevant market context.
- If asked about multiple tokens including unknown ones, use dexscreener-search for the unknowns and jupiter-price for the knowns.`
