package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/schowave/llmenu/internal/config"
	"github.com/schowave/llmenu/internal/secrets"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show and change single settings",
		Long: `Settings live in config.toml (llmenu config path). llmenu config list shows
every key with its current value and where it comes from.`,
		Example: `  llmenu config set claude.default sonnet
  llmenu config set bench.metric tb4
  llmenu config unset claude.opus`,
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List all settings with value and source",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "KEY\tVALUE\tSOURCE")
				for _, e := range cfg.List() {
					fmt.Fprintf(w, "%s\t%s\t%s\n", e.Key, orDash(e.Value), orDash(e.Source))
				}
				for _, name := range []string{secrets.Gateway, secrets.ArtificialAnalysis} {
					_, src, err := secrets.Get(name)
					val := "missing"
					switch {
					case err != nil:
						val, src = "error", err.Error()
					case src != "":
						val = "set"
					}
					fmt.Fprintf(w, "secrets.%s\t%s\t%s\n", name, val, orDash(src))
				}
				return w.Flush()
			},
		},
		&cobra.Command{
			Use:   "get <key>",
			Short: "Print the value of a setting",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				e, err := cfg.Get(args[0])
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), e.Value)
				return nil
			},
		},
		&cobra.Command{
			Use:   "set <key> <value>",
			Short: "Change a setting",
			Args:  cobra.ExactArgs(2),
			RunE:  func(cmd *cobra.Command, args []string) error { return setConfig(args[0], args[1]) },
		},
		&cobra.Command{
			Use:   "unset <key>",
			Short: "Remove a setting so the default applies again",
			Args:  cobra.ExactArgs(1),
			RunE:  func(cmd *cobra.Command, args []string) error { return setConfig(args[0], "") },
		},
		&cobra.Command{
			Use:   "path",
			Short: "Print the path of config.toml",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				p, err := config.Path()
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), p)
				return nil
			},
		},
	)
	return cmd
}

func setConfig(key, value string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if key == "gateway.url" && value != "" {
		if err := validateURL(value); err != nil {
			return err
		}
	}
	if err := cfg.Set(key, value); err != nil {
		return err
	}
	_, err = cfg.Save()
	return err
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func newSecretsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Store and remove API keys in the OS keyring",
		Long: `Known keys: gateway (the LiteLLM gateway) and artificialanalysis (benchmarks).
Keys are read hidden from the terminal or from stdin, never from arguments,
so they do not end up in the shell history.`,
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "set <name>",
			Short: "Store a key (asked for hidden, or read from stdin)",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				value, err := readSecret("Key for " + args[0] + ": ")
				if err != nil {
					return err
				}
				return secrets.Set(args[0], value)
			},
		},
		&cobra.Command{
			Use:   "get <name>",
			Short: "Print a key (pi uses: !llmenu secrets get gateway)",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				v, _, err := secrets.Get(args[0])
				if err != nil {
					return err
				}
				if v == "" {
					return fmt.Errorf("key %s not set: llmenu secrets set %s", args[0], args[0])
				}
				fmt.Fprintln(cmd.OutOrStdout(), v)
				return nil
			},
		},
		&cobra.Command{
			Use:   "delete <name>",
			Short: "Delete a key",
			Args:  cobra.ExactArgs(1),
			RunE:  func(cmd *cobra.Command, args []string) error { return secrets.Delete(args[0]) },
		},
	)
	return cmd
}

// readSecret reads hidden input from the terminal, otherwise one line from
// stdin, so that `echo $KEY | llmenu secrets set gateway` works too. Never as
// an argument, so the key does not end up in shell history or the process list.
func readSecret(prompt string) (string, error) {
	if isTerminal(os.Stdin) {
		fmt.Fprint(os.Stderr, prompt)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return strings.TrimSpace(string(b)), err
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("no key on stdin")
	}
	return strings.TrimSpace(line), nil
}
