package agent

import (
	"fmt"
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
	// LLM is the initialized language model. Assigned by the caller after LoadConfigFrom returns.
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
	loader := func(bwrapPath string, capManager *capability.CapabilityManager) ([]lctools.Tool, []error, error) {
		return tool.LoadTools(toolsDir, llmProvider, capManager, bwrapPath, cfg)
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
