package capability

import (
	"fmt"
	"os"
	"strings"
)

// knownProviders is the list of LLM providers whose credentials are read from env.
// To add a new provider, append its lowercase name here.
var knownProviders = []string{"google", "openai", "anthropic"}

// LLMModelOption carries the data LLMProvider needs to resolve credentials.
// Defined here (not in tool/) to keep the dependency one-way: tool → capability.
type LLMModelOption struct {
	Model    string
	Provider string
}

// LLMConfig is the resolved model and credentials to inject into a tool subprocess.
type LLMConfig struct {
	Provider string
	Model    string
	APIKey   string
}

// Env returns env var assignments to pass to the tool subprocess.
// The tool reads SOLAI_LLM_PROVIDER, SOLAI_LLM_MODEL, and SOLAI_LLM_API_KEY
// to initialise its own LLM client.
func (c *LLMConfig) Env() []string {
	return []string{
		"SOLAI_LLM_PROVIDER=" + c.Provider,
		"SOLAI_LLM_MODEL=" + c.Model,
		"SOLAI_LLM_API_KEY=" + c.APIKey,
	}
}

// LLMProvider stores API credentials for configured LLM providers.
// It is a Core-class system component: invisible to the main LLM and agentic tools.
//
// Credentials are loaded from environment variables at startup:
//
//	LLM_PROVIDER_GOOGLE=<key>
//	LLM_PROVIDER_OPENAI=<key>
//	LLM_PROVIDER_ANTHROPIC=<key>
type LLMProvider struct {
	credentials map[string]string // provider name (lowercase) → api key
}

// NewLLMProvider reads LLM_PROVIDER_* env vars and returns a configured LLMProvider.
// Providers whose env var is empty or unset are not stored.
func NewLLMProvider() *LLMProvider {
	creds := make(map[string]string)
	for _, name := range knownProviders {
		key := os.Getenv("LLM_PROVIDER_" + strings.ToUpper(name))
		if key != "" {
			creds[name] = key
		}
	}
	return &LLMProvider{credentials: creds}
}

// NewLLMProviderFromMap creates an LLMProvider from an explicit credentials map.
// Keys are lowercase provider names ("google", "openai", "anthropic").
// Entries for unknown or empty-valued providers are silently ignored.
func NewLLMProviderFromMap(creds map[string]string) *LLMProvider {
	filtered := make(map[string]string)
	for _, name := range knownProviders {
		if key, ok := creds[name]; ok && key != "" {
			filtered[name] = key
		}
	}
	return &LLMProvider{credentials: filtered}
}

// IsConfigured reports whether the given provider has credentials loaded.
func (p *LLMProvider) IsConfigured(provider string) bool {
	_, ok := p.credentials[strings.ToLower(provider)]
	return ok
}

// ResolveForTool selects the best LLMConfig for a tool based on its declared model
// preferences. Resolution order:
//  1. If the primary model's provider is configured → use it.
//  2. Otherwise iterate supported in declaration order, return the first configured provider.
//  3. If no supported provider is configured → return nil (caller should disable the tool).
func (p *LLMProvider) ResolveForTool(primary string, supported []LLMModelOption) *LLMConfig {
	// Build a lookup: model name → option, for finding the primary entry quickly.
	byModel := make(map[string]LLMModelOption, len(supported))
	for _, opt := range supported {
		byModel[opt.Model] = opt
	}

	// Step 1: try primary.
	if primaryOpt, ok := byModel[primary]; ok {
		if key, configured := p.credentials[strings.ToLower(primaryOpt.Provider)]; configured {
			return &LLMConfig{
				Provider: primaryOpt.Provider,
				Model:    primaryOpt.Model,
				APIKey:   key,
			}
		}
	}

	// Step 2: first supported with a configured provider.
	for _, opt := range supported {
		if key, configured := p.credentials[strings.ToLower(opt.Provider)]; configured {
			return &LLMConfig{
				Provider: opt.Provider,
				Model:    opt.Model,
				APIKey:   key,
			}
		}
	}

	return nil
}

// ConfiguredProviders returns the names of all providers with credentials loaded.
// Used for logging at startup.
func (p *LLMProvider) ConfiguredProviders() []string {
	names := make([]string, 0, len(p.credentials))
	for name := range p.credentials {
		names = append(names, name)
	}
	return names
}

// SupportedProviderList formats a ManifestLLMModel slice as a readable provider list.
// Used in warning messages when a tool is disabled.
func SupportedProviderList(models []LLMModelOption) string {
	seen := make(map[string]bool)
	var parts []string
	for _, m := range models {
		if !seen[m.Provider] {
			seen[m.Provider] = true
			parts = append(parts, fmt.Sprintf("%s (%s)", m.Model, m.Provider))
		}
	}
	return strings.Join(parts, ", ")
}
