package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	_ "github.com/joho/godotenv/autoload"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/tools"
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

	agent, err := agents.Initialize(
		llm,
		[]tools.Tool{},
		agents.ZeroShotReactDescription,
		agents.WithMaxIterations(3),
	)
	if err != nil {
		log.Fatalf("unable to initialize agent: %v", err)
	}
	executor := agents.NewExecutor(agent)
	answer, err := chains.Run(ctx, executor, string(prompt))
	if err != nil {
		log.Print(err)
	}

	fmt.Print(answer)
}
