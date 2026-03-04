package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	solaiconfig "github.com/melodyogonna/solai/solai-agent/config"
	"github.com/melodyogonna/solai/solai-agent/sandbox"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the autonomous agent",
	Long: `Start launches the SolAI agent.

By default the agent runs inside a bubblewrap (bwrap) sandbox for isolation.
Use --no-sandbox to skip sandboxing (useful for debugging).

Prerequisites:
  solai config set model.provider <google|openai|anthropic>
  solai config set model.name <model-name>
  solai config set provider.<name> <api-key>
  solai config set user-goals "Monitor SOL price and report daily"`,
	Args: cobra.NoArgs,
	RunE: runStart,
}

func init() {
	startCmd.Flags().Bool("no-sandbox", false, "Run without bwrap sandbox (debug mode)")
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	cfg, err := solaiconfig.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if cfg.Model.Provider == "" || cfg.Model.Name == "" {
		return fmt.Errorf("model not configured; run:\n  solai config set model.provider <google|openai|anthropic>\n  solai config set model.name <model-name>")
	}
	if cfg.Providers[cfg.Model.Provider] == "" {
		return fmt.Errorf("no API key for provider %q; run: solai config set provider.%s <key>", cfg.Model.Provider, cfg.Model.Provider)
	}
	if cfg.UserGoals == "" {
		return fmt.Errorf("user_goals is not configured; run: solai config set user-goals \"<goals>\"")
	}

	noSandbox, _ := cmd.Flags().GetBool("no-sandbox")
	if noSandbox {
		return agentRun(cmd.Context(), cfg, solaiconfig.ToolsDir())
	}

	return runWithSandbox(cfg)
}

// runWithSandbox extracts bwrap, builds sandbox args, and launches the agent
// inside the sandbox by re-invoking this binary with the __agent-run subcommand.
func runWithSandbox(cfg *solaiconfig.SolaiConfig) error {
	bwrapPath, err := sandbox.Extract()
	if err != nil {
		return fmt.Errorf("extracting sandbox binary: %w\n(run with --no-sandbox to skip sandboxing)", err)
	}
	defer os.Remove(bwrapPath)

	selfExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	configPath := solaiconfig.ConfigPath()
	toolsDir := solaiconfig.ToolsDir()

	bwrapArgs := buildAgentBwrapArgs(cfg, selfExe, configPath, toolsDir)
	bwrapArgs = append(bwrapArgs, "/solai", "__agent-run")

	bwrapCmd := exec.Command(bwrapPath, bwrapArgs...)
	bwrapCmd.Stdin = os.Stdin
	bwrapCmd.Stdout = os.Stdout
	bwrapCmd.Stderr = os.Stderr

	if err := bwrapCmd.Start(); err != nil {
		return fmt.Errorf("starting bwrap: %w", err)
	}

	// Forward SIGINT and SIGTERM to the bwrap child.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		for sig := range sigCh {
			if bwrapCmd.Process != nil {
				_ = bwrapCmd.Process.Signal(sig)
			}
		}
	}()

	waitErr := bwrapCmd.Wait()
	signal.Stop(sigCh)
	close(sigCh)

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("bwrap exited with error: %w", waitErr)
	}
	return nil
}

// buildAgentBwrapArgs constructs the bwrap argument list for the agent sandbox.
// The returned slice does not include the "--" separator or the command to run;
// the caller appends those.
func buildAgentBwrapArgs(cfg *solaiconfig.SolaiConfig, selfExe, configPath, toolsDir string) []string {
	args := []string{"--unshare-all"}

	if cfg.Sandbox.ShareNet {
		args = append(args, "--share-net")
	}

	args = append(args,
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--ro-bind", configPath, "/etc/solai/config.json",
		"--ro-bind", toolsDir, "/tools",
		"--ro-bind", selfExe, "/solai",
		"--die-with-parent",
	)

	// Bind essential network/TLS files so the agent can make LLM API calls.
	for _, p := range []string{
		"/etc/resolv.conf",
		"/etc/nsswitch.conf",
		"/etc/ssl/certs",
		"/etc/ca-certificates.conf",
		"/etc/pki/tls/certs",
	} {
		if _, err := os.Stat(p); err == nil {
			args = append(args, "--ro-bind", p, p)
		}
	}

	// User-configured extra binds.
	for _, bind := range cfg.Sandbox.ExtraBinds {
		if bind.ReadOnly {
			args = append(args, "--ro-bind", bind.Path, bind.Path)
		} else {
			args = append(args, "--bind", bind.Path, bind.Path)
		}
	}

	args = append(args, "--")
	return args
}
