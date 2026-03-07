package capability

import (
	"context"
	"fmt"
	"strings"

	lctools "github.com/tmc/langchaingo/tools"
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

// capabilityTool adapts a Capability to the langchaingo tools.Tool interface
// so Internal and Regular capabilities can be called by the LLM via the ReAct loop.
type capabilityTool struct{ c Capability }

func (t capabilityTool) Name() string        { return t.c.Name() }
func (t capabilityTool) Description() string { return t.c.Description() }
func (t capabilityTool) Call(ctx context.Context, input string) (string, error) {
	return t.c.Execute(ctx, input)
}

// GetAgentTools returns Internal and Regular capabilities wrapped as
// langchaingo tools so the coordinator's ReAct agent can call them directly.
// Capabilities with an empty Description are excluded — they are infrastructure
// concerns (e.g. network-manager) that the LLM does not need to invoke.
func (m *CapabilityManager) GetAgentTools() []lctools.Tool {
	var tools []lctools.Tool
	for _, c := range m.capabilities {
		if (c.Class() == Internal || c.Class() == Regular) && c.Description() != "" {
			tools = append(tools, capabilityTool{c})
		}
	}
	return tools
}

// GetByName returns the capability with the given name, or nil if not found.
func (m *CapabilityManager) GetByName(name string) Capability {
	for _, c := range m.capabilities {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// BuildCapabilityPromptSection generates a Markdown block describing all
// Internal and Regular capabilities for injection into the coordinator's
// per-cycle prompt. Capabilities with an empty Description are omitted.
// Returns an empty string if none remain.
func (m *CapabilityManager) BuildCapabilityPromptSection() string {
	var visible []Capability
	for _, c := range m.capabilities {
		if (c.Class() == Internal || c.Class() == Regular) && c.Description() != "" {
			visible = append(visible, c)
		}
	}
	if len(visible) == 0 {
		return ""
	}
	var b strings.Builder
	for i, c := range visible {
		fmt.Fprintf(&b, "- **%s**: %s", c.Name(), c.Description())
		if i < len(visible)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

