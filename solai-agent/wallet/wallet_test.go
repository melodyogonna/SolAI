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

	seed, err := seedFromPhrase(phrase)
	if err != nil {
		t.Fatalf("Failed to get seed from phrase: %v", err)
	}
	if seed == nil {
		t.Error("Expected non-nil seed")
	}

	if len(seed) != 32 {
		t.Errorf("Expected seed length 32, got %d", len(seed))
	}
}
