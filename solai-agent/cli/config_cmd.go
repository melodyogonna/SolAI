package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/melodyogonna/solai/solai-agent/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage SolAI configuration",
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value in ~/.solai/config.json.

Supported keys:
  api-key              Gemini API key for the main agent LLM
  provider.google      Google API key for agentic tools
  provider.openai      OpenAI API key for agentic tools
  provider.anthropic   Anthropic API key for agentic tools
  wallet-seed          BIP39 seed phrase for the agent wallet
  cycle-interval       Duration between agent cycles (e.g. 5m, 1h)
  user-goals           Goals the agent should pursue autonomously
  sandbox.share-net    Allow agent sandbox network access (true/false)`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration values (sensitive values are redacted)",
	Args:  cobra.NoArgs,
	RunE:  runConfigList,
}

func init() {
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configListCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key, value := args[0], args[1]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if err := cfg.Set(key, value); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("Set %s\n", key)
	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	val, err := cfg.Get(key)
	if err != nil {
		return err
	}
	fmt.Println(val)
	return nil
}

func runConfigList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Build a redacted copy for display.
	display := map[string]any{
		"api_key":        redact(cfg.APIKey),
		"wallet_seed":    redact(cfg.WalletSeed),
		"cycle_interval": cfg.CycleInterval,
		"user_goals":     cfg.UserGoals,
		"sandbox": map[string]any{
			"share_net":   cfg.Sandbox.ShareNet,
			"extra_binds": cfg.Sandbox.ExtraBinds,
		},
	}

	providers := make(map[string]string)
	for k, v := range cfg.Providers {
		providers[k] = redact(v)
	}
	display["providers"] = providers

	data, err := json.MarshalIndent(display, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// redact replaces a non-empty sensitive string with a redaction marker.
func redact(s string) string {
	if s == "" {
		return ""
	}
	// Show a short prefix for identification, redact the rest.
	if len(s) > 8 {
		return s[:4] + strings.Repeat("*", len(s)-4)
	}
	return strings.Repeat("*", len(s))
}
