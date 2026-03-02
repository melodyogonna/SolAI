package tool

import (
	"encoding/json"
	"fmt"
	"os"
)

// Manifest represents the contents of a tool's manifest.json file.
// Every agentic tool directory must contain one.
type Manifest struct {
	// Name is the tool's unique identifier used by the LLM to invoke it.
	// Convention: lowercase with hyphens, e.g. "solana-balance".
	Name string `json:"name"`

	// Description is shown verbatim to the LLM to help it decide when to
	// use this tool. Should describe what the tool does and what input it expects.
	Description string `json:"description"`

	// Version is the semver string for the tool, e.g. "1.0.0".
	Version string `json:"version"`

	// Executable is the path to the binary or script, relative to the manifest
	// directory. E.g. "./solana-balance" or "./run.sh".
	Executable string `json:"executable"`
}

// LoadManifest reads and JSON-decodes the manifest.json at the given file path.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("reading manifest %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parsing manifest %s: %w", path, err)
	}
	return m, nil
}

// Validate checks that required Manifest fields are non-empty.
func (m Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("manifest missing required field: name")
	}
	if m.Description == "" {
		return fmt.Errorf("manifest %q missing required field: description", m.Name)
	}
	if m.Executable == "" {
		return fmt.Errorf("manifest %q missing required field: executable", m.Name)
	}
	return nil
}
