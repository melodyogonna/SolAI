package wallet
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
