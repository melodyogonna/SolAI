package main

import "encoding/json"

type ToolInput struct {
	Prompt  string            `json:"prompt"`
	Payload map[string]string `json:"payload,omitempty"`
}

type ToolOutput struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// CapabilityRequest is the payload written inside a "request" output when the
// tool needs the coordinator to invoke a capability on its behalf.
type CapabilityRequest struct {
	Capability string `json:"capability"`
	Action     string `json:"action"`
	Input      string `json:"input"`
	// Instruction is natural language telling the coordinator LLM what to do
	// after calling the capability (e.g. re-invoke this tool with payloads).
	Instruction string `json:"instruction"`
}

// ---- Jupiter API types ------------------------------------------------------

type quoteRequest struct {
	InputMint   string `json:"inputMint"`
	OutputMint  string `json:"outputMint"`
	Amount      int64  `json:"amount"`
	SlippageBps int    `json:"slippageBps"`
}

// QuoteResponse is Jupiter's v6 /quote response. Stored as raw JSON so it can
// be forwarded verbatim to the /swap endpoint.
type QuoteResponse struct {
	InputMint            string          `json:"inputMint"`
	InAmount             string          `json:"inAmount"`
	OutputMint           string          `json:"outputMint"`
	OutAmount            string          `json:"outAmount"`
	OtherAmountThreshold string          `json:"otherAmountThreshold"`
	SwapMode             string          `json:"swapMode"`
	SlippageBps          int             `json:"slippageBps"`
	PriceImpactPct       string          `json:"priceImpactPct"`
	RoutePlan            json.RawMessage `json:"routePlan"`
}

type swapRequest struct {
	QuoteResponse             json.RawMessage `json:"quoteResponse"`
	UserPublicKey             string          `json:"userPublicKey"`
	DynamicComputeUnitLimit   bool            `json:"dynamicComputeUnitLimit"`
	PrioritizationFeeLamports string          `json:"prioritizationFeeLamports"`
}

type swapResponse struct {
	SwapTransaction string `json:"swapTransaction"`
}

// SwapResult is the structured output returned to the coordinator.
type SwapResult struct {
	InputMint       string `json:"input_mint"`
	OutputMint      string `json:"output_mint"`
	InAmount        string `json:"in_amount"`
	OutAmount       string `json:"out_amount"`
	PriceImpactPct  string `json:"price_impact_pct"`
	SlippageBps     int    `json:"slippage_bps"`
	SwapTransaction string `json:"swap_transaction,omitempty"`
	Note            string `json:"note,omitempty"`
}
