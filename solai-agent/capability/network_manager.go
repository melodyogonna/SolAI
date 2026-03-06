package capability

import (
	"context"
	"encoding/json"
)

// NetworkManagerCapability is a Regular capability that grants outbound network
// access to agentic tools declaring "network-manager" in their manifest.
//
// When registered and passed to SetUp, tools listing "network-manager" in
// required_capabilities receive --share-net in their sandbox policy, allowing
// outbound HTTP/HTTPS and other network calls. Tools without this declaration
// run with --unshare-all (network isolated).
type NetworkManagerCapability struct{}

// NewNetworkManagerCapability constructs a NetworkManagerCapability.
func NewNetworkManagerCapability() *NetworkManagerCapability {
	return &NetworkManagerCapability{}
}

// Name implements Capability.
func (n *NetworkManagerCapability) Name() string { return "network-manager" }

// Class implements Capability — Regular, so tools can declare it in their manifest.
func (n *NetworkManagerCapability) Class() CapabilityClass { return Regular }

// Description implements Capability.
func (n *NetworkManagerCapability) Description() string {
	return "Grants outbound network access to tools that declare it"
}

// Execute implements Capability. Returns a simple status JSON.
func (n *NetworkManagerCapability) Execute(_ context.Context, _ string) (string, error) {
	out, _ := json.Marshal(map[string]string{"status": "network access granted"})
	return string(out), nil
}

// ToolRequestDescription implements Capability. Network access is granted at
// sandbox setup time via required_capabilities, not via runtime requests.
func (n *NetworkManagerCapability) ToolRequestDescription() string { return "" }
