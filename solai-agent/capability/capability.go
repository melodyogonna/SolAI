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

func SetUp(enabledCapabilities []string) *capabilityManager {
	cm := &capabilityManager{}

	for _, capName := range enabledCapabilities {
		switch capName {
		// case "some_capability":
		// 	cm.enabledCapabilities = append(cm.enabledCapabilities, NewSomeCapability())
		}
	}

	return cm
}
