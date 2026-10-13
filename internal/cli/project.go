package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
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

// activeProject is the project whose key is used: LLMENU_PROJECT, else the
// folder's project (gateway.dirs), else gateway.project; "" is the main key.
// An unknown project is an error.
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
	v := os.Getenv(targets.ProjectEnv)
	if v == "" {
		p, _, ok := folderProject(cfg)
		if !ok {
			return cfg.Gateway.Project
		}
		v = p
	}
	switch v {
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
	if _, dir, ok := folderProject(cfg); ok {
		return "folder " + tildePath(dir)
	}
	return "gateway.project"
}

// folderProject returns the folder binding that applies in the current
// directory; project "none" is the main key.
func folderProject(cfg *config.Config) (project, dir string, ok bool) {
	if len(cfg.Gateway.Dirs) == 0 {
		return "", "", false
	}
	wd := workDir()
	if wd == "" {
		return "", "", false
	}
	return cfg.Gateway.ProjectFor(wd)
}

// workDir is the current directory the way bindings store folders.
func workDir() string {
	d, err := os.Getwd()
	if err != nil {
		return ""
	}
	return realPath(d)
}

// realPath makes a folder absolute and resolves symlinks, so /tmp and
// /private/tmp are the same folder.
func realPath(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		dir = r
	}
	return filepath.Clean(dir)
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return realPath(home)
}

// tildePath shortens a folder in the home directory to ~/….
func tildePath(dir string) string {
	home := homeDir()
	if home == "" {
		return dir
	}
	rel, err := filepath.Rel(home, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return dir
	}
	if rel == "." {
		return "~"
	}
	return "~" + string(filepath.Separator) + rel
}

// bindable reports whether the picker offers to bind dir: not the home
// directory or a root, where a binding would hardly differ from the global
// choice.
func bindable(dir string) bool {
	return dir != "" && dir != homeDir() && filepath.Dir(dir) != dir
}

// projectLabel names a project for messages; "" and "none" are the main key.
func projectLabel(p string) string {
	if p == "" || p == secrets.NoProject {
		return "the main key"
	}
	return "project " + p
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

Outside the home directory the picker also asks whether the choice applies
everywhere or only in the current folder and below. A folder's project wins
over the global choice; choosing again replaces it, unpin removes it.

LLMENU_PROJECT overrides both for one shell or command; none means the main
key.`,
		Example: `  llmenu project add acme
  llmenu project use acme
  llmenu project use --none
  llmenu project use acme --here
  llmenu project unpin
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
	var none, here bool
	use := &cobra.Command{
		Use:               "use <project> | --none",
		ValidArgsFunction: completeFirst(projectNames),
		Short:             "Switch to a project, or with --none back to the main key",
		Args:              cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case none && len(args) == 0, len(args) == 1 && (args[0] == "-" || args[0] == secrets.NoProject):
				return useProject("", here)
			case len(args) == 1 && !none:
				return useProject(args[0], here)
			}
			return errors.New("name a project, or --none for the main key")
		},
	}
	use.Flags().BoolVar(&none, "none", false, "use the main key (short: llmenu project use -)")
	use.Flags().BoolVar(&here, "here", false, "only in the current folder and below; replaces its earlier choice")

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
				activeMark := "*"
				if _, _, ok := folderProject(cfg); ok && os.Getenv(targets.ProjectEnv) == "" {
					activeMark = "* (folder)"
				}
				all := append([]string{""}, cfg.Gateway.Projects...)
				spends := projectSpends(cmd.Context(), cfg, all)
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "PROJECT\tKEY\tACTIVE\tSPEND")
				for i, p := range all {
					name := p
					if p == "" {
						name = "(main key)"
					}
					mark := ""
					if p == active {
						mark = activeMark
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, keyStatus(p), orDash(mark), orDash(spends[i]))
				}
				if err := w.Flush(); err != nil {
					return err
				}
				if len(cfg.Gateway.Dirs) == 0 {
					return nil
				}
				fmt.Fprintln(cmd.OutOrStdout())
				w = tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "FOLDER\tPROJECT")
				for _, d := range boundDirs(cfg) {
					p := cfg.Gateway.Dirs[d]
					if p == secrets.NoProject {
						p = "(main key)"
					}
					fmt.Fprintf(w, "%s\t%s\n", tildePath(d), p)
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
			Use:               "unpin [folder]",
			ValidArgsFunction: completeFirst(boundFolders),
			Short:             "Remove a folder's project; the global choice applies there again",
			Long: `Removes the project bound to a folder (default: the current one) with
llmenu project use --here or the picker. The folder then follows the global
choice again, or the binding of a folder above it.`,
			Args: cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				dir := workDir()
				if len(args) == 1 {
					dir = realPath(args[0])
				}
				return unpin(dir)
			},
		},
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

