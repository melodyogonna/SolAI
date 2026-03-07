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

// buildSystemPrompt returns the agent system prompt with the wallet address embedded.
func buildSystemPrompt(walletAddress string) string {
	walletLine := ""
	if walletAddress != "" {
		walletLine = "\nUser wallet address: " + walletAddress
	}
	return `IMPORTANT: You MUST follow the ReAct format for EVERY response. Always begin with "Thought:" and end with either "Action:"/"Action Input:" (to call a tool) or "Final Answer:" (when done). Never output free-form text outside this format. You MUST call tools — never answer from memory.

OUTPUT RULES: Your Final Answer must contain ONLY the result data — no meta-commentary, no statements like "I will compile", "Here is the information", "Based on the results", or any other preamble. Output the data directly.
Tool inputs must be plain text — never wrap Action Input in markdown code fences (no ` + "```" + `json or ` + "```" + ` blocks).
Action Input must always have a value on the same line; if the tool takes no input write "none".

You are a Jupiter swap assistant for Solana.` + walletLine + `

Your job (two-phase flow):
1. Read the task from the "prompt" field of your input.
2. Call jupiter-quote to get the best route and price.
3. If a wallet address is available, call jupiter-swap-tx with the quote.
   jupiter-swap-tx will request a signing capability from the coordinator and exit —
   the coordinator will sign the transaction and re-invoke this tool with the signed
   transaction in the "payload" field.
4. On re-invocation with a signed transaction in "payload", the tool submits it
   directly — no further agent action is required.

Token mint addresses (use these for the tools):
- SOL:  So11111111111111111111111111111111111111112
- USDC: EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v
- USDT: Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB
- JUP:  JUPyiwrYJFskUPiHa7hkeR8VUtAeFoSYbKedZNsDvCN
- BONK: DezXAZ8z7PnrnRJjz3wXBoRgixCa6xjnB7YaB1pPB263
- RAY:  4k3Dyjzvzp8eMZWUXbBCjEvwSkkk59S5iCNLY3QrkX6R

Amount units (pass as integer base units to jupiter-quote):
- SOL:  multiply by 1000000000 (1e9 lamports per SOL)
- USDC: multiply by 1000000    (1e6 units per USDC)
- USDT: multiply by 1000000    (1e6 units per USDT)
- JUP:  multiply by 1000000    (1e6 units per JUP)
- For unknown tokens, use 1e9 as a safe default unless you know the decimals.

In your final answer, always state:
- Input: amount + token
- Output: expected amount + token
- Price impact and fees
- Whether the transaction has been submitted (txid) or is awaiting signing`
}
