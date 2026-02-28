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

func seedFromPhrase(phrase string) []byte {
	return bip39.NewSeed(phrase, "")
}

func CreateWallet() {}

func SignTransaction() {}

func VerifyTransaction() {}
