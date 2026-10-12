package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/schowave/llmenu/internal/config"
	"github.com/schowave/llmenu/internal/gateway"
	"github.com/schowave/llmenu/internal/secrets"
	"github.com/schowave/llmenu/internal/targets"
)

// Projects: one gateway, one key per cost project (ADR-0005). Without a
// project the main key applies, exactly as before projects existed.

// activeProject is the project whose key is used: LLMENU_PROJECT, else
// gateway.project; "" is the main key. An unknown project is an error.
func activeProject(cfg *config.Config) (string, error) {
	name := currentProject(cfg)
	if name != "" && !cfg.Gateway.HasProject(name) {
		return "", fmt.Errorf("unknown project %q (%s): llmenu project add %s", name, projectSource(cfg), name)
	}
	return name, nil
}

// currentProject is activeProject without any check, for status lines and
// prompts that must stay fast and quiet.
func currentProject(cfg *config.Config) string {
	switch v := os.Getenv(targets.ProjectEnv); v {
	case "":
		return cfg.Gateway.Project
	case secrets.NoProject:
		return ""
	default:
		return v
	}
}

func projectSource(cfg *config.Config) string {
	if os.Getenv(targets.ProjectEnv) != "" {
		return targets.ProjectEnv
	}
	return "gateway.project"
}

func newProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Switch the gateway key between projects",
		Long: `A project is a name with its own gateway key, e.g. one key per customer so
the gateway bills each project separately. URL and models stay the same.

Without a project the main key applies (llmenu secrets set gateway), as if
projects did not exist. llmenu project opens a list of the main key and all
projects; Enter switches. The choice holds for every later command and for
tools that ask llmenu for the key (pi). Running sessions keep their key.

LLMENU_PROJECT overrides the choice for one shell or command; none means the
main key.`,
		Example: `  llmenu project add acme
  llmenu project use acme
  llmenu project use --none
  LLMENU_PROJECT=acme llmenu claude`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !canPrompt() {
				return printCurrent(cmd.OutOrStdout())
			}
			err := pickProject(cmd.Context())
			if errors.Is(err, huh.ErrUserAborted) {
				return exitCodeError{130}
			}
			return err
		},
	}
	var none bool
	use := &cobra.Command{
		Use:               "use <project> | --none",
		ValidArgsFunction: completeFirst(projectNames),
		Short:             "Switch to a project, or with --none back to the main key",
		Args:              cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case none && len(args) == 0, len(args) == 1 && (args[0] == "-" || args[0] == secrets.NoProject):
				return useProject("")
			case len(args) == 1 && !none:
				return useProject(args[0])
			}
			return errors.New("name a project, or --none for the main key")
		},
	}
	use.Flags().BoolVar(&none, "none", false, "use the main key (short: llmenu project use -)")

	cmd.AddCommand(
		&cobra.Command{
			Use:   "current",
			Short: "Print the active project; nothing for the main key",
			Long: `Prints the active project, or nothing for the main key. It reads neither the
keyring nor the network, so it is fast enough for a shell prompt or status line.`,
			Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return printCurrent(cmd.OutOrStdout()) },
		},
		&cobra.Command{
			Use:   "list",
			Short: "List the main key and all projects",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				active := currentProject(cfg)
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "PROJECT\tKEY\tACTIVE")
				for _, p := range append([]string{""}, cfg.Gateway.Projects...) {
					name := p
					if p == "" {
						name = "(main key)"
					}
					mark := ""
					if p == active {
						mark = "*"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\n", name, keyStatus(p), orDash(mark))
				}
				return w.Flush()
			},
		},
		&cobra.Command{
			Use:   "add <project>",
			Short: "Add a project: its key is asked for hidden and checked with the gateway",
			Long: `Adds a project, or replaces its key. The key is read hidden from the terminal
or from stdin, never from arguments, and checked with the gateway before it is
stored in the keyring. Switch to it with llmenu project use.`,
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := secrets.CheckProject(args[0]); err != nil {
					return err
				}
				key, err := readSecret("Key for project " + args[0] + ": ")
				if err != nil {
					return err
				}
				return addProject(cmd.Context(), args[0], key)
			},
		},
		use,
		&cobra.Command{
			Use:               "remove <project>",
			Aliases:           []string{"rm"},
			ValidArgsFunction: completeFirst(projectNames),
			Short:             "Remove a project and its key; the main key is never touched",
			Args:              cobra.ExactArgs(1),
			RunE:              func(cmd *cobra.Command, args []string) error { return removeProject(args[0]) },
		},
	)
	return cmd
}

func printCurrent(w io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if p := currentProject(cfg); p != "" {
		fmt.Fprintln(w, p)
	}
	return nil
}

// keyStatus describes a project's key ("" is the main key) for lists.
func keyStatus(project string) string {
	v, src, err := secrets.Get(secrets.ProjectKey(project))
	switch {
	case err != nil:
		return "error: " + err.Error()
	case v == "":
		return "missing"
	}
	return "set (" + src + ")"
}

