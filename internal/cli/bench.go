package cli

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/schowave/llmenu/internal/bench"
	"github.com/schowave/llmenu/internal/secrets"
)

func newBenchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bench",
		Short: "Fetch benchmarks from Artificial Analysis and match them to the models",
		Long: `Fetches Intelligence Index, Coding Index and Terminal-Bench v4 and matches them
to the gateway's models. Variants (-high, -max, …) count as one model
with the best score. Fixed mappings: llmenu config set bench.map.<model> <slug>.
Key: llmenu setup benchmarks or LLMENU_AA_API_KEY.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			key, _, err := secrets.Get(secrets.ArtificialAnalysis)
			if err != nil {
				return err
			}
			if key == "" {
				return errors.New("no Artificial Analysis key: llmenu setup benchmarks")
			}
			env, err := loadEnv(cmd.Context())
			if err != nil {
				return err
			}
			entries, err := bench.Fetch(cmd.Context(), nil, bench.URL, key)
			if err != nil {
				return err
			}

			st := &bench.Store{
				FetchedAt: time.Now().UTC(),
				Models:    bench.Match(entries, env.Models, env.Config.Bench.Map),
			}
			if err := st.Save(); err != nil {
				return err
			}

			var intel, tb4, code int
			var missing []string
			for _, m := range env.Models {
				b, ok := st.Models[m.Name]
				if !ok {
					missing = append(missing, m.Name)
					continue
				}
				if b.Intelligence != nil {
					intel++
				}
				if b.TerminalBench != nil {
					tb4++
				}
				if b.Coding != nil {
					code++
				}
			}
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "Data:", bench.Source)
			fmt.Fprintf(out, "%d of %d models matched. Intelligence Index: %d, Terminal-Bench v4: %d, Coding Index: %d.\n",
				len(st.Models), len(env.Models), intel, tb4, code)
			if len(missing) > 0 {
				slices.Sort(missing)
				fmt.Fprintln(out, "No match (fixed mapping: llmenu config set bench.map.<model> <slug>):")
				for _, m := range missing {
					fmt.Fprintln(out, "  "+m)
				}
			}
			for name, slug := range env.Config.Bench.Map {
				if _, ok := st.Models[name]; !ok {
					fmt.Fprintf(os.Stderr, "Warning: bench.map.%s = %s yields no match.\n", name, slug)
				}
			}
			return nil
		},
	}
}
