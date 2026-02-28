package wallet

import (
	"crypto/ed25519"

	"github.com/mr-tron/base58"
)

type SolKeyPair struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

func (kp SolKeyPair) Base58PubKey() string {
	return base58.Encode(kp.publicKey)
}
