package agent

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/melodyogonna/solai/solai-agent/capability"
	solaiconfig "github.com/melodyogonna/solai/solai-agent/config"
	"github.com/melodyogonna/solai/solai-agent/tool"
	"github.com/melodyogonna/solai/solai-agent/wallet"
	"github.com/tmc/langchaingo/llms"
	lctools "github.com/tmc/langchaingo/tools"
)

// Config holds all runtime configuration for the agent.
// It is built once in main.go and passed into Run.
type Config struct {
	// LLM is the initialized language model. Set by main.go after LoadConfig returns.
	LLM llms.Model

	// SystemPrompt is the content of the SYSTEM_PROMPT file, loaded at startup.
	// It defines the agent's role, rules, and tool usage patterns.
	SystemPrompt string

	// UserGoals is the content of the USER_PROMPT file, loaded at startup.
	// It defines what the user wants the agent to accomplish autonomously.
	UserGoals string

	// ToolsDir is the path to the directory containing agentic tool subdirectories.
	ToolsDir string

	// Wallet is the agent's Solana keypair, exposed as an Internal capability.
	Wallet *wallet.SolKeyPair

	// CycleTimeout is the maximum time allowed for a single autonomous cycle.
	// If a cycle exceeds this, it is cancelled and the next one starts immediately.
	CycleTimeout time.Duration

	// LLMProvider manages API credentials for LLM providers used by agentic tools.
	LLMProvider *capability.LLMProvider

	// SystemManager owns tool loading, LLM provider logging, and cleanup job scheduling.
	SystemManager *capability.SystemManager
}

// LoadConfig reads all configuration from environment variables.
// Returns an error if any required variable is missing or a required file cannot be read.
//
// Required env vars: API_KEY, SYSTEM_PROMPT, USER_PROMPT, TOOLS_DIR
// Optional env vars: WALLET_SEED (empty → generate new wallet), CYCLE_TIMEOUT (default: 5m)
//
// Note: LoadConfig does not initialize the LLM or set Config.LLM. The caller (main.go)
// is responsible for constructing the LLM with the API_KEY and assigning it.
func LoadConfig() (Config, error) {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		return Config{}, fmt.Errorf("API_KEY environment variable is required")
	}

	systemPromptPath := os.Getenv("SYSTEM_PROMPT")
	if systemPromptPath == "" {
		return Config{}, fmt.Errorf("SYSTEM_PROMPT environment variable is required")
	}
	systemPrompt, err := loadFile(systemPromptPath)
	if err != nil {
		return Config{}, fmt.Errorf("loading system prompt: %w", err)
	}

	userPromptPath := os.Getenv("USER_PROMPT")
	if userPromptPath == "" {
		return Config{}, fmt.Errorf("USER_PROMPT environment variable is required")
	}
	userGoals, err := loadFile(userPromptPath)
	if err != nil {
		return Config{}, fmt.Errorf("loading user goals: %w", err)
	}

	toolsDir := os.Getenv("TOOLS_DIR")
	if toolsDir == "" {
		return Config{}, fmt.Errorf("TOOLS_DIR environment variable is required")
	}

	seedPhrase := os.Getenv("WALLET_SEED")
	kp, err := wallet.CreateWallet(seedPhrase)
	if err != nil {
		return Config{}, fmt.Errorf("creating wallet: %w", err)
	}

	cycleInterval := parseDuration(os.Getenv("CYCLE_TIMEOUT"), 5*time.Minute)

	llmProvider := capability.NewLLMProvider()
	loader := func(bwrapPath string, checker capability.CapabilityChecker) ([]lctools.Tool, []error, error) {
		return tool.LoadTools(toolsDir, llmProvider, checker, bwrapPath, nil)
	}
	systemManager := capability.NewSystemManager(loader, llmProvider)

	return Config{
		SystemPrompt:  systemPrompt,
		UserGoals:     userGoals,
		ToolsDir:      toolsDir,
		Wallet:        &kp,
		CycleTimeout: cycleInterval,
		LLMProvider:   llmProvider,
		SystemManager: systemManager,
	}, nil
}

// LoadConfigFrom builds an agent Config from a SolaiConfig (from ~/.solai/config.json).
// toolsDir and systemPrompt are injected directly rather than read from env vars,
// allowing the CLI path to bypass environment variables entirely.
//
// Note: LoadConfigFrom does not initialize the LLM or set Config.LLM. The caller
// is responsible for constructing the LLM with cfg.APIKey and assigning it.
func LoadConfigFrom(cfg *solaiconfig.SolaiConfig, toolsDir, systemPrompt string) (Config, error) {
	if cfg.Model.Provider == "" || cfg.Model.Name == "" {
		return Config{}, fmt.Errorf("model not configured (run: solai config set model.provider <google|openai|anthropic> && solai config set model.name <model>)")
	}
	if cfg.Providers[cfg.Model.Provider] == "" {
		return Config{}, fmt.Errorf("no API key for provider %q (run: solai config set provider.%s <key>)", cfg.Model.Provider, cfg.Model.Provider)
	}

	kp, err := wallet.CreateWallet(cfg.WalletSeed)
	if err != nil {
		return Config{}, fmt.Errorf("creating wallet: %w", err)
	}

	cycleInterval := parseDuration(cfg.CycleTimeout, 5*time.Minute)

	llmProvider := capability.NewLLMProviderFromMap(cfg.Providers)
	loader := func(bwrapPath string, checker capability.CapabilityChecker) ([]lctools.Tool, []error, error) {
		return tool.LoadTools(toolsDir, llmProvider, checker, bwrapPath, cfg)
	}
	systemManager := capability.NewSystemManager(loader, llmProvider)

	return Config{
		SystemPrompt:  systemPrompt,
		UserGoals:     cfg.UserGoals,
		ToolsDir:      toolsDir,
		Wallet:        &kp,
		CycleTimeout: cycleInterval,
		LLMProvider:   llmProvider,
		SystemManager: systemManager,
	}, nil
}

// loadFile reads the full content of a file and returns it as a trimmed string.
func loadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading file %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// parseDuration parses a Go duration string. Returns defaultVal if the string
// is empty or cannot be parsed.
func parseDuration(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}
