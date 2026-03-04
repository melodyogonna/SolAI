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

Model (coordinator LLM — pick one):
  model.provider       Provider for the coordinator: google, openai, or anthropic
  model.name           Model name (e.g. gemini-2.5-pro, gpt-4o, claude-opus-4-6)

Provider credentials (used by coordinator and injected into agentic tools):
  provider.google      Google AI API key
  provider.openai      OpenAI API key
  provider.anthropic   Anthropic API key

Agent settings:
  user-goals           Goals the agent should pursue autonomously
  cycle-interval       Duration between agent cycles (e.g. 5m, 1h)
  wallet-seed          BIP39 seed phrase for the agent wallet
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

	providers := make(map[string]string)
	for k, v := range cfg.Providers {
		providers[k] = redact(v)
	}

	display := map[string]any{
		"model": map[string]any{
			"provider": cfg.Model.Provider,
			"name":     cfg.Model.Name,
		},
		"providers":      providers,
		"user_goals":     cfg.UserGoals,
		"cycle_interval": cfg.CycleInterval,
		"wallet_seed":    redact(cfg.WalletSeed),
		"sandbox": map[string]any{
			"share_net":   cfg.Sandbox.ShareNet,
			"extra_binds": cfg.Sandbox.ExtraBinds,
		},
	}

	data, err := json.MarshalIndent(display, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// redact replaces a non-empty sensitive string with a redaction marker,
// preserving a short prefix for identification.
func redact(s string) string {
	if s == "" {
		return ""
	}
	if len(s) > 8 {
		return s[:4] + strings.Repeat("*", len(s)-4)
	}
	return strings.Repeat("*", len(s))
}
