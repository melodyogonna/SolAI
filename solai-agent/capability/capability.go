package capability

import "context"

type Capability interface {
	Execute(ctx context.Context)
}

type capabilityManager struct {
	enabledCapabilities []Capability
}

func (m capabilityManager) GetSystemCapabilities() []Capability {
	return m.enabledCapabilities
}

// AI! make this setup function take a config with enabled capabilities. I think it should be a slice of strings, then we can match each string to a function and add them to capability manager
func SetUp() *capabilityManager {
	cm := &capabilityManager{}
}
