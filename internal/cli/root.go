// Package cli wires up the llmenu commands (ADR-0002, section Commands).
package cli

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/schowave/llmenu/internal/config"
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

Supported tools:
  Claude Code   started with the chosen model          llmenu claude
  Codex CLI     started with the chosen model          llmenu codex
  opencode      models written into its config         llmenu sync, llmenu opencode
  pi            models written into its config         llmenu sync

Without a command, llmenu opens the menu and starts your default tool with
the chosen model (llmenu default), or only prints the name, e.g. when the
output is captured.`,
		Example: `  # First run: gateway, key and defaults (Enter keeps values on later runs)
  llmenu setup

  # Pick a model and start Claude Code
  llmenu claude

  # Use the picked model name in any other tool
  strix --model "$(llmenu)"`,
		Version:       buildVersion(),
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !isTerminal(os.Stdin) {
				return errors.New("no terminal for the menu; for scripts: llmenu list --json")
			}
			// The default tool only applies on a terminal: "$(llmenu)" keeps
			// printing the name.
			toTerminal := isTerminal(os.Stdout)
			if toTerminal {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				err = askDefaultOnce(cfg)
				if errors.Is(err, huh.ErrUserAborted) {
					return exitCodeError{130}
				}
				if err != nil {
					return err
				}
				if l, err := defaultLauncher(cfg); err != nil || l != nil {
					if err != nil {
						return err
					}
					return launch(cmd.Context(), l, nil)
				}
			}
			env, err := loadEnv(cmd.Context())
			if err != nil {
				return err
			}
			// The first run's setup may just have chosen a default tool.
			if l, err := defaultLauncher(env.Config); toTerminal && err == nil && l != nil {
				return launch(cmd.Context(), l, nil)
			}
			spend := startBudget(cmd.Context(), env)
			autoRefreshBench(cmd.Context(), env)
			m, err := chooseModel(env.Models, choice{config: env.Config, project: env.Project, spend: spend, interactive: true})
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
	root.SetUsageTemplate(root.UsageTemplate() + `{{if not .HasParent}}
Environment variables: llmenu help environment
Documentation:         https://github.com/schowave/llmenu#readme
Report a problem:      https://github.com/schowave/llmenu/issues
{{end}}`)
	root.SetCompletionCommandGroupID(groupOther)
	root.AddCommand(
		newRunCmd(),
		newSyncCmd(),
		newBenchCmd(),
		newListCmd(),
		newSecretsCmd(),
		newProjectCmd(),
		newDefaultCmd(),
		newResetCmd(),
		newStatusLineCmd(),
		newSetupCmd(),
		newConfigCmd(),
		newTargetsCmd(),
		newEnvironmentTopic(),
	)
	// Shorthands for run, one per launch target: llmenu claude …
	for _, t := range targets.All() {
		if l, ok := t.(targets.Launcher); ok {
			root.AddCommand(newLaunchCmd(l))
		}
	}
	// Launch shorthands for tools you do not use are hidden from the help
	// (they keep working). Without a config, nothing is hidden.
	var used []string
	if cfg, err := config.Load(); err == nil && cfg.ToolsConfigured() {
		used = cfg.ToolList()
	}
	for _, c := range root.Commands() {
		if g, ok := commandGroups[c.Name()]; ok {
			c.GroupID = g
		} else if c.GroupID == "" {
			c.GroupID = groupLaunch // launch shorthands from the registry
			c.Hidden = used != nil && !slices.Contains(used, c.Name())
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
	"project":    groupSettings,
	"default":    groupSettings,
	"reset":      groupSettings,
	"statusline": groupOther,
	"targets":    groupSettings,
	"help":       groupOther,
	"completion": groupOther,
}

// newEnvironmentTopic is a help topic: `llmenu help environment`.
func newEnvironmentTopic() *cobra.Command {
	return &cobra.Command{
		Use:   "environment",
		Short: "Environment variables that override settings",
		Long: `Environment variables take precedence over config.toml and the keyring.

  LLMENU_BASE_URL       gateway URL, e.g. https://llm.example.com/
  LLMENU_API_KEY        gateway key (wins over the project's key)
  LLMENU_PROJECT        project whose key to use, none for the main key
                        (default: llmenu project use)
  LLMENU_AA_API_KEY     Artificial Analysis key for llmenu bench
  LLMENU_CONFIG         path of config.toml (default: llmenu config path)
  LLMENU_CACHE_DIR      directory for benchmarks, budgets and the last selection
  LLMENU_ACCESSIBLE=1   line-based setup prompts, e.g. for screen readers

For tests and the development sandbox (see CONTRIBUTING.md):

  LLMENU_KEYRING_FILE      keep keys in this plain-text file instead of the keyring
  LLMENU_KEYRING_SERVICE   keyring service name (default llmenu)
  LLMENU_AA_URL            Artificial Analysis API URL`,
	}
}
