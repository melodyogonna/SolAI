package cli

import (
	"fmt"

	"github.com/melodyogonna/solai/solai-agent/config"
	"github.com/melodyogonna/solai/solai-agent/registry"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install <owner/repo[@tag]>",
	Short: "Install a tool from a GitHub release",
	Long: `Install downloads a tool from a GitHub release into ~/.solai/tools/.

The release must include:
  manifest.json          — tool manifest (executable must be "./bin/<name>")
  <name>-linux-amd64     — AMD64 binary
  <name>-linux-arm64     — ARM64 binary
  checksums.txt          — (optional) SHA256 checksums

Examples:
  solai install melodyogonna/token-price
  solai install melodyogonna/token-price@v1.0.0`,
	Args: cobra.ExactArgs(1),
	RunE: runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	ref := args[0]
	toolsDir := config.ToolsDir()

	fmt.Printf("Installing %s into %s ...\n", ref, toolsDir)
	if err := registry.Install(ref, toolsDir); err != nil {
		return fmt.Errorf("install failed: %w", err)
	}
	fmt.Printf("Installed %s successfully.\n", ref)
	return nil
}
