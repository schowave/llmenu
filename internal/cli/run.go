package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/config"
	"github.com/schowave/llmenu/internal/menu"
	"github.com/schowave/llmenu/internal/state"
	"github.com/schowave/llmenu/internal/targets"
)

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <target> [args…]",
		Short: "Launch a tool with the gateway and a model",
		Long: `Everything after the target name is passed to the tool unchanged, including --help.
If the arguments already contain a model, no menu opens.`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
				return cmd.Help()
			}
			t, ok := targets.Get(args[0])
			if !ok {
				return fmt.Errorf("unknown target %q, see llmenu targets", args[0])
			}
			l, ok := t.(targets.Launcher)
			if !ok {
				return fmt.Errorf("%s cannot be launched, only synced: llmenu sync %s", t.Name(), t.Name())
			}
			return launch(cmd.Context(), l, args[1:])
		},
	}
}

func newLaunchCmd(l targets.Launcher) *cobra.Command {
	return &cobra.Command{
		Use:                l.Name() + " [args…]",
		Short:              l.Description() + " (shorthand for run " + l.Name() + ")",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return launch(cmd.Context(), l, args)
		},
	}
}

func launch(ctx context.Context, l targets.Launcher, args []string) error {
	if _, ok := targets.Installed(l); !ok {
		return fmt.Errorf("%s not found on PATH", l.Binary())
	}
	env, err := loadEnv(ctx)
	if err != nil {
		return err
	}

	var model *catalog.Model
	if _, own := l.(targets.OwnSelection); !own && !l.HasModelArg(args) {
		var def string
		if d, ok := l.(targets.Defaulter); ok {
			def = d.DefaultModel(env)
		}
		model, err = chooseModel(env.Models, choice{
			config:      env.Config,
			key:         l.Name(),
			defaultName: def,
			supports:    l.Supports,
			interactive: isTerminal(os.Stdin) && isTerminal(os.Stdout),
		})
		if err != nil {
			return err
		}
	}

	cmd, err := l.Command(ctx, model, args, env)
	if err != nil {
		return err
	}
	return runChild(cmd)
}

// choice describes what a model is being chosen for.
type choice struct {
	config      *config.Config
	key         string // key for the last chosen model, usually the target name
	defaultName string // the target's default; empty if it has none
	supports    func(catalog.Model) (bool, string)
	interactive bool
}

// chooseModel opens the menu with the cursor on the default, or without a
// default on the last chosen model. Without a terminal the default applies
// without asking; if there is none, nil, and the tool decides itself (ADR-0002).
func chooseModel(all []catalog.Model, c choice) (*catalog.Model, error) {
	var shown []catalog.Model
	hints := map[string]string{}
	for _, m := range catalog.Dedupe(all) {
		ok, hint := true, ""
		if c.supports != nil {
			ok, hint = c.supports(m)
		}
		if ok {
			shown = append(shown, m)
			hints[m.Name] = hint
		}
	}
	sortModels(shown, c.config)

	if !c.interactive {
		if i := catalog.Resolve(all, shown, c.defaultName); c.defaultName != "" && i >= 0 {
			return &shown[i], nil
		}
		return nil, nil
	}

	st := state.Load()
	i, err := menu.Pick(shown, menu.Options{
		Note:      benchNote(),
		Preselect: catalog.Resolve(all, shown, cmp.Or(c.defaultName, st.LastModel[c.key])),
		Hint:      func(m catalog.Model) string { return hints[m.Name] },
	})
	if errors.Is(err, menu.ErrAborted) {
		return nil, exitCodeError{130}
	}
	if err != nil {
		return nil, err
	}
	st.LastModel[c.key] = shown[i].Name
	if err := st.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "llmenu: selection not saved:", err)
	}
	return &shown[i], nil
}

// runChild starts the tool in the same terminal and passes its exit code
// through. Ctrl-C reaches the tool via the process group; llmenu itself
// ignores it while the tool is running.
func runChild(cmd *exec.Cmd) error {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)

	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return exitCodeError{ee.ExitCode()}
	}
	return err
}
