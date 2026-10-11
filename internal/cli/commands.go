package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/schowave/llmenu/internal/targets"
)

func newTargetsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "targets",
		Short: "Show supported tools, install status and config files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "TARGET\tCAPABILITY\tINSTALLED\tCONFIG")
			for _, t := range targets.All() {
				capability, config := "", "-"
				if s, ok := t.(targets.Syncer); ok {
					capability = "sync"
					if p, err := s.ConfigPath(); err == nil {
						config = p
					}
				}
				if _, ok := t.(targets.Launcher); ok {
					capability = join(capability, "launch")
				}
				installed := "no"
				if p, ok := targets.Installed(t); ok {
					installed = p
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.Name(), capability, installed, config)
			}
			return w.Flush()
		},
	}
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + "+" + b
}
