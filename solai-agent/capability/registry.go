package capability

import (
	"fmt"
	"strings"
)

// Factory is a function that creates a new Capability instance.
// Used for lazy initialization — the factory is called once during SetUp.
type Factory func() Capability

// registry holds all registered capability factories, keyed by capability name.
var registry = make(map[string]Factory)

// Register adds a capability factory to the global registry.
// Typically called from init() functions in capability implementation files,
// or from main() before calling SetUp.
func Register(name string, factory Factory) {
	registry[name] = factory
}

// CapabilityChecker checks whether Regular capabilities are available.
// Implemented by CapabilityManager; accepted by the tool loader to validate
// required_capabilities declarations without creating a circular import.
type CapabilityChecker interface {
	IsRegularCapabilityAvailable(name string) bool
}

// CapabilityManager holds the active set of capabilities for a running agent.
type CapabilityManager struct {
	capabilities []Capability
}

// SetUp creates a CapabilityManager from a list of capability names.
// Names not found in the registry are silently skipped.
// Call Register() for all capabilities before calling SetUp.
func SetUp(names []string) *CapabilityManager {
	cm := &CapabilityManager{}
	for _, name := range names {
		if factory, ok := registry[name]; ok {
			cm.capabilities = append(cm.capabilities, factory())
		}
	}
	return cm
}

// GetByClass returns all capabilities of the given class.
func (m *CapabilityManager) GetByClass(class CapabilityClass) []Capability {
	var result []Capability
	for _, c := range m.capabilities {
		if c.Class() == class {
			result = append(result, c)
		}
	}
	return result
}

// GetAll returns all active capabilities regardless of class.
func (m *CapabilityManager) GetAll() []Capability {
	return m.capabilities
}

// IsRegularCapabilityAvailable reports whether a Regular capability with the
// given name is currently registered in the manager. Used by the tool loader
// to validate tool required_capabilities declarations at startup.
func (m *CapabilityManager) IsRegularCapabilityAvailable(name string) bool {
	for _, c := range m.capabilities {
		if c.Class() == Regular && c.Name() == name {
			return true
		}
	}
	return false
}

// BuildCapabilityPromptSection generates a Markdown block describing all
// Internal capabilities, for injection into the per-cycle prompt so the LLM
// is aware of them (e.g. its own wallet address).
// Returns an empty string if there are no Internal capabilities.
func (m *CapabilityManager) BuildCapabilityPromptSection() string {
	internals := m.GetByClass(Internal)
	if len(internals) == 0 {
		return ""
	}
	var b strings.Builder
	for i, c := range internals {
		fmt.Fprintf(&b, "- **%s**: %s", c.Name(), c.Description())
		if i < len(internals)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
