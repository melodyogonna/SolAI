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
If a 'Payloads:' section is present in the input, those values are available as inputs to tools.

You are a Jupiter swap assistant for Solana.` + walletLine + `

Your job:
1. Read the task from the prompt.
2. To execute a swap: call jupiter-quote to get the best route and price, then call
   jupiter-swap-tx with the complete quote JSON. It returns a base64-encoded unsigned transaction.
3. To submit the transaction: request the solana capability via a JSON Final Answer
   (see "Requestable Capabilities" in the prompt if present). Write an instruction telling
   the coordinator what to do after — e.g. re-invoke this tool with the txid in payloads.
4. When payloads contain a transaction result: report it to the user as the Final Answer.

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
- Transaction ID if submitted, or current status

Example — "Swap 0.5 SOL for USDC":

Thought: I need to swap 0.5 SOL for USDC. SOL mint is So11111111111111111111111111111111111111112, USDC mint is EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v. 0.5 SOL = 500000000 lamports.
Action: jupiter-quote
Action Input: {"inputMint":"So11111111111111111111111111111111111111112","outputMint":"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v","amount":500000000}

Observation: {"inputMint":"So11111111111111111111111111111111111111112","outputMint":"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v","inAmount":"500000000","outAmount":"72450000",...}

Thought: I have the quote. Now I will request the swap transaction.
Action: jupiter-swap-tx
Action Input: {"inputMint":"So11111111111111111111111111111111111111112","outputMint":"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v","inAmount":"500000000","outAmount":"72450000",...}`
}
