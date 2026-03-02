package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// ToolInput mirrors the structure the main SolAI agent writes to our stdin.
type ToolInput struct {
	Overview string   `json:"overview"`
	Tasks    []string `json:"tasks"`
}

// ToolOutput mirrors the structure the main SolAI agent reads from our stdout.
type ToolOutput struct {
	Type   string          `json:"type"`
	Output json.RawMessage `json:"output"`
}

// TokenPrice is one entry in a successful price response.
type TokenPrice struct {
	Symbol    string  `json:"symbol"`
	Mint      string  `json:"mint"`
	PriceUSD  float64 `json:"price_usd"`
	Change24h float64 `json:"change_24h"`
}

// PriceResult is the full success output written to stdout.
type PriceResult struct {
	Prices    []TokenPrice `json:"prices"`
	Timestamp string       `json:"timestamp"`
}

// jupiterEntry matches a single token entry in the Jupiter Price V3 response.
type jupiterEntry struct {
	UsdPrice    float64 `json:"usdPrice"`
	PriceChange float64 `json:"priceChange24h"`
}

// knownTokens maps uppercase symbols to their Solana mint addresses.
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

// mintToSymbol is the reverse map for labelling API responses.
var mintToSymbol = func() map[string]string {
	m := make(map[string]string, len(knownTokens))
	for sym, mint := range knownTokens {
		m[mint] = sym
	}
	return m
}()

// symbolRegexes matches each known symbol as a whole word (case-insensitive).
var symbolRegexes = func() map[string]*regexp.Regexp {
	m := make(map[string]*regexp.Regexp, len(knownTokens))
	for sym := range knownTokens {
		m[sym] = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(sym) + `\b`)
	}
	return m
}()

// mintRegex matches a raw Solana base58 mint address.
var mintRegex = regexp.MustCompile(`[1-9A-HJ-NP-Za-km-z]{32,44}`)

const (
	// liteBaseURL requires no API key. If JUPITER_API_KEY is set, apiBaseURL is used instead.
	liteBaseURL = "https://lite-api.jup.ag/price/v3"
	apiBaseURL  = "https://api.jup.ag/price/v3"
	httpTimeout = 10 * time.Second
)

func main() {
	var input ToolInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		writeError(fmt.Sprintf("failed to read input: %v", err))
		return
	}

	mints := extractMints(input)
	if len(mints) == 0 {
		supported := make([]string, 0, len(knownTokens))
		for sym := range knownTokens {
			supported = append(supported, sym)
		}
		writeError("no token symbols or mint addresses found in input — supported symbols: " +
			strings.Join(supported, ", "))
		return
	}

	prices, err := fetchPrices(mints)
	if err != nil {
		writeError(fmt.Sprintf("price fetch failed: %v", err))
		return
	}
	if len(prices) == 0 {
		writeError("Jupiter returned no price data for the requested tokens")
		return
	}

	result, _ := json.Marshal(PriceResult{
		Prices:    prices,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
	writeSuccess(result)
}

// extractMints scans the input text for known token symbols and bare mint
// addresses, returning a deduplicated list of mint addresses to query.
func extractMints(input ToolInput) []string {
	text := input.Overview + " " + strings.Join(input.Tasks, " ")

	seen := make(map[string]bool)
	var mints []string

	add := func(mint string) {
		if !seen[mint] {
			seen[mint] = true
			mints = append(mints, mint)
		}
	}

	for sym, mint := range knownTokens {
		if symbolRegexes[sym].MatchString(text) {
			add(mint)
		}
	}

	for _, match := range mintRegex.FindAllString(text, -1) {
		// Only treat it as a mint address if it's not already covered by a symbol.
		if _, isSymbolMint := mintToSymbol[match]; !isSymbolMint {
			add(match)
		}
	}

	return mints
}

// fetchPrices calls the Jupiter Price V3 API and returns prices for the given mints.
func fetchPrices(mints []string) ([]TokenPrice, error) {
	baseURL := liteBaseURL
	apiKey := os.Getenv("JUPITER_API_KEY")
	if apiKey != "" {
		baseURL = apiBaseURL
	}

	reqURL := baseURL + "?ids=" + strings.Join(mints, ",")
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var jupResp map[string]jupiterEntry
	if err := json.Unmarshal(body, &jupResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w\nbody: %s", err, string(body))
	}

	prices := make([]TokenPrice, 0, len(mints))
	for _, mint := range mints {
		entry, ok := jupResp[mint]
		if !ok {
			continue
		}
		sym := mintToSymbol[mint]
		if sym == "" {
			// Unknown mint — use a truncated address as the display name.
			sym = mint[:8] + "…"
		}
		prices = append(prices, TokenPrice{
			Symbol:    sym,
			Mint:      mint,
			PriceUSD:  entry.UsdPrice,
			Change24h: entry.PriceChange,
		})
	}
	return prices, nil
}

func writeSuccess(data json.RawMessage) {
	json.NewEncoder(os.Stdout).Encode(ToolOutput{Type: "success", Output: data})
}

func writeError(msg string) {
	errMsg, _ := json.Marshal(msg)
	json.NewEncoder(os.Stdout).Encode(ToolOutput{Type: "error", Output: errMsg})
}
