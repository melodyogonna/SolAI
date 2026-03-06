package main

import "encoding/json"

type ToolInput struct {
	Overview string   `json:"overview"`
	Tasks    []string `json:"tasks"`
}

type ToolOutput struct {
	Type   string          `json:"type"`
	Output json.RawMessage `json:"output"`
}

type jupiterEntry struct {
	UsdPrice    float64 `json:"usdPrice"`
	PriceChange float64 `json:"priceChange24h"`
}

type TokenPrice struct {
	Symbol       string  `json:"symbol"`
	Mint         string  `json:"mint"`
	PriceUSD     float64 `json:"price_usd"`
	Change24hPct float64 `json:"change_24h_pct"`
}
