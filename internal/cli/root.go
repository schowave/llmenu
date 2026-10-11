// Package cli wires up the llmenu commands (ADR-0002, section Commands).
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/schowave/llmenu/internal/targets"
)

// version is set by goreleaser via -ldflags at release time.
var version = "dev"

// exitCodeError passes through the exit code of a launched tool.
type exitCodeError struct{ code int }

func (e exitCodeError) Error() string { return fmt.Sprintf("exit code %d", e.code) }

// Execute runs llmenu and returns the process exit code.
func Execute() int {
	err := newRootCmd().Execute()
	var ec exitCodeError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ec):
		return ec.code
	default:
		fmt.Fprintln(os.Stderr, "llmenu:", err)
		return 1
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "llmenu",
		Short: "Pick models from a LiteLLM gateway and wire them into coding tools",
		Long: `Without a subcommand, llmenu opens the menu and prints the selected
model name, e.g. for: strix --model "$(llmenu)"`,
		Version:       version,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !isTerminal(os.Stdin) {
				return errors.New("no terminal for the menu; for scripts: llmenu list --json")
			}
			env, err := loadEnv(cmd.Context())
			if err != nil {
				return err
			}
			m, err := chooseModel(env.Models, choice{config: env.Config, interactive: true})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), m.Name)
			return nil
		},
	}
	root.AddCommand(
		newRunCmd(),
		newSyncCmd(),
		newBenchCmd(),
		newListCmd(),
		newSecretsCmd(),
		newSetupCmd(),
		newConfigCmd(),
		newTargetsCmd(),
	)
	// Shorthands for run, one per launch target: llmenu claude …
	for _, t := range targets.All() {
		if l, ok := t.(targets.Launcher); ok {
			root.AddCommand(newLaunchCmd(l))
		}
	}
	return root
}
