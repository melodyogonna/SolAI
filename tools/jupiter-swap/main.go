package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"github.com/melodyogonna/solai/ratelimit"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	lctools "github.com/tmc/langchaingo/tools"
)

const (
	jupiterQuoteURL   = "https://quote-api.jup.ag/v6/quote"
	jupiterSwapURL    = "https://quote-api.jup.ag/v6/swap"
	jupiterExecuteURL = "https://quote-api.jup.ag/v6/execute"

	jupiterLiteQuoteURL   = "https://lite-api.jup.ag/swap/v1/quote"
	jupiterLiteSwapURL    = "https://lite-api.jup.ag/swap/v1/swap"
	jupiterLiteExecuteURL = "https://lite-api.jup.ag/swap/v1/execute"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

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

func jupiterBaseURLs() (quoteURL, swapURL, executeURL string) {
	if os.Getenv("JUPITER_API_KEY") != "" {
		return jupiterQuoteURL, jupiterSwapURL, jupiterExecuteURL
	}
	return jupiterLiteQuoteURL, jupiterLiteSwapURL, jupiterLiteExecuteURL
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
	input = stripMarkdownFence(input)
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
	input = stripMarkdownFence(input)
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

	// Write a "request" output asking the coordinator to sign the transaction,
	// then exit. The coordinator will re-invoke this tool with the signed tx
	// in input.Payload.
	capReq := CapabilityRequest{
		Capability:  "wallet",
		Action:      "sign",
		Input:       resp.SwapTransaction,
		Description: "Sign and submit the swap transaction",
	}
	payload, _ := json.Marshal(capReq)
	writeRequest(payload)
	os.Exit(0)
	return "", nil // unreachable
}

// ---- Jupiter submit-tx tool (phase 2) ---------------------------------------

type jupiterSubmitTxTool struct {
	executeURL string
}

// submitSignedTx posts a base64-encoded signed transaction to Jupiter's execute
// endpoint and returns the transaction signature on success.
func (t *jupiterSubmitTxTool) submit(ctx context.Context, signedTx string) (string, error) {
	body, _ := json.Marshal(map[string]string{"signedTransaction": signedTx})
	apiKey := os.Getenv("JUPITER_API_KEY")
	resp, err := doPost(ctx, t.executeURL, body, apiKey)
	if err != nil {
		return "", fmt.Errorf("Jupiter execute: %w", err)
	}
	var result struct {
		TxID string `json:"txid"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("parsing execute response: %w", err)
	}
	if result.TxID == "" {
		return "", fmt.Errorf("Jupiter execute returned no txid: %s", resp)
	}
	return result.TxID, nil
}

// ---- main -------------------------------------------------------------------

func main() {
	ipcDir := os.Getenv("SOLAI_IPC_DIR")
	if ipcDir == "" {
		fmt.Fprintln(os.Stderr, "SOLAI_IPC_DIR is not set")
		os.Exit(1)
	}

	data, err := os.ReadFile(filepath.Join(ipcDir, "input.json"))
	if err != nil {
		writeError(fmt.Sprintf("failed to read input: %v", err))
		return
	}
	var input ToolInput
	if err := json.Unmarshal(data, &input); err != nil {
		writeError(fmt.Sprintf("failed to parse input: %v", err))
		return
	}

	ctx := context.Background()

	// Wallet address is pre-injected by the coordinator via Capabilities.
	walletAddress := input.Capabilities["wallet_address"]

	quoteURL, swapURL, executeURL := jupiterBaseURLs()

	// Phase 2: coordinator re-invoked us with a signed transaction in Payload.
	if input.Payload != "" {
		submitter := &jupiterSubmitTxTool{executeURL: executeURL}
		txID, err := submitter.submit(ctx, input.Payload)
		if err != nil {
			writeError(fmt.Sprintf("submitting transaction: %v", err))
			return
		}
		out, _ := json.Marshal(map[string]string{"txid": txID})
		writeSuccess(out)
		return
	}

	// Phase 1: get quote, build unsigned tx, request signing.
	llm, err := newLLM(ctx)
	if err != nil {
		writeError(fmt.Sprintf("LLM initialisation failed: %v", err))
		return
	}

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

	prompt := input.Prompt
	if len(input.Tasks) > 0 {
		prompt = fmt.Sprintf("%s\nTasks:\n- %s", input.Prompt, strings.Join(input.Tasks, "\n- "))
	}

	result, err := chains.Run(ctx, executor, prompt)
	if err != nil {
		const parsePrefix = "unable to parse agent output: "
		if i := strings.Index(err.Error(), parsePrefix); i >= 0 {
			extracted := err.Error()[i+len(parsePrefix):]
			if strings.HasPrefix(extracted, "Thought:") {
				writeError(fmt.Sprintf("agent run failed: %v", err))
				return
			}
			result = extracted
		} else {
			writeError(fmt.Sprintf("agent run failed: %v", err))
			return
		}
	}

	out, _ := json.Marshal(result)
	writeSuccess(out)
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

func stripMarkdownFence(s string) string {
	s = strings.TrimSpace(s)
	for _, fence := range []string{"```json", "```"} {
		if strings.HasPrefix(s, fence) {
			s = strings.TrimPrefix(s, fence)
			s = strings.TrimSuffix(strings.TrimSpace(s), "```")
			s = strings.TrimSpace(s)
			break
		}
	}
	return s
}

func writeSuccess(data json.RawMessage) {
	writeOutput(ToolOutput{Type: "success", Payload: data})
}

func writeError(msg string) {
	out, _ := json.Marshal(msg)
	writeOutput(ToolOutput{Type: "error", Payload: out})
}

func writeRequest(payload json.RawMessage) {
	writeOutput(ToolOutput{Type: "request", Payload: payload})
}

func writeOutput(out ToolOutput) {
	data, _ := json.Marshal(out)
	if err := os.WriteFile(filepath.Join(os.Getenv("SOLAI_IPC_DIR"), "output.json"), data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write output.json: %v\n", err)
	}
}
