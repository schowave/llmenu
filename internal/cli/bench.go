package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/schowave/llmenu/internal/bench"
	"github.com/schowave/llmenu/internal/secrets"
	"github.com/schowave/llmenu/internal/state"
	"github.com/schowave/llmenu/internal/targets"
)

// aaKeyURL explains how to get the free Artificial Analysis key.
const aaKeyURL = "https://artificialanalysis.ai/data-api/docs"

const noAAKeyHelp = `llmenu can show benchmark scores from Artificial Analysis (Intelligence Index,
output speed; Terminal-Bench and Long Context with a Pro key) next to each
model and sort by them. Their API is free:

  1. Create an account on the Artificial Analysis Insights Platform and
     generate an API key: ` + aaKeyURL + `
  2. Store it:  llmenu setup benchmarks   (or: llmenu secrets set artificialanalysis)
  3. Run llmenu bench again.`

func newBenchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bench",
		Short: "Fetch benchmark scores from Artificial Analysis",
		Long: `Fetches Intelligence Index and output speed from Artificial Analysis, with a
Pro key also Terminal-Bench and Long Context Reasoning, and matches them to the
gateway's models; the menu and llmenu list then show and sort by them.
Variants such as -high or -max count as one model with its best score.

Needs an API key, a free one is enough (llmenu setup benchmarks). If a model is not matched,
map it by hand: llmenu config set bench.map.<model> <slug>`,
		Example: `  llmenu bench
  llmenu config set bench.map.mistral/mistral-large-2512 mistral-large-3`,
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
			// Always try the full list first: the key may have become Pro.
			st, err := fetchBench(cmd.Context(), env, key, false)
			if err != nil {
				return err
			}

			var missing []string
			counts := map[string]int{}
			for _, m := range env.Models {
				b, ok := st.Models[m.Name]
				if !ok {
					missing = append(missing, m.Name)
					continue
				}
				for label, v := range map[string]*float64{
					"Intelligence Index": b.Intelligence, "Terminal-Bench v4": b.TerminalBench,
					"Coding Index": b.Coding, "Long Context": b.LongContext, "output speed": b.OutputSpeed,
				} {
					if v != nil {
						counts[label]++
					}
				}
			}
			// The Intelligence Index always, the others only where the API
			// delivered values: the Coding Index is no longer published,
			// Terminal-Bench and Long Context need a Pro key.
			found := []string{fmt.Sprintf("Intelligence Index: %d", counts["Intelligence Index"])}
			for _, label := range []string{"Terminal-Bench v4", "Long Context", "Coding Index", "output speed"} {
				if counts[label] > 0 {
					found = append(found, fmt.Sprintf("%s: %d", label, counts[label]))
				}
			}
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "Data:", bench.Source)
			fmt.Fprintf(out, "%d of %d models matched. %s.\n", len(st.Models), len(env.Models), strings.Join(found, ", "))
			if st.Tier == bench.TierFree {
				fmt.Fprintln(out, "Free API key: Terminal-Bench and Long Context need an Artificial Analysis Pro key.")
			}
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

// fetchBench fetches the benchmarks, matches them to the gateway's models,
// saves them and attaches them to env.Models.
// freeFirst skips the full list, which a free key cannot read.
func fetchBench(ctx context.Context, env targets.Env, key string, freeFirst bool) (*bench.Store, error) {
	res, err := bench.Fetch(ctx, nil, cmp.Or(os.Getenv("LLMENU_AA_URL"), bench.URL), key, freeFirst)
	if err != nil {
		return nil, err
	}
	if res.Truncated {
		fmt.Fprintln(os.Stderr, "Warning: Artificial Analysis has more pages than llmenu reads; some models may be missing.")
	}
	st := &bench.Store{
		FetchedAt: now().UTC(),
		Tier:      res.Tier,
		Models:    bench.Match(res.Entries, env.Models, env.Config.Bench.Map),
	}
	if err := st.Save(); err != nil {
		return nil, err
	}
	st.Apply(env.Models)
	return st, nil
}

// Automatic refresh: with a key, benchmarks that are missing or older than
// bench.StaleAfter are fetched before the menu or list opens. At most one
// attempt per autoBenchInterval, with a short timeout; failures are silent
// and the old values (if any) stay.
const (
	autoBenchInterval = 24 * time.Hour
	autoBenchTimeout  = 3 * time.Second
)

var now = time.Now // replaceable in tests

func autoRefreshBench(ctx context.Context, env targets.Env) {
	st, err := bench.Load()
	if err != nil || (st != nil && now().Sub(st.FetchedAt) < bench.StaleAfter) {
		return
	}
	s := state.Load()
	if now().Sub(s.BenchAttempt) < autoBenchInterval {
		return
	}
	// Only now read the key: most starts never touch the keyring for this.
	key, _, err := secrets.Get(secrets.ArtificialAnalysis)
	if err != nil || key == "" {
		return
	}
	s.BenchAttempt = now()
	_ = s.Save()
	ctx, cancel := context.WithTimeout(ctx, autoBenchTimeout)
	defer cancel()
	// A free key gets HTTP 403 for the full list; skipping it keeps the
	// refresh within autoBenchTimeout.
	_, _ = fetchBench(ctx, env, key, st != nil && st.Tier == bench.TierFree)
}
