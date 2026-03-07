package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/melodyogonna/solai/ratelimit"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	lctools "github.com/tmc/langchaingo/tools"
)

const (
	jupiterQuoteURL = "https://quote-api.jup.ag/v6/quote"
	jupiterSwapURL  = "https://quote-api.jup.ag/v6/swap"

	jupiterLiteQuoteURL = "https://lite-api.jup.ag/swap/v1/quote"
	jupiterLiteSwapURL  = "https://lite-api.jup.ag/swap/v1/swap"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// ---- Capability request helpers ---------------------------------------------

// requestCapability writes a capability request to stdout and reads the response
// from stdin using the shared encoder/decoder.
func requestCapability(enc *json.Encoder, dec *json.Decoder, capability, action, input string) (string, error) {
	req := capabilityRequest{
		Type:       "request",
		Capability: capability,
		Action:     action,
		Input:      input,
	}
	if err := enc.Encode(req); err != nil {
		return "", fmt.Errorf("writing capability request: %w", err)
	}
	var resp capabilityResponse
	if err := dec.Decode(&resp); err != nil {
		return "", fmt.Errorf("reading capability response: %w", err)
	}
	if resp.Error != "" {
		return "", fmt.Errorf("capability %q error: %s", capability, resp.Error)
	}
	return resp.Output, nil
}

// ---- Rate limiter -----------------------------------------------------------

func newJupiterLimiter() ratelimit.RateLimitStrategy {
	const defaultRPS = 1
	rps := defaultRPS
	if v := os.Getenv("JUPITER_RATE_LIMIT_RPS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			rps = n
		}
	}
	return ratelimit.NewFixedWindowLimiter(rps, time.Second)
}

func jupiterBaseURLs() (quoteURL, swapURL string) {
	if os.Getenv("JUPITER_API_KEY") != "" {
		return jupiterQuoteURL, jupiterSwapURL
	}
	return jupiterLiteQuoteURL, jupiterLiteSwapURL
}

// ---- Jupiter quote tool (internal agent tool) -------------------------------

type jupiterQuoteTool struct {
	limiter  ratelimit.RateLimitStrategy
	quoteURL string
}

func (t *jupiterQuoteTool) Name() string { return "jupiter-quote" }

func (t *jupiterQuoteTool) Description() string {
	return `Get the best swap route and price quote from Jupiter aggregator.
Input JSON: {"inputMint":"<mint>","outputMint":"<mint>","amount":<integer_base_units>,"slippageBps":<int>}
- amount: integer in the input token's smallest unit (lamports for SOL = 1e9, 1e6 for USDC/USDT/JUP)
- slippageBps: optional, default 50 (0.5%)
Returns: quote with expected output amount, price impact, and route details.`
}

func (t *jupiterQuoteTool) Call(ctx context.Context, input string) (string, error) {
	var req struct {
		InputMint   string `json:"inputMint"`
		OutputMint  string `json:"outputMint"`
		Amount      int64  `json:"amount"`
		SlippageBps int    `json:"slippageBps"`
	}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", fmt.Errorf("invalid input JSON: %w", err)
	}
	if req.InputMint == "" || req.OutputMint == "" || req.Amount <= 0 {
		return "", fmt.Errorf("inputMint, outputMint, and amount (>0) are required")
	}
	if req.SlippageBps == 0 {
		req.SlippageBps = 50
	}

	if err := t.limiter.Wait(ctx); err != nil {
		return "", fmt.Errorf("rate limiter: %w", err)
	}

	url := fmt.Sprintf("%s?inputMint=%s&outputMint=%s&amount=%d&slippageBps=%d",
		t.quoteURL, req.InputMint, req.OutputMint, req.Amount, req.SlippageBps)

	body, err := doGet(ctx, url)
	if err != nil {
		return "", fmt.Errorf("Jupiter quote: %w", err)
	}
	return string(body), nil
}

// ---- Jupiter swap-tx tool (internal agent tool) -----------------------------

type jupiterSwapTxTool struct {
	limiter       ratelimit.RateLimitStrategy
	swapURL       string
	walletAddress string
}

func (t *jupiterSwapTxTool) Name() string { return "jupiter-swap-tx" }

func (t *jupiterSwapTxTool) Description() string {
	return `Get the serialized swap transaction from Jupiter for a previously obtained quote.
Input: the complete JSON response from the jupiter-quote tool (pass it verbatim).
Returns JSON with "swapTransaction" field containing a base64-encoded unsigned transaction.
The coordinator will sign and submit this transaction via the Solana capability.`
}

