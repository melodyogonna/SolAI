package capability

import "context"

// CapabilityClass defines the visibility and access level of a capability.
type CapabilityClass int

const (
	// Core capabilities run invisibly in the background and are never
	// exposed to the LLM or to agentic tools. Reserved for future use.
	Core CapabilityClass = iota

	// Internal capabilities are known to the main LLM but never exposed
	// to agentic tools. The wallet is the primary Internal capability.
	// Internal capabilities are injected into the system prompt so the
	// LLM knows they exist.
	Internal

	// Regular capabilities are available to both the main LLM and can
	// be requested by agentic tools. Reserved for future expansion.
	Regular
)

// Capability is the interface that all system capabilities must implement.
// Capabilities are managed separately from agentic tools — they are injected
// into the system prompt rather than the LLM's tool list.
type Capability interface {
	// Name returns the unique identifier for this capability.
	Name() string

	// Description returns a human-readable description for injection into
	// the system prompt. This is what the LLM sees.
	Description() string

	// Class returns the CapabilityClass, determining visibility.
	Class() CapabilityClass

	// Execute performs the capability's action.
	Execute(ctx context.Context, input string) (string, error)
}
