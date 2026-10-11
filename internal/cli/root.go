// Package cli wires up the llmenu commands (ADR-0002, section Commands).
package cli

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/schowave/llmenu/internal/targets"
)

// version is set by goreleaser via -ldflags at release time.
var version = "dev"

// buildVersion is the goreleaser version, or for `go install …@v1.2.3` the
// module version Go records in the binary.
func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return version
}

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

const (
	groupStart    = "start"
	groupLaunch   = "launch"
	groupModels   = "models"
	groupSync     = "sync"
	groupSettings = "settings"
	groupOther    = "other"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "llmenu",
		Short: "Pick LLMs from a LiteLLM gateway and use them in coding tools",
		Long: `llmenu reads the models of your LiteLLM gateway with context size, prices
and benchmark scores, lets you pick one in a menu and starts your coding tool
with it, or writes the model list into the tool's configuration.

Without a command, llmenu just opens the menu and prints the chosen name:
  strix --model "$(llmenu)"

Getting started:
  llmenu setup     set up gateway, key and defaults (repeatable)
  llmenu claude    pick a model and start Claude Code

Supported tools:
  Claude Code   llmenu claude starts it with the chosen model
  Codex CLI     llmenu codex starts it with the chosen model
  opencode      llmenu sync writes the models into its config,
                llmenu opencode starts it with the gateway key
  pi            llmenu sync writes the models into its config;
                pi then picks the model and gets the key from llmenu`,
		Version:       buildVersion(),
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
	// Group the help output by task instead of one alphabetical list.
	root.AddGroup(
		&cobra.Group{ID: groupStart, Title: "Getting started:"},
		&cobra.Group{ID: groupLaunch, Title: "Pick a model and start a tool:"},
		&cobra.Group{ID: groupModels, Title: "Models and benchmarks:"},
		&cobra.Group{ID: groupSync, Title: "Write models into tool configs:"},
		&cobra.Group{ID: groupSettings, Title: "Settings and keys:"},
		&cobra.Group{ID: groupOther, Title: "Other:"},
	)
	root.SetHelpCommandGroupID(groupOther)
	root.SetCompletionCommandGroupID(groupOther)
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
	for _, c := range root.Commands() {
		if g, ok := commandGroups[c.Name()]; ok {
			c.GroupID = g
		} else if c.GroupID == "" {
			c.GroupID = groupLaunch // launch shorthands from the registry
		}
	}
	return root
}

var commandGroups = map[string]string{
	"setup":      groupStart,
	"run":        groupLaunch,
	"list":       groupModels,
	"bench":      groupModels,
	"sync":       groupSync,
	"config":     groupSettings,
	"secrets":    groupSettings,
	"targets":    groupSettings,
	"help":       groupOther,
	"completion": groupOther,
}
