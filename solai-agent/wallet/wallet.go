package wallet

import (
	"crypto/ed25519"

	"github.com/tyler-smith/go-bip39"
)

func generateSeedPhrase() (string, error) {
	entropy, err := bip39.NewEntropy(256)
	if err != nil {
		return "", err
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", err
	}
	return mnemonic, nil
}

func seedFromPhrase(phrase string) []byte {
	return bip39.NewSeed(phrase, "")
}

func createWalletFromSeed(seed []byte) (ed25519.PrivateKey, ed25519.PublicKey) {
	// Ed25519 requires a 32-byte seed. If we have more (like 64 bytes from BIP-39),
	// we use the first 32 bytes.
	if len(seed) > 32 {
		seed = seed[:32]
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	return privateKey, privateKey.Public().(ed25519.PublicKey)
}

func SignTransaction() {}

func VerifyTransaction() {}
