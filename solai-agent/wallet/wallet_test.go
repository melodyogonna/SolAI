package wallet

import (
	"strings"
	"testing"
)

func TestGenerateSeedPhrase(t *testing.T) {
	phrase, err := generateSeedPhrase()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if phrase == "" {
		t.Error("Expected a non-empty phrase")
	}

	words := strings.Split(phrase, " ")
	if len(words) != 24 {
		t.Errorf("Expected 24 words, got %d", len(words))
	}
}

func TestSeedFromPhrase(t *testing.T) {
	phrase, err := generateSeedPhrase()
	if err != nil {
		t.Fatalf("Failed to generate phrase: %v", err)
	}

	seed := seedFromPhrase(phrase)
	if seed == nil {
		t.Error("Expected non-nil seed")
	}

	if len(seed) != 64 { // BIP-39 seeds are 512 bits (64 bytes)
		t.Errorf("Expected seed length 64, got %d", len(seed))
	}
}
