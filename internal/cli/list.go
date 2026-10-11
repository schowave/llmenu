package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/table"
)

func newListCmd() *cobra.Command {
	var asJSON, all bool
	var sortBy string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show the gateway's models as a table",
		Long: `Shows the gateway's chat models with context size, prices per million tokens
and, after llmenu bench, benchmark scores; sorted by the benchmark in
bench.metric, or by price. Aliases and the same model from several providers
are folded together; --all shows them.`,
		Example: `  llmenu list
  llmenu list --sort price --all
  llmenu list --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if sortBy != "bench" && sortBy != "price" {
				return fmt.Errorf("--sort bench or price, not %q", sortBy)
			}
			models, err := fetchModels(cmd.Context(), all, sortBy)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(models)
			}
			printTable(cmd.OutOrStdout(), models, all)
			fmt.Fprintln(cmd.OutOrStdout(), "\n"+benchNote())
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print as JSON")
	cmd.Flags().BoolVar(&all, "all", false, "also show aliases and duplicates")
	cmd.Flags().StringVar(&sortBy, "sort", "bench", "bench (by bench.metric) or price")
	return cmd
}

// fetchModels fetches the chat models from the gateway, without all stripped
// of aliases and duplicates, and sorts them.
func fetchModels(ctx context.Context, all bool, sortBy string) ([]catalog.Model, error) {
	env, err := loadEnv(ctx)
	if err != nil {
		return nil, err
	}
	models := env.Models
	if !all {
		models = catalog.Dedupe(models)
	}
	if sortBy == "price" {
		catalog.SortByPrice(models)
	} else {
		sortModels(models, env.Config)
	}
	return models, nil
}

func printTable(w io.Writer, models []catalog.Model, all bool) {
	header, rows := table.Format(models, table.Options{ShowAliasTarget: all})
	fmt.Fprintln(w, header)
	for _, r := range rows {
		fmt.Fprintln(w, r)
	}
}
