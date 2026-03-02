// Package cli implements the solai command-line interface.
package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "solai",
	Short: "SolAI — Autonomous Solana AI agent",
	Long:  "SolAI is an autonomous Solana blockchain agent powered by LLMs and agentic tools.",
}

// Execute runs the root command and exits on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
