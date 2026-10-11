package cli

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/schowave/llmenu/internal/bench"
	"github.com/schowave/llmenu/internal/secrets"
)

// aaKeyURL explains how to get the free Artificial Analysis key.
const aaKeyURL = "https://artificialanalysis.ai/api-reference#authentication"

const noAAKeyHelp = `llmenu can show benchmark scores from Artificial Analysis (Intelligence Index,
Terminal-Bench, Coding Index) next to each model and sort by them. Their API
is free:

  1. Create an account on the Artificial Analysis Insights Platform and
     generate an API key: ` + aaKeyURL + `
  2. Store it:  llmenu setup benchmarks   (or: llmenu secrets set artificialanalysis)
  3. Run llmenu bench again.`

func newBenchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bench",
		Short: "Fetch benchmark scores from Artificial Analysis",
		Long: `Fetches Intelligence Index, Terminal-Bench and Coding Index from Artificial
Analysis and matches them to the gateway's models; the menu and llmenu list
then show and sort by them. Variants such as -high or -max count as one model
with its best score.

Needs a free API key (llmenu setup benchmarks). If a model is not matched,
map it by hand: llmenu config set bench.map.<model> <slug>`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			key, _, err := secrets.Get(secrets.ArtificialAnalysis)
			if err != nil {
				return err
			}
			if key == "" {
				fmt.Fprintln(cmd.ErrOrStderr(), noAAKeyHelp)
				fmt.Fprintln(cmd.ErrOrStderr())
				return errors.New("no Artificial Analysis key yet")
			}
			env, err := loadEnv(cmd.Context())
			if err != nil {
				return err
			}
			entries, err := bench.Fetch(cmd.Context(), nil, cmp.Or(os.Getenv("LLMENU_AA_URL"), bench.URL), key)
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