func (t *jupiterSwapTxTool) Call(ctx context.Context, input string) (string, error) {
	if t.walletAddress == "" {
		return "", fmt.Errorf("wallet address not available — cannot build swap transaction")
	}

	// Validate that input looks like a Jupiter quote response.
	var quote map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &quote); err != nil {
		return "", fmt.Errorf("input must be a valid Jupiter quote JSON: %w", err)
	}
	if _, ok := quote["inputMint"]; !ok {
		return "", fmt.Errorf("input does not look like a Jupiter quote response (missing inputMint)")
	}

	if err := t.limiter.Wait(ctx); err != nil {
		return "", fmt.Errorf("rate limiter: %w", err)
	}

	swapReq := swapRequest{
		QuoteResponse:             json.RawMessage(input),
		UserPublicKey:             t.walletAddress,
		DynamicComputeUnitLimit:   true,
		PrioritizationFeeLamports: "auto",
	}
	reqBody, err := json.Marshal(swapReq)
	if err != nil {
		return "", fmt.Errorf("marshalling swap request: %w", err)
	}

	apiKey := os.Getenv("JUPITER_API_KEY")
	body, err := doPost(ctx, t.swapURL, reqBody, apiKey)
	if err != nil {
		return "", fmt.Errorf("Jupiter swap: %w", err)
	}

	var resp swapResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parsing swap response: %w", err)
	}
	if resp.SwapTransaction == "" {
		return "", fmt.Errorf("Jupiter swap API returned empty transaction: %s", body)
	}

	out, _ := json.Marshal(map[string]string{
		"swapTransaction": resp.SwapTransaction,
		"note":            "Pass swapTransaction to the solana capability send_transaction action to sign and submit.",
	})
	return string(out), nil
}

// ---- main -------------------------------------------------------------------

func main() {
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(os.Stdout)

	var input ToolInput
	if err := dec.Decode(&input); err != nil {
		writeErrorEnc(enc, fmt.Sprintf("failed to read input: %v", err))
		return
	}

	ctx := context.Background()

	// Request wallet address if the coordinator has the wallet capability.
	walletAddress := ""
	if strings.Contains(input.AvailableCapabilities, "wallet") {
		addr, err := requestCapability(enc, dec, "wallet", "address", "")
		if err != nil {
			// Non-fatal: we can still return a quote without the transaction.
			fmt.Fprintf(os.Stderr, "wallet capability request failed: %v\n", err)
		} else {
			walletAddress = addr
		}
	}

	llm, err := newLLM(ctx)
	if err != nil {
		writeErrorEnc(enc, fmt.Sprintf("LLM initialisation failed: %v", err))
		return
	}

	quoteURL, swapURL := jupiterBaseURLs()
	limiter := newJupiterLimiter()

	tools := []lctools.Tool{
		&jupiterQuoteTool{limiter: limiter, quoteURL: quoteURL},
		&jupiterSwapTxTool{limiter: limiter, swapURL: swapURL, walletAddress: walletAddress},
	}
	agentInst := agents.NewOneShotAgent(llm, tools,
		agents.WithMaxIterations(6),
		agents.WithPromptPrefix(buildSystemPrompt(walletAddress)),
	)
	executor := agents.NewExecutor(agentInst)

	prompt := fmt.Sprintf("Overview: %s\nTasks:\n- %s",
		input.Overview, strings.Join(input.Tasks, "\n- "))

	result, err := chains.Run(ctx, executor, prompt)
	if err != nil {
		writeErrorEnc(enc, fmt.Sprintf("agent run failed: %v", err))
		return
	}

	out, _ := json.Marshal(result)
	enc.Encode(ToolOutput{Type: "success", Output: out}) //nolint:errcheck
}

// ---- HTTP helpers -----------------------------------------------------------

func doGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if key := os.Getenv("JUPITER_API_KEY"); key != "" {
		req.Header.Set("x-api-key", key)
	}
	return doRequest(req)
}

func doPost(ctx context.Context, url string, body []byte, apiKey string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	return doRequest(req)
}

func doRequest(req *http.Request) ([]byte, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}
	return b, nil
}

func writeErrorEnc(enc *json.Encoder, msg string) {
	out, _ := json.Marshal(msg)
	enc.Encode(ToolOutput{Type: "error", Output: out}) //nolint:errcheck
}
