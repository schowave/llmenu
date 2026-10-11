package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"

	"github.com/schowave/llmenu/internal/bench"
	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/config"
	"github.com/schowave/llmenu/internal/gateway"
	"github.com/schowave/llmenu/internal/secrets"
	"github.com/schowave/llmenu/internal/targets"
)

var errNotConfigured = errors.New("gateway not set up yet: run llmenu setup")

// loadEnv reads config, gateway URL and key, and fetches the models.
// If a required setting is missing, it offers setup when on a terminal
// (ADR-0003); without a terminal it aborts.
func loadEnv(ctx context.Context) (targets.Env, error) {
	cfg, err := config.Load()
	if err != nil {
		return targets.Env{}, err
	}
	env, err := gatewayFrom(cfg)
	if err != nil {
		return env, err
	}
	if env.BaseURL == "" || env.APIKey == "" {
		if !isTerminal(os.Stdin) {
			return env, errNotConfigured
		}
		start := true
		err := huh.NewConfirm().
			Title("Gateway not set up yet. Set it up now?").
			Affirmative("Yes").Negative("No").
			Value(&start).Run()
		if err != nil || !start {
			return env, errNotConfigured
		}
		if err := setupGateway(ctx, cfg); err != nil {
			return env, err
		}
		if env, err = gatewayFrom(cfg); err != nil {
			return env, err
		}
	}

	env.Models, err = gateway.Client{BaseURL: env.BaseURL, APIKey: env.APIKey}.Models(ctx)
	if err != nil {
		return env, err
	}
	st, err := bench.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "llmenu: cannot read benchmarks:", err)
	}
	st.Apply(env.Models)
	return env, nil
}

// sortModels sorts by the configured benchmark, or by price without benchmarks.
func sortModels(models []catalog.Model, cfg *config.Config) {
	if slices.ContainsFunc(models, func(m catalog.Model) bool { return m.Bench != nil }) {
		catalog.SortByBench(models, cfg.Bench.MetricOrDefault())
		return
	}
	catalog.SortByPrice(models)
}

// benchNote describes the source and age of the benchmarks.
func benchNote() string {
	st, _ := bench.Load()
	return st.Note(time.Now())
}

// gatewayFrom reads URL and key; environment variables take precedence.
func gatewayFrom(cfg *config.Config) (targets.Env, error) {
	env := targets.Env{BaseURL: os.Getenv("LLMENU_BASE_URL"), Config: cfg}
	if env.BaseURL == "" {
		env.BaseURL = cfg.Gateway.URL
	}
	key, _, err := secrets.Get(secrets.Gateway)
	if err != nil {
		return env, fmt.Errorf("gateway key: %w", err)
	}
	env.APIKey = key
	return env, nil
}

func isTerminal(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }
