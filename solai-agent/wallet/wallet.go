package wallet

import (
	"crypto/ed25519"
	"fmt"

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

func seedFromPhrase(phrase string) ([]byte, error) {
	return bip39.EntropyFromMnemonic(phrase)
}

func createWalletFromSeed(seed []byte) (ed25519.PrivateKey, ed25519.PublicKey) {
	privateKey := ed25519.NewKeyFromSeed(seed)
	return privateKey, privateKey.Public().(ed25519.PublicKey)
}

func CreateWallet(seed []byte) (SolKeyPair, error) {
	if seed != nil {
		prv, pub := createWalletFromSeed(seed)
		return SolKeyPair{privateKey: prv, publicKey: pub}, nil
	}

	mnemonic, err := generateSeedPhrase()
	if err != nil {
		return SolKeyPair{}, fmt.Errorf("unable to generate key phrase %w", err)
	}

	seed, err = seedFromPhrase(mnemonic)
	if err != nil {
		return SolKeyPair{}, fmt.Errorf("unable to generate seed from phrase %w", err)
	}

	prv, pub := createWalletFromSeed(seed)
	return SolKeyPair{privateKey: prv, publicKey: pub}, nil
}

func SignTransaction() {}

func VerifyTransaction() {}
