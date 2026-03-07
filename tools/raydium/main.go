package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	lctools "github.com/tmc/langchaingo/tools"
)

const raydiumPoolsBase = "https://api-v3.raydium.io/pools/info"

var httpClient = &http.Client{Timeout: 15 * time.Second}

// ---- raydium-top-pools tool -------------------------------------------------

type topPoolsTool struct{}

func (t *topPoolsTool) Name() string { return "raydium-top-pools" }

func (t *topPoolsTool) Description() string {
	return `Fetch the top Raydium liquidity pools sorted by TVL.
Input: optional token symbol or mint address to filter results (e.g. "SOL", "USDC", or empty for top pools overall).
Returns: list of pools with TVL, 24h volume, APR (fee + reward), and farming status.`
}

func (t *topPoolsTool) Call(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)

	var apiURL string
	if input != "" {
		apiURL = raydiumPoolsBase + "/search?q=" + url.QueryEscape(input) +
			"&poolType=all&poolSortField=tvl&sortType=desc&pageSize=15&page=1"
	} else {
		apiURL = raydiumPoolsBase + "/list?poolType=all&poolSortField=tvl&sortType=desc&pageSize=15&page=1"
	}

	pools, err := fetchPools(ctx, apiURL)
	if err != nil {
		return "", err
	}

	summaries := summarisePools(pools, 15)
	data, _ := json.Marshal(summaries)
	return string(data), nil
}

// ---- raydium-search tool ----------------------------------------------------

type searchPoolsTool struct{}

func (t *searchPoolsTool) Name() string { return "raydium-search" }

func (t *searchPoolsTool) Description() string {
	return `Search Raydium for pools matching a specific token pair, symbol, or mint address.
Input: search query (e.g. "SOL-USDC", "RAY", "4k3Dyjz...").
Returns: matching pools with TVL, APR, volume, and farm details.`
}

func (t *searchPoolsTool) Call(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("search query is required")
	}

	apiURL := raydiumPoolsBase + "/search?q=" + url.QueryEscape(input) +
		"&poolType=all&poolSortField=tvl&sortType=desc&pageSize=10&page=1"

	pools, err := fetchPools(ctx, apiURL)
	if err != nil {
		return "", err
	}

	summaries := summarisePools(pools, 10)
	data, _ := json.Marshal(summaries)
	return string(data), nil
}

// ---- main -------------------------------------------------------------------

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

	tools := []lctools.Tool{
		&topPoolsTool{},
		&searchPoolsTool{},
	}
	agentInst := agents.NewOneShotAgent(llm, tools,
		agents.WithMaxIterations(6),
		agents.WithPromptPrefix(agentSystemPrompt),
	)
	executor := agents.NewExecutor(agentInst)

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

// ---- Helpers ----------------------------------------------------------------

func fetchPools(ctx context.Context, apiURL string) ([]PoolInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Raydium API request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Raydium API returned HTTP %d: %s", resp.StatusCode, body)
	}

	var apiResp poolsAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing Raydium response: %w", err)
	}
	if !apiResp.Success {
		return nil, fmt.Errorf("Raydium API returned success=false")
	}
	return apiResp.Data.Data, nil
}

func summarisePools(pools []PoolInfo, n int) []PoolSummary {
	summaries := make([]PoolSummary, 0, n)
	for _, p := range pools {
		if len(summaries) == n {
			break
		}
		totalAPR := p.Day.APR
		s := PoolSummary{
			PoolID:       p.ID,
			Type:         p.Type,
			TokenA:       symbolOrShort(p.MintA),
			TokenB:       symbolOrShort(p.MintB),
			Price:        p.Price,
			TVLUSD:       p.TVL,
			Volume24hUSD: p.Day.VolumeUSD,
			APR24h:       totalAPR,
			FeeAPR24h:    p.Day.FeeAPR,
			ActiveFarms:  p.FarmCount,
			FeeRate:      p.FeeRate * 100,
		}
		summaries = append(summaries, s)
	}
	return summaries
}

func symbolOrShort(m TokenMint) string {
	if m.Symbol != "" {
		return m.Symbol
	}
	if len(m.Address) >= 8 {
		return m.Address[:8] + "…"
	}
	return m.Address
}

func writeSuccess(data json.RawMessage) {
	json.NewEncoder(os.Stdout).Encode(ToolOutput{Type: "success", Output: data})
}

func writeError(msg string) {
	out, _ := json.Marshal(msg)
	json.NewEncoder(os.Stdout).Encode(ToolOutput{Type: "error", Output: out})
}
