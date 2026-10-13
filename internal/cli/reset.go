package cli

import (
	"cmp"
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
	"github.com/schowave/llmenu/internal/secrets"
	"github.com/schowave/llmenu/internal/state"
	"github.com/schowave/llmenu/internal/targets"
)

func newResetCmd() *cobra.Command {
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Remove all settings, keys and caches, e.g. to set up anew or before uninstalling",
		Long: `Removes everything llmenu stored: config.toml, all keys in the keyring
(main key, projects, benchmark key), the caches, and the provider entry
llmenu sync wrote into pi and opencode (the previous file stays as *.bak).
Entries in those files that llmenu did not write are left alone.

It lists what it removes and asks first. Afterwards llmenu setup starts
afresh; to uninstall, remove the program as you installed it.`,
		Example: `  llmenu reset --dry-run
  llmenu reset
  llmenu reset --yes`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			items, err := resetItems()
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Fprintln(os.Stderr, "Nothing to remove: llmenu stores nothing yet.")
				return nil
			}
			printResetItems(os.Stderr, items)
			if dryRun {
				return nil
			}
			if !yes {
				if !canPrompt() {
					return errors.New("no terminal to confirm: llmenu reset --yes")
				}
				ok := false
				err := runPrompt(huh.NewConfirm().Title("Remove all of this?").
					Affirmative("Yes, remove").Negative("No").Value(&ok))
				if errors.Is(err, huh.ErrUserAborted) || err == nil && !ok {
					fmt.Fprintln(os.Stderr, "Nothing removed.")
					return exitCodeError{1}
				}
				if err != nil {
					return err
				}
			}
			var failed []string
			for _, it := range items {
				if err := it.remove(); err != nil {
					fmt.Fprintf(os.Stderr, "%s: %v\n", it.what, err)
					failed = append(failed, it.what)
				}
			}
			if len(failed) > 0 {
				return fmt.Errorf("not removed: %s", strings.Join(failed, ", "))
			}
			fmt.Fprintln(os.Stderr, "Removed. llmenu setup sets llmenu up again.")
			for _, v := range []string{"LLMENU_API_KEY", "LLMENU_AA_API_KEY", "LLMENU_BASE_URL", targets.ProjectEnv} {
				if os.Getenv(v) != "" {
					fmt.Fprintf(os.Stderr, "Note: %s is still set in your environment.\n", v)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "only list what would be removed")
	return cmd
}

// resetItem is one thing llmenu reset removes.
type resetItem struct {
	what, detail string
	remove       func() error
}

func printResetItems(w io.Writer, items []resetItem) {
	fmt.Fprintln(w, "llmenu reset removes:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, it := range items {
		fmt.Fprintf(tw, "  %s\t%s\n", it.what, it.detail)
	}
	_ = tw.Flush()
}

// resetItems collects what llmenu stored, in the order it is removed: the
// tools' entries and keys need the config, so it goes last but one.
func resetItems() ([]resetItem, error) {
	cfg, err := config.Load()
	if err != nil {
		// A broken config is removed too; the rest still works without it.
		fmt.Fprintln(os.Stderr, "Warning:", err)
		cfg = &config.Config{}
	}
	var items []resetItem

	provider := cfg.Gateway.Name
	if provider == "" {
		provider = config.NameFromURL(cmp.Or(os.Getenv("LLMENU_BASE_URL"), cfg.Gateway.URL))
	}
	opt := targets.SyncOptions{Provider: provider}
	for _, t := range targets.All() {
		s, ok := t.(targets.Syncer)
		if !ok {
			continue
		}
		dry := opt
		dry.DryRun = true
		d, err := s.Unsync(dry)
		switch {
		case errors.Is(err, targets.ErrNotOwned):
			fmt.Fprintf(os.Stderr, "%s: provider %q in %s was not written by llmenu; it stays.\n", t.Name(), provider, tildePath(d.Path))
		case err != nil:
			return nil, fmt.Errorf("%s: %w", t.Name(), err)
		case d.Changed:
			items = append(items, resetItem{t.Name(), fmt.Sprintf("provider %q in %s", provider, tildePath(d.Path)),
				func() error { _, err := s.Unsync(opt); return err }})
		}
	}

	names := []string{secrets.Gateway}
	for _, p := range cfg.Gateway.Projects {
		names = append(names, secrets.ProjectKey(p))
	}
	names = append(names, secrets.ArtificialAnalysis)
	var stored []string
	for _, n := range names {
		ok, err := secrets.Stored(n)
		if err != nil {
			return nil, err
		}
		if ok {
			stored = append(stored, n)
		}
	}
	origin, err := secrets.Origin()
	if err != nil {
		return nil, err
	}
	if len(stored) > 0 || origin != "" {
		label := keyringLabel()
		items = append(items, resetItem{"keys", strings.Join(stored, ", ") + " (" + label + ")", func() error {
			for _, n := range stored {
				if err := secrets.Delete(n); err != nil {
					return err
				}
			}
			return secrets.DeleteOrigin()
		}})
		if len(stored) == 0 {
			items[len(items)-1].detail = "gateway origin (" + label + ")"
		}
	}

	if p, err := config.Path(); err == nil && exists(p) {
		items = append(items, resetItem{"config", tildePath(p), func() error {
			if err := os.Remove(p); err != nil {
				return err
			}
			if os.Getenv("LLMENU_CONFIG") == "" {
				_ = os.Remove(filepath.Dir(p)) // only if empty
			}
			return nil
		}})
	}

	if p, err := state.Path(); err == nil {
		dir := filepath.Dir(p)
		if exists(dir) && (os.Getenv("LLMENU_CACHE_DIR") == "" || slices.ContainsFunc(cacheFiles, func(f string) bool {
			return exists(filepath.Join(dir, f))
		})) {
			items = append(items, resetItem{"cache", tildePath(dir), func() error { return removeCache(dir) }})
		}
	}
	return items, nil
}

// cacheFiles are llmenu's files in the cache directory.
var cacheFiles = []string{"state.json", "models.json", "spend.json", "bench.json"}

// removeCache removes llmenu's cache directory, or with LLMENU_CACHE_DIR,
// which may be shared, only llmenu's files in it.
func removeCache(dir string) error {
	if os.Getenv("LLMENU_CACHE_DIR") == "" {
		return os.RemoveAll(dir)
	}
	for _, f := range cacheFiles {
		if err := os.Remove(filepath.Join(dir, f)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func keyringLabel() string {
	if f := os.Getenv("LLMENU_KEYRING_FILE"); f != "" {
		return "keyring file " + tildePath(f)
	}
	return "keyring"
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
