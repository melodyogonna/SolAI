package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/melodyogonna/solai/solai-agent/config"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall <name>",
	Short: "Remove an installed tool",
	Args:  cobra.ExactArgs(1),
	RunE:  runUninstall,
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}

func runUninstall(cmd *cobra.Command, args []string) error {
	if err := removeToolDir(config.ToolsDir(), args[0]); err != nil {
		return err
	}
	fmt.Printf("Uninstalled %s.\n", args[0])
	return nil
}

func removeToolDir(toolsDir, name string) error {
	toolDir := filepath.Join(toolsDir, name)
	if _, err := os.Stat(toolDir); os.IsNotExist(err) {
		return fmt.Errorf("tool %q is not installed", name)
	}
	if err := os.RemoveAll(toolDir); err != nil {
		return fmt.Errorf("uninstalling %q: %w", name, err)
	}
	return nil
}