// checkKey asks the gateway whether it accepts the key.
func checkKey(ctx context.Context, cfg *config.Config, key string) error {
	base := os.Getenv("LLMENU_BASE_URL")
	if base == "" {
		base = cfg.Gateway.URL
	}
	if base == "" {
		return errNotConfigured
	}
	models, err := gateway.Client{BaseURL: base, APIKey: key}.Models(ctx)
	if err != nil {
		return fmt.Errorf("gateway check failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Gateway accepts the key, %d chat models.\n", len(models))
	return nil
}

// addProject checks the key, stores it and records the project. "" stores
// the main key.
func addProject(ctx context.Context, name, key string) error {
	if key == "" {
		return errors.New("empty key")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := checkKey(ctx, cfg, key); err != nil {
		return err
	}
	if err := secrets.Set(secrets.ProjectKey(name), key); err != nil {
		return err
	}
	if name == "" {
		return nil
	}
	cfg.Gateway.AddProject(name)
	return save(cfg)
}

// useProject switches to a project, "" to the main key. A project without a
// key is refused, so no command later fails on it.
func useProject(name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if name != "" {
		if !cfg.Gateway.HasProject(name) {
			return fmt.Errorf("unknown project %q: llmenu project add %s", name, name)
		}
		if v, _, err := secrets.Get(secrets.ProjectKey(name)); err != nil {
			return err
		} else if v == "" {
			return fmt.Errorf("no key for project %s: llmenu project add %s", name, name)
		}
	}
	cfg.Gateway.Project = name
	if _, err := cfg.Save(); err != nil {
		return err
	}
	if name == "" {
		fmt.Fprintln(os.Stderr, "Using the main key (no project).")
	} else {
		fmt.Fprintf(os.Stderr, "Using project %s.\n", name)
	}
	if env := os.Getenv(targets.ProjectEnv); env != "" {
		fmt.Fprintf(os.Stderr, "Note: %s=%s overrides this in the current shell.\n", targets.ProjectEnv, env)
	}
	return nil
}

func removeProject(name string) error {
	if name == "" || name == secrets.NoProject {
		return errors.New("the main key is not a project: llmenu secrets delete gateway")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.Gateway.HasProject(name) {
		return fmt.Errorf("unknown project %q, see llmenu project list", name)
	}
	if err := secrets.Delete(secrets.ProjectKey(name)); err != nil {
		return err
	}
	wasActive := cfg.Gateway.Project == name
	cfg.Gateway.RemoveProject(name)
	if _, err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Removed project %s.\n", name)
	if wasActive {
		fmt.Fprintln(os.Stderr, "Using the main key (no project).")
	}
	return nil
}

// addChoice is the picker entry that adds a project; no project name can
// start with "+".
const addChoice = "+add"

// pickProject lists the main key, every project and "add project"; the
// cursor starts on the active one. A missing key is asked for right away,
// so the choice never leaves a project without a key.
func pickProject(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	active := currentProject(cfg)
	label := func(text, project string) string {
		if project == active {
			text += "  ✓ active"
		}
		if v, _, err := secrets.Get(secrets.ProjectKey(project)); err == nil && v == "" {
			text += "  (key missing)"
		}
		return text
	}
	opts := []huh.Option[string]{huh.NewOption(label("Main key (no project)", ""), "")}
	for _, p := range cfg.Gateway.Projects {
		opts = append(opts, huh.NewOption(label(p, p), p))
	}
	opts = append(opts, huh.NewOption("+ add project…", addChoice))

	choice := active
	if err := runPrompt(huh.NewSelect[string]().Title("Gateway key").
		Description("Which key llmenu and your tools use from now on.").
		Options(opts...).Value(&choice)); err != nil {
		return err
	}

	if choice == addChoice {
		var name, key string
		err := runPrompt(asForm{huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Project name").Description("a-z, 0-9, '.', '_' and '-'").Value(&name).
				Validate(func(s string) error {
					if cfg.Gateway.HasProject(s) {
						return fmt.Errorf("project %s exists already", s)
					}
					return secrets.CheckProject(s)
				}),
			huh.NewInput().Title("Gateway key for the project").Description("Checked with the gateway, then stored in the keyring.").
				EchoMode(secretEcho()).Value(&key).Validate(required("key missing")),
		))})
		if err != nil {
			return err
		}
		if err := addProject(ctx, name, key); err != nil {
			return err
		}
		return useProject(name)
	}

	if v, _, err := secrets.Get(secrets.ProjectKey(choice)); err != nil {
		return err
	} else if v == "" {
		what := "the main key"
		if choice != "" {
			what = "project " + choice
		}
		var key string
		if err := runPrompt(huh.NewInput().Title("Gateway key for " + what).
			Description("Checked with the gateway, then stored in the keyring.").
			EchoMode(secretEcho()).Value(&key).Validate(required("key missing"))); err != nil {
			return err
		}
		if err := addProject(ctx, choice, key); err != nil {
			return err
		}
	}
	return useProject(choice)
}

func required(msg string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return errors.New(msg)
		}
		return nil
	}
}

// newStatusLineCmd prints the status line for Claude Code (claude.statusline):
// the session's project and the model. Claude Code passes session data as
// JSON on stdin.
func newStatusLineCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "statusline",
		Short:  "Status line for Claude Code: project and model",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var session struct {
				Model struct {
					DisplayName string `json:"display_name"`
				} `json:"model"`
			}
			_ = json.NewDecoder(cmd.InOrStdin()).Decode(&session) // a status line never fails
			cfg, err := config.Load()
			if err != nil {
				cfg = &config.Config{}
			}
			var parts []string
			if p := currentProject(cfg); p != "" {
				parts = append(parts, "project "+p)
			}
			if m := session.Model.DisplayName; m != "" {
				parts = append(parts, m)
			}
			fmt.Fprintln(cmd.OutOrStdout(), strings.Join(parts, " · "))
			return nil
		},
	}
}