// useProject switches to a project, "" to the main key; with here only in
// the current folder and below. A project without a key is refused, so no
// command later fails on it.
func useProject(name string, here bool) error {
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
	if here {
		return bindHere(cfg, name)
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
	if p, dir, ok := folderProject(cfg); ok && p != cmp.Or(name, secrets.NoProject) {
		fmt.Fprintf(os.Stderr, "Note: %s uses %s; llmenu project unpin removes that.\n", tildePath(dir), projectLabel(p))
	}
	if env := os.Getenv(targets.ProjectEnv); env != "" {
		fmt.Fprintf(os.Stderr, "Note: %s=%s overrides this in the current shell.\n", targets.ProjectEnv, env)
	}
	return nil
}

// bindHere binds the current folder to a project, "" to the main key.
func bindHere(cfg *config.Config, name string) error {
	dir := workDir()
	if dir == "" {
		return errors.New("cannot determine the current folder")
	}
	value := cmp.Or(name, secrets.NoProject)
	old, had := cfg.Gateway.Dirs[dir]
	cfg.Gateway.Bind(dir, value)
	if _, err := cfg.Save(); err != nil {
		return err
	}
	if had && old != value {
		fmt.Fprintf(os.Stderr, "%s: %s → %s.\n", tildePath(dir), projectLabel(old), projectLabel(value))
	} else {
		fmt.Fprintf(os.Stderr, "Using %s in %s.\n", projectLabel(value), tildePath(dir))
	}
	if env := os.Getenv(targets.ProjectEnv); env != "" {
		fmt.Fprintf(os.Stderr, "Note: %s=%s overrides this in the current shell.\n", targets.ProjectEnv, env)
	}
	return nil
}

// unpin removes a folder's own binding.
func unpin(dir string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.Gateway.Unbind(dir) {
		if _, parent, ok := cfg.Gateway.ProjectFor(dir); ok {
			return fmt.Errorf("%s has no project of its own, it follows %s: llmenu project unpin %s",
				tildePath(dir), tildePath(parent), tildePath(parent))
		}
		return fmt.Errorf("%s has no project of its own", tildePath(dir))
	}
	if _, err := cfg.Save(); err != nil {
		return err
	}
	now := cfg.Gateway.Project
	if p, _, ok := cfg.Gateway.ProjectFor(dir); ok {
		now = p
	}
	fmt.Fprintf(os.Stderr, "Removed the project of %s; it uses %s now.\n", tildePath(dir), projectLabel(now))
	return nil
}

// boundDirs are the bound folders, sorted.
func boundDirs(cfg *config.Config) []string {
	dirs := make([]string, 0, len(cfg.Gateway.Dirs))
	for d := range cfg.Gateway.Dirs {
		dirs = append(dirs, d)
	}
	slices.Sort(dirs)
	return dirs
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
	p, _, _ := folderProject(cfg)
	folderBound := p == name
	cfg.Gateway.RemoveProject(name)
	if _, err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Removed project %s.\n", name)
	if wasActive || folderBound {
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
	activeMark := "  ✓ active"
	if _, _, ok := folderProject(cfg); ok && os.Getenv(targets.ProjectEnv) == "" {
		activeMark += " (folder)"
	}
	label := func(text, project string) string {
		if project == active {
			text += activeMark
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
		return applyChoice(name)
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
	return applyChoice(choice)
}

// Scopes of a choice in the picker.
const (
	scopeHere   = "here"
	scopeGlobal = "global"
	scopeUnpin  = "unpin"
)

// applyChoice asks, outside the home directory, whether the chosen project
// applies only in the current folder or everywhere, and switches.
func applyChoice(project string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	dir := workDir()
	if !bindable(dir) {
		return useProject(project, false)
	}
	where := tildePath(dir)
	scope := scopeGlobal
	if _, _, ok := cfg.Gateway.ProjectFor(dir); ok {
		scope = scopeHere
	}
	opts := []huh.Option[string]{
		huh.NewOption("Only in "+where+" and below", scopeHere),
		huh.NewOption("Everywhere", scopeGlobal),
	}
	if _, own := cfg.Gateway.Dirs[dir]; own {
		opts = append(opts, huh.NewOption("Remove the project of "+where+" (use the global choice, "+
			strings.TrimPrefix(projectLabel(cfg.Gateway.Project), "project ")+")", scopeUnpin))
	}
	if err := runPrompt(huh.NewSelect[string]().Title("Applies to").
		Options(opts...).Value(&scope)); err != nil {
		return err
	}
	if scope == scopeUnpin {
		return unpin(dir)
	}
	return useProject(project, scope == scopeHere)
}

func required(msg string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return errors.New(msg)
		}
		return nil
	}
}
