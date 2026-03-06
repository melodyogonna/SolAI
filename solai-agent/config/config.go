// Package config manages the ~/.solai/ configuration directory and config.json.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SolaiConfig is the top-level configuration stored in ~/.solai/config.json.
type SolaiConfig struct {
	Model         ModelConfig                  `json:"model"`
	Providers     map[string]string            `json:"providers"` // google/openai/anthropic → api key
	WalletSeed    string                       `json:"wallet_seed"`
	CycleTimeout  string                       `json:"cycle_timeout"`
	UserGoals     string                       `json:"user_goals"`
	Sandbox       SandboxConfig                `json:"sandbox"`
	Solana        SolanaConfig                 `json:"solana"`
	ToolEnv       map[string]map[string]string `json:"tool_env,omitempty"` // tool name → var name → value
}

// SolanaConfig controls how the agent interacts with the Solana blockchain.
type SolanaConfig struct {
	RPCURL     string `json:"rpc_url"`    // default: https://api.mainnet-beta.solana.com
	Commitment string `json:"commitment"` // "finalized", "confirmed", or "processed"
}

// ModelConfig identifies the LLM the coordinator uses.
type ModelConfig struct {
	Provider string `json:"provider"` // "google", "openai", or "anthropic"
	Name     string `json:"name"`     // e.g. "gemini-2.5-pro", "gpt-4o", "claude-opus-4-6"
}

// SandboxConfig controls how the agent process is sandboxed by bwrap.
type SandboxConfig struct {
	ShareNet   bool     `json:"share_net"`
	ExtraBinds []FSBind `json:"extra_binds"`
}

// FSBind is an extra filesystem path exposed inside the agent sandbox.
type FSBind struct {
	Path     string `json:"path"`
	ReadOnly bool   `json:"read_only"`
}

// Dir returns the ~/.solai directory path.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".solai")
	}
	return filepath.Join(home, ".solai")
}

// ToolsDir returns the ~/.solai/tools directory path.
func ToolsDir() string {
	path := filepath.Join(Dir(), "tools")
	_ = os.MkdirAll(path, 0o755)
	return path
}

// ConfigPath returns the path to ~/.solai/config.json.
func ConfigPath() string {
	return filepath.Join(Dir(), "config.json")
}

// DefaultConfig returns a SolaiConfig with sensible defaults.
func DefaultConfig() *SolaiConfig {
	return &SolaiConfig{
		Providers:     map[string]string{},
		CycleTimeout: "5m",
		Sandbox: SandboxConfig{
			ShareNet:   true,
			ExtraBinds: []FSBind{},
		},
		Solana: SolanaConfig{
			RPCURL:     "https://api.mainnet-beta.solana.com",
			Commitment: "confirmed",
		},
	}
}

// Load reads ~/.solai/config.json, creating it with defaults if absent.
func Load() (*SolaiConfig, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := DefaultConfig()
		if err := cfg.Save(); err != nil {
			return nil, fmt.Errorf("creating default config: %w", err)
		}
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

// Save writes the config atomically (temp file + rename) to ~/.solai/config.json.
func (c *SolaiConfig) Save() error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	tmp := ConfigPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing temp config: %w", err)
	}
	if err := os.Rename(tmp, ConfigPath()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming temp config: %w", err)
	}
	return nil
}

