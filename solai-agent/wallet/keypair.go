package wallet

import "crypto/ed25519"

type SolKeyPair struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

// AI! populate this function to return Base58 encoded public key
func (kp SolKeyPair) Base58PubKey() {}
