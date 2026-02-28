package wallet

import (
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

// AI! populate this function to take a phrase and seed by passing the phrase through PBKDF2 derivation function
func seedFromPhrase(phrase string) {}

func CreateWallet() {}

func SignTransaction() {}

func VerifyTransaction() {}
