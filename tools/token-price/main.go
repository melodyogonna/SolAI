package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/openai"
	lctools "github.com/tmc/langchaingo/tools"
)

// ---- IPC types (coordinator ↔ subagent contract) -------------------------

type ToolInput struct {
	Overview string   `json:"overview"`
	Tasks    []string `json:"tasks"`
}

type ToolOutput struct {
	Type   string          `json:"type"`
	Output json.RawMessage `json:"output"`
}

// ---- Known tokens ---------------------------------------------------------

var knownTokens = map[string]string{
	"SOL":  "So11111111111111111111111111111111111111112",
	"USDC": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
	"USDT": "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB",
	"JUP":  "JUPyiwrYJFskUPiHa7hkeR8VUtAeFoSYbKedZNsDvCN",
	"BONK": "DezXAZ8z7PnrnRJjz3wXBoRgixCa6xjnB7YaB1pPB263",
	"WIF":  "EKpQGSJtjMFqKZ9KQanSqYXRcF8fBopzLHYxdM65zcjm",
	"PYTH": "HZ1JovNiVvGrGNiiYvEozEVgZ58xaU3RKwX8eACQBCt3",
	"RAY":  "4k3Dyjzvzp8eMZWUXbBCjEvwSkkk59S5iCNLY3QrkX6R",
}

var mintToSymbol = func() map[string]string {
	m := make(map[string]string, len(knownTokens))
	for sym, mint := range knownTokens {
		m[mint] = sym
	}
	return m
}()

// ---- Jupiter price tool (internal langchaingo tool) ----------------------

// jupiterTool is a langchaingo tool the internal agent calls to fetch live
// Solana token prices from Jupiter's price API.
type jupiterTool struct{}

func (t *jupiterTool) Name() string { return "jupiter-price" }

func (t *jupiterTool) Description() string {
	syms := make([]string, 0, len(knownTokens))
	for s := range knownTokens {
		syms = append(syms, s)
	}
	return fmt.Sprintf(
		`Fetches current USD prices and 24h change for Solana tokens from Jupiter.
Input: comma-separated token symbols (%s) or raw Solana mint addresses.
Example input: "SOL,USDC,JUP"
Returns JSON with price_usd and change_24h_pct for each token.`,
		strings.Join(syms, ", "),
	)
}

func (t *jupiterTool) Call(_ context.Context, input string) (string, error) {
	mints := resolveToMints(strings.Split(input, ","))
	if len(mints) == 0 {
		return "", fmt.Errorf("no recognised token symbols or mint addresses in: %q", input)
	}

	prices, err := fetchPrices(mints)
	if err != nil {
		return "", err
	}

	data, err := json.Marshal(prices)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---- LLM initialisation ---------------------------------------------------

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

// ---- Entry point ----------------------------------------------------------

func main() {
	var input ToolInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		writeError(fmt.Sprintf("failed to read input: %v", err))
		return
	}

	ctx := context.Background()

	llm, err := newLLM(ctx)
	if err != nil {
		writeError(fmt.Sprintf("LLM initialisation failed: %v", err))
		return
	}

	tools := []lctools.Tool{&jupiterTool{}}
	agent := agents.NewOneShotAgent(llm, tools,
		agents.WithMaxIterations(5),
		agents.WithPromptPrefix(agentSystemPrompt),
	)
	executor := agents.NewExecutor(agent)

	prompt := fmt.Sprintf("Overview: %s\nTasks:\n- %s",
		input.Overview, strings.Join(input.Tasks, "\n- "))

	result, err := chains.Run(ctx, executor, prompt)
	if err != nil {
		writeError(fmt.Sprintf("agent run failed: %v", err))
		return
	}

	out, _ := json.Marshal(result)
	writeSuccess(out)
}

// ---- Helpers --------------------------------------------------------------

// resolveToMints converts a list of token symbols or raw mint addresses into
// mint addresses, skipping unrecognised entries.
func resolveToMints(tokens []string) []string {
	seen := make(map[string]bool)
	var mints []string
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		upper := strings.ToUpper(t)
		if mint, ok := knownTokens[upper]; ok {
			if !seen[mint] {
				seen[mint] = true
				mints = append(mints, mint)
			}
		} else if len(t) >= 32 && len(t) <= 44 {
			if !seen[t] {
				seen[t] = true
				mints = append(mints, t)
			}
		}
	}
	return mints
}

type jupiterEntry struct {
	UsdPrice    float64 `json:"usdPrice"`
	PriceChange float64 `json:"priceChange24h"`
}

type TokenPrice struct {
	Symbol      string  `json:"symbol"`
	Mint        string  `json:"mint"`
	PriceUSD    float64 `json:"price_usd"`
	Change24hPct float64 `json:"change_24h_pct"`
}

func fetchPrices(mints []string) ([]TokenPrice, error) {
	baseURL := "https://lite-api.jup.ag/price/v3"
	apiKey := os.Getenv("JUPITER_API_KEY")
	if apiKey != "" {
		baseURL = "https://api.jup.ag/price/v3"
	}

	req, err := http.NewRequest(http.MethodGet, baseURL+"?ids="+strings.Join(mints, ","), nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Jupiter API returned HTTP %d: %s", resp.StatusCode, body)
	}

	var jupResp map[string]jupiterEntry
	if err := json.Unmarshal(body, &jupResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	prices := make([]TokenPrice, 0, len(mints))
	for _, mint := range mints {
		entry, ok := jupResp[mint]
		if !ok {
			continue
		}
		sym := mintToSymbol[mint]
		if sym == "" {
			sym = mint[:8] + "…"
		}
		prices = append(prices, TokenPrice{
			Symbol:       sym,
			Mint:         mint,
			PriceUSD:     entry.UsdPrice,
			Change24hPct: entry.PriceChange,
		})
	}
	return prices, nil
}

func writeSuccess(data json.RawMessage) {
	json.NewEncoder(os.Stdout).Encode(ToolOutput{Type: "success", Output: data})
}

func writeError(msg string) {
	out, _ := json.Marshal(msg)
	json.NewEncoder(os.Stdout).Encode(ToolOutput{Type: "error", Output: out})
}
