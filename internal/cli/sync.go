package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/schowave/llmenu/internal/secrets"
	"github.com/schowave/llmenu/internal/targets"
)

func newSyncCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:               "sync [target…]",
		ValidArgsFunction: completeArgs(func() []string { return targetNames(isSyncer) }),
		Short:             "Write the gateway's models into pi and opencode",
		Long: `Writes all chat models of the gateway, with limits and prices, into the
configuration of the given tools, or of your tools (llmenu setup tools).
Only llmenu's own provider entry is changed; comments and other providers
stay, and the previous file is kept as *.bak.`,
		Example: `  llmenu sync --dry-run
  llmenu sync
  llmenu sync pi`,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := loadEnv(cmd.Context())
			if err != nil {
				return err
			}
			names := args
			if len(names) == 0 {
				for _, t := range enabledTools(env.Config) {
					if tg, ok := targets.Get(t); ok {
						if _, isSyncer := tg.(targets.Syncer); isSyncer {
							names = append(names, t)
						}
					}
				}
			}
			if len(names) == 0 {
				return errors.New("none of your tools keeps a model list: llmenu setup tools, or name one, e.g. llmenu sync pi")
			}
			warnKeySource()

			opt := targets.SyncOptions{
				BaseURL:  env.BaseURL,
				Provider: env.ProviderName(),
				DryRun:   dryRun,
			}
			out := cmd.OutOrStdout()
			var failed []string
			for _, name := range names {
				t, ok := targets.Get(name)
				s, isSyncer := t.(targets.Syncer)
				if !ok || !isSyncer {
					fmt.Fprintf(os.Stderr, "%s: not a sync target, see llmenu targets\n", name)
					failed = append(failed, name)
					continue
				}
				d, err := s.Sync(cmd.Context(), env.Models, opt)
				switch {
				case err != nil:
					fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
					failed = append(failed, name)
				case !d.Changed:
					fmt.Fprintf(out, "%s: unchanged (%s)\n", name, d.Path)
				case dryRun:
					fmt.Fprintf(out, "%s: would change\n%s", name, d.Patch)
				default:
					fmt.Fprintf(out, "%s: %d models written under %q (%s)\n", name, len(env.Models), opt.Provider, d.Path)
				}
			}
			if len(failed) > 0 {
				return fmt.Errorf("failed: %v", failed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "only show the diff, write nothing")
	return cmd
}

// warnKeySource warns if tools would not find the key later: they call
// `llmenu secrets get gateway`, which needs llmenu on the PATH and the key in
// the keyring.
func warnKeySource() {
	if _, err := exec.LookPath("llmenu"); err != nil {
		fmt.Fprintln(os.Stderr, "Warning: llmenu is not on the PATH; pi will not find the key.")
	}
	if _, src, _ := secrets.Get(secrets.Gateway); src == "environment" {
		fmt.Fprintln(os.Stderr, "Warning: the gateway key comes from LLMENU_API_KEY. pi and opencode need it permanently: llmenu secrets set gateway")
	}
}
