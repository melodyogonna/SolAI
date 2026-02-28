package capability

import "context"

type Capability interface {
	// Name returns the unique identifier for this capability
	Name() string
	// Description returns what this capability does (for the LLM system prompt)
	Description() string
	// Execute performs the actual action
	Execute(ctx context.Context, input string) (string, error)
}

// Factory defines a function that creates a new instance of a Capability
type Factory func() Capability

// registry holds all available capability constructors
var registry = make(map[string]Factory)

// Register adds a capability to the available options
func Register(name string, factory Factory) {
	registry[name] = factory
}

type capabilityManager struct {
	enabledCapabilities []Capability
}

func (m capabilityManager) GetSystemCapabilities() []Capability {
	return m.enabledCapabilities
}

func SetUp(enabledCapabilities []string) *capabilityManager {
	cm := &capabilityManager{}

	for _, capName := range enabledCapabilities {
		if factory, exists := registry[capName]; exists {
			cm.enabledCapabilities = append(cm.enabledCapabilities, factory())
		}
	}

	return cm
}