// Set updates a single configuration value by dot-notation key.
//
// Supported keys: model.provider, model.name,
// provider.google, provider.openai, provider.anthropic,
// wallet-seed, cycle-timeout, user-goals, sandbox.share-net.
func (c *SolaiConfig) Set(key, value string) error {
	switch key {
	case "model.provider":
		c.Model.Provider = value
	case "model.name":
		c.Model.Name = value
	case "provider.google":
		c.ensureProviders()
		c.Providers["google"] = value
	case "provider.openai":
		c.ensureProviders()
		c.Providers["openai"] = value
	case "provider.anthropic":
		c.ensureProviders()
		c.Providers["anthropic"] = value
	case "wallet-seed":
		c.WalletSeed = value
	case "cycle-timeout":
		c.CycleTimeout = value
	case "user-goals":
		c.UserGoals = value
	case "sandbox.share-net":
		switch strings.ToLower(value) {
		case "true", "1", "yes":
			c.Sandbox.ShareNet = true
		case "false", "0", "no":
			c.Sandbox.ShareNet = false
		default:
			return fmt.Errorf("sandbox.share-net: expected true/false, got %q", value)
		}
	case "solana.rpc-url":
		c.Solana.RPCURL = value
	case "solana.commitment":
		switch value {
		case "finalized", "confirmed", "processed":
			c.Solana.Commitment = value
		default:
			return fmt.Errorf("solana.commitment: expected finalized/confirmed/processed, got %q", value)
		}
	default:
		// Dynamic key: tool-env.<toolname>.<VAR_NAME>
		if rest, ok := strings.CutPrefix(key, "tool-env."); ok {
			dot := strings.IndexByte(rest, '.')
			if dot < 1 || dot == len(rest)-1 {
				return fmt.Errorf("tool-env key must be tool-env.<tool>.<VAR>, got %q", key)
			}
			toolName, varName := rest[:dot], rest[dot+1:]
			c.ensureToolEnv(toolName)
			c.ToolEnv[toolName][varName] = value
			return nil
		}
		return fmt.Errorf("unknown config key %q; valid keys: model.provider, model.name, provider.google, provider.openai, provider.anthropic, wallet-seed, cycle-timeout, user-goals, sandbox.share-net, solana.rpc-url, solana.commitment, tool-env.<tool>.<VAR>", key)
	}
	return nil
}

// Get retrieves a single configuration value by dot-notation key.
func (c *SolaiConfig) Get(key string) (string, error) {
	switch key {
	case "model.provider":
		return c.Model.Provider, nil
	case "model.name":
		return c.Model.Name, nil
	case "provider.google":
		return c.Providers["google"], nil
	case "provider.openai":
		return c.Providers["openai"], nil
	case "provider.anthropic":
		return c.Providers["anthropic"], nil
	case "wallet-seed":
		return c.WalletSeed, nil
	case "cycle-timeout":
		return c.CycleTimeout, nil
	case "user-goals":
		return c.UserGoals, nil
	case "sandbox.share-net":
		if c.Sandbox.ShareNet {
			return "true", nil
		}
		return "false", nil
	case "solana.rpc-url":
		return c.Solana.RPCURL, nil
	case "solana.commitment":
		return c.Solana.Commitment, nil
	default:
		if rest, ok := strings.CutPrefix(key, "tool-env."); ok {
			dot := strings.IndexByte(rest, '.')
			if dot < 1 || dot == len(rest)-1 {
				return "", fmt.Errorf("tool-env key must be tool-env.<tool>.<VAR>, got %q", key)
			}
			toolName, varName := rest[:dot], rest[dot+1:]
			if c.ToolEnv == nil {
				return "", nil
			}
			return c.ToolEnv[toolName][varName], nil
		}
		return "", fmt.Errorf("unknown config key %q", key)
	}
}

// ToolEnvFor returns the configured environment variables for a tool as a
// "KEY=VALUE" slice ready for injection into a subprocess environment.
func (c *SolaiConfig) ToolEnvFor(toolName string) []string {
	if c.ToolEnv == nil {
		return nil
	}
	vars := c.ToolEnv[toolName]
	if len(vars) == 0 {
		return nil
	}
	result := make([]string, 0, len(vars))
	for k, v := range vars {
		result = append(result, k+"="+v)
	}
	return result
}

func (c *SolaiConfig) ensureProviders() {
	if c.Providers == nil {
		c.Providers = make(map[string]string)
	}
}

func (c *SolaiConfig) ensureToolEnv(toolName string) {
	if c.ToolEnv == nil {
		c.ToolEnv = make(map[string]map[string]string)
	}
	if c.ToolEnv[toolName] == nil {
		c.ToolEnv[toolName] = make(map[string]string)
	}
}
