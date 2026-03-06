package capability

import (
	"context"
	"fmt"

	"github.com/melodyogonna/solai/solai-agent/wallet"
)

// WalletCapability exposes the agent's Solana wallet as an Internal capability.
// It injects the wallet's public address into the system prompt so the LLM
// knows which wallet it controls, without exposing the private key.
type WalletCapability struct {
	keypair *wallet.SolKeyPair
}

// NewWalletCapability creates a WalletCapability wrapping the given keypair.
func NewWalletCapability(kp *wallet.SolKeyPair) *WalletCapability {
	return &WalletCapability{keypair: kp}
}

func (w *WalletCapability) Name() string {
	return "wallet"
}

func (w *WalletCapability) Class() CapabilityClass {
	return Internal
}

// Description returns a prompt-ready description of the wallet capability.
// This is injected into the per-cycle prompt so the LLM is aware of its
// wallet address when coordinating tools that need to know the source wallet.
func (w *WalletCapability) Description() string {
	return fmt.Sprintf(
		"Your Solana wallet. Public address: %s. "+
			"Call this tool with any input to retrieve the wallet's public address as a string. "+
			"Use this when you need to pass your wallet address to another tool or include it in a transaction.",
		w.keypair.Base58PubKey(),
	)
}

// Execute returns the wallet's public key as a string.
// Signing and transaction capabilities are reserved for future implementation.
func (w *WalletCapability) Execute(_ context.Context, _ string) (string, error) {
	return w.keypair.Base58PubKey(), nil
}
