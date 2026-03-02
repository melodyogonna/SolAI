//go:build ignore

// fetch_bwrap.go downloads pre-built static bubblewrap binaries and writes them to:
//
//	../sandbox/linux_amd64/bwrap
//	../sandbox/linux_arm64/bwrap
//
// Run via:
//
//	go generate ./sandbox/...
//
// Binaries are sourced from https://github.com/VHSgunzo/bubblewrap-static
// (statically linked with musl, no dependencies). SHA256 checksums are
// hardcoded below and verified after download.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

const sourceVersion = "v0.11.0.2"

type archTarget struct {
	goarch   string
	url      string
	sha256   string
}

var targets = []archTarget{
	{
		goarch: "amd64",
		url:    "https://github.com/VHSgunzo/bubblewrap-static/releases/download/v0.11.0.2/bwrap-x86_64",
		sha256: "019dcf296d5f84f000b35db4f005900fa44ddead2723f932d63c022d24992ed7",
	},
	{
		goarch: "arm64",
		url:    "https://github.com/VHSgunzo/bubblewrap-static/releases/download/v0.11.0.2/bwrap-aarch64",
		sha256: "3bdcd048a3c06a22a4909c47f9acad5683a4097d25b0560a165428d4ba66bff4",
	},
}

func main() {
	// go generate sets CWD to the directory of the file containing the
	// //go:generate directive, which is solai-agent/sandbox/.
	sandboxDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("getwd: %v", err)
	}

	for _, t := range targets {
		outDir := filepath.Join(sandboxDir, "linux_"+t.goarch)
		outBin := filepath.Join(outDir, "bwrap")

		fmt.Printf("==> Downloading bwrap %s for linux/%s\n", sourceVersion, t.goarch)
		fmt.Printf("    %s\n", t.url)

		data, err := download(t.url)
		if err != nil {
			log.Fatalf("download %s: %v", t.url, err)
		}

		got := sha256hex(data)
		if got != t.sha256 {
			log.Fatalf("checksum mismatch for linux/%s:\n  got:      %s\n  expected: %s",
				t.goarch, got, t.sha256)
		}
		fmt.Printf("    SHA256 verified: %s\n", got)

		if err := os.MkdirAll(outDir, 0755); err != nil {
			log.Fatalf("mkdir %s: %v", outDir, err)
		}
		if err := os.WriteFile(outBin, data, 0755); err != nil {
			log.Fatalf("writing %s: %v", outBin, err)
		}
		fmt.Printf("    written to %s\n", outBin)
	}

	fmt.Printf("\nDone. bubblewrap %s binaries written to sandbox/linux_*/bwrap\n", sourceVersion)
}

func download(url string) ([]byte, error) {
	resp, err := http.Get(url) //nolint:gosec // URL is hardcoded above
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func sha256hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
