package main

import (
	"context"
	"io"
	"log"
	"os"

	_ "github.com/joho/godotenv/autoload"
	"github.com/melodyogonna/solai/solai-agent/wallet"
	"github.com/tmc/langchaingo/llms/googleai"
)

func main() {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("API key needs to be specified.")
	}
	systemPromptLocation := os.Getenv("SYSTEM_PROMPT")
	if systemPromptLocation == "" {
		log.Fatal("system prompt not passed, exiting.")
	}

	file, err := os.Open(systemPromptLocation)
	if err != nil {
		log.Fatalf("could not open system prompt at location %s - err %s", systemPromptLocation, err)
	}
	defer file.Close()

	ctx := context.Background()
	llm, err := googleai.New(ctx, googleai.WithAPIKey(apiKey), googleai.WithDefaultModel("gemini-2.5-pro"))
	if err != nil {
		log.Fatal(err)
	}

	prompt, err := io.ReadAll(file)
	if err != nil {
		log.Fatalf("unable to read prompt due to err - %s", err)
	}
	wallet, err := wallet.CreateWallet("")
	if err != nil {
		log.Fatal(err)
	}
	config := agentConfig{
		model:        llm,
		systemPrompt: prompt,
		wallet:       &wallet,
	}
	run(ctx, config)
}
