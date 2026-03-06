package cli

import (
	"fmt"

	"github.com/melodyogonna/solai/solai-agent/config"
	"github.com/melodyogonna/solai/solai-agent/registry"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install <name[@tag] | owner/repo[@tag]>",
	Short: "Install a tool from the registry or a GitHub release",
	Long: `Install downloads a tool into ~/.solai/tools/.

Short names are resolved via the curated registry index:
  solai install token-price
  solai install token-price@v1.0.0

Third-party tools are installed directly from a GitHub release:
  solai install melodyogonna/my-tool
  solai install melodyogonna/my-tool@v1.0.0

The release must include:
  manifest.json          — tool manifest (executable must be "./bin/<name>")
  <name>-linux-amd64     — AMD64 binary
  <name>-linux-arm64     — ARM64 binary
  checksums.txt          — (optional) SHA256 checksums`,
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
