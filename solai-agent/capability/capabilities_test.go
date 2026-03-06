package capability

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/melodyogonna/solai/solai-agent/wallet"
)

// ---- WalletCapability -------------------------------------------------------

func newTestWallet(t *testing.T) *wallet.SolKeyPair {
	t.Helper()
	kp, err := wallet.CreateWallet("")
	if err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}
	return &kp
}

func TestWalletCapability_Name(t *testing.T) {
	kp := newTestWallet(t)
	w := NewWalletCapability(kp)
	if w.Name() != "wallet" {
		t.Errorf("Name: got %q, want wallet", w.Name())
	}
}

func TestWalletCapability_Class(t *testing.T) {
	kp := newTestWallet(t)
	w := NewWalletCapability(kp)
	if w.Class() != Regular {
		t.Errorf("Class: got %v, want Regular", w.Class())
	}
}

func TestWalletCapability_Description_ContainsPubKey(t *testing.T) {
	kp := newTestWallet(t)
	w := NewWalletCapability(kp)
	desc := w.Description()

	pubKey := kp.Base58PubKey()
	if !strings.Contains(desc, pubKey) {
		t.Errorf("Description should contain public key %q, got: %q", pubKey, desc)
	}
}

func TestWalletCapability_Description_NonEmpty(t *testing.T) {
	kp := newTestWallet(t)
	w := NewWalletCapability(kp)
	if w.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestWalletCapability_Execute_ReturnsPubKey(t *testing.T) {
	kp := newTestWallet(t)
	w := NewWalletCapability(kp)
	result, err := w.Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != kp.Base58PubKey() {
		t.Errorf("Execute: got %q, want %q", result, kp.Base58PubKey())
	}
}

func TestWalletCapability_Execute_InputIgnored(t *testing.T) {
	kp := newTestWallet(t)
	w := NewWalletCapability(kp)
	r1, _ := w.Execute(context.Background(), "anything")
	r2, _ := w.Execute(context.Background(), "")
	if r1 != r2 {
		t.Errorf("Execute should ignore input: got %q vs %q", r1, r2)
	}
}

func TestWalletCapability_DifferentWallets_DifferentKeys(t *testing.T) {
	kp1 := newTestWallet(t)
	kp2 := newTestWallet(t)
	w1 := NewWalletCapability(kp1)
	w2 := NewWalletCapability(kp2)
	if w1.Description() == w2.Description() {
		t.Error("different wallets should produce different descriptions")
	}
}

// ---- NetworkManagerCapability -----------------------------------------------

func TestNetworkManagerCapability_Name(t *testing.T) {
	n := NewNetworkManagerCapability()
	if n.Name() != "network-manager" {
		t.Errorf("Name: got %q, want network-manager", n.Name())
	}
}

func TestNetworkManagerCapability_Class(t *testing.T) {
	n := NewNetworkManagerCapability()
	if n.Class() != Regular {
		t.Errorf("Class: got %v, want Regular", n.Class())
	}
}

func TestNetworkManagerCapability_Description_NonEmpty(t *testing.T) {
	n := NewNetworkManagerCapability()
	if n.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestNetworkManagerCapability_Execute_ReturnsJSON(t *testing.T) {
	n := NewNetworkManagerCapability()
	result, err := n.Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Errorf("Execute result is not valid JSON: %v (got %q)", err, result)
	}
}

func TestNetworkManagerCapability_IsNotCore(t *testing.T) {
	n := NewNetworkManagerCapability()
	if n.Class() == Core {
		t.Error("NetworkManager should not be Core class")
	}
}
