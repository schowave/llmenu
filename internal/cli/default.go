package cli

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/schowave/llmenu/internal/config"
	"github.com/schowave/llmenu/internal/targets"
)

// The default tool: what llmenu without a command starts after the menu
// (config key launch). Unset, llmenu asks once.

func init() {
	config.ValidLaunch = func(name string) error {
		_, err := launcherNamed(name)
		return err
	}
}

// launcherNamed returns the tool for launch, or an error naming the problem.
func launcherNamed(name string) (targets.Launcher, error) {
	t, ok := targets.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown tool %q, see llmenu targets", name)
	}
	l, ok := t.(targets.Launcher)
	if !ok {
		return nil, fmt.Errorf("%s cannot be started, only synced: llmenu sync %s", name, name)
	}
	return l, nil
}

// defaultLauncher is the tool llmenu without a command starts; nil: only
// the menu.
func defaultLauncher(cfg *config.Config) (targets.Launcher, error) {
	if cfg.Launch == "" || cfg.Launch == config.NoLaunch {
		return nil, nil
	}
	l, err := launcherNamed(cfg.Launch)
	if err != nil {
		return nil, fmt.Errorf("launch = %q: %w; change it with llmenu default", cfg.Launch, err)
	}
	return l, nil
}

// launchChoices are the tools the default can be: the ones in use that
// llmenu can start, plus the current default if it is not among them.
func launchChoices(cfg *config.Config) []string {
	var out []string
	for _, name := range targetNames(isLauncher) {
		if toolEnabled(cfg, name) || name == cfg.Launch {
			out = append(out, name)
		}
	}
	return out
}

var launchLabels = map[string]string{
	"claude":   "Start Claude Code",
	"codex":    "Start Codex CLI",
	"opencode": "Start opencode",
}

func launchLabel(name string) string {
	if name == "" || name == config.NoLaunch {
		return "Only pick a model and print its name"
	}
	if l := launchLabels[name]; l != "" {
		return l
	}
	return "Start " + name
}

// pickDefault asks what llmenu without a command does and stores it. Before
// the first choice the cursor is on the first tool.
func pickDefault(cfg *config.Config, description string) error {
	tools := launchChoices(cfg)
	choice := cfg.Launch
	if choice == "" {
		choice = config.NoLaunch
		if len(tools) > 0 {
			choice = tools[0]
		}
	}
	var opts []huh.Option[string]
	for _, name := range append(tools, config.NoLaunch) {
		label := launchLabel(name)
		if name == cfg.Launch {
			label += "  ✓ current"
		}
		opts = append(opts, huh.NewOption(label, name))
	}
	if err := runPrompt(huh.NewSelect[string]().Title("What should `llmenu` without a command do?").
		Description(description).Options(opts...).Value(&choice)); err != nil {
		return err
	}
	cfg.Launch = choice
	return save(cfg)
}

func newDefaultCmd() *cobra.Command {
	var none bool
	cmd := &cobra.Command{
		Use:               "default [<tool> | --none]",
		ValidArgsFunction: completeFirst(func() []string { return targetNames(isLauncher) }),
		Short:             "Choose the tool llmenu without a command starts",
		Long: `llmenu without a command opens the menu and then starts this tool, like
llmenu claude. With --none it only prints the chosen model name.

Without arguments a list opens. Arguments for the tool still need the long
form, e.g. llmenu claude -c. When the output of llmenu is captured, as in
"$(llmenu)", it always prints only the name.`,
		Example: `  llmenu default
  llmenu default claude
  llmenu default --none`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			switch {
			case len(args) == 1 && none:
				return errors.New("name a tool or --none, not both")
			case none, len(args) == 1 && (args[0] == "-" || args[0] == config.NoLaunch):
				return setDefault(cfg, config.NoLaunch)
			case len(args) == 1:
				return setDefault(cfg, args[0])
			case !canPrompt():
				return printDefault(cmd.OutOrStdout(), cfg)
			}
			err = pickDefault(cfg, "")
			if errors.Is(err, huh.ErrUserAborted) {
				return exitCodeError{130}
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&none, "none", false, "only print the model name (short: llmenu default -)")
	return cmd
}

func setDefault(cfg *config.Config, name string) error {
	if name != config.NoLaunch {
		if _, err := launcherNamed(name); err != nil {
			return err
		}
	}
	cfg.Launch = name
	if err := save(cfg); err != nil {
		return err
	}
	if name == config.NoLaunch {
		fmt.Fprintln(os.Stderr, "llmenu without a command now only prints the model name.")
	} else {
		fmt.Fprintf(os.Stderr, "llmenu without a command now starts %s.\n", name)
	}
	if !toolEnabled(cfg, name) && name != config.NoLaunch {
		fmt.Fprintf(os.Stderr, "Note: %s is not among your tools; llmenu setup tools adds it.\n", name)
	}
	return nil
}

// printDefault prints the default tool, or none.
func printDefault(w io.Writer, cfg *config.Config) error {
	fmt.Fprintln(w, cmp.Or(cfg.Launch, config.NoLaunch))
	return nil
}

// askDefaultOnce asks for the default the first time llmenu runs without a
// command in a terminal, if there is a tool to start. On the very first run
// setup asks instead.
func askDefaultOnce(cfg *config.Config) error {
	if cfg.Launch != "" || firstRun() || len(launchChoices(cfg)) == 0 {
		return nil
	}
	return pickDefault(cfg, "Change it later with llmenu default.")
}
