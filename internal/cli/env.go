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
// If a required setting is missing, it offers setup when on a terminal or in
// accessible mode (ADR-0003); otherwise it aborts, so scripts never hang.
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
		if !canPrompt() {
			return env, errNotConfigured
		}
		if start, err := confirmSetup(); err != nil || !start {
			return env, errNotConfigured
		}
		// First run (no config file yet): the whole setup, so tools, default
		// models and the benchmark key are asked too. Otherwise only the
		// missing gateway part.
		if firstRun() {
			if err := runSetup(ctx, nil); err != nil {
				return env, err
			}
			if cfg, err = config.Load(); err != nil {
				return env, err
			}
		} else if err := setupGateway(ctx, cfg); err != nil {
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
	cacheModelNames(env.Models)
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

// menuNote is the benchmark source for the preview; without benchmarks the
// menu's footer suggests llmenu bench instead.
func menuNote() string {
	if st, _ := bench.Load(); st != nil {
		return st.Note(time.Now())
	}
	return ""
}

// benchNote describes the source and age of the benchmarks.
func benchNote() string {
	st, _ := bench.Load()
	return st.Note(time.Now())
}

// gatewayFrom reads URL and key; environment variables take precedence. With
// an active project its key is used; a missing one is an error, never a
// silent fallback to the main key, which would bill the wrong project.
func gatewayFrom(cfg *config.Config) (targets.Env, error) {
	env := targets.Env{BaseURL: os.Getenv("LLMENU_BASE_URL"), Config: cfg}
	if env.BaseURL == "" {
		env.BaseURL = cfg.Gateway.URL
	}
	project, err := activeProject(cfg)
	if err != nil {
		return env, err
	}
	env.Project = project
	key, _, err := secrets.Get(secrets.ProjectKey(project))
	if err != nil {
		return env, fmt.Errorf("gateway key: %w", err)
	}
	if key == "" && project != "" {
		return env, fmt.Errorf("no key for project %s: llmenu project add %s", project, project)
	}
	env.APIKey = key
	return env, nil
}

// firstRun reports that there is no config file yet.
func firstRun() bool {
	p, err := config.Path()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return os.IsNotExist(err)
}

// Replaceable in tests.
var (
	isTerminal = func(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

	confirmSetup = func() (bool, error) {
		start := true
		err := runPrompt(huh.NewConfirm().
			Title("llmenu is not set up yet. Set it up now?").
			Affirmative("Yes").Negative("No").
			Value(&start))
		return start, err
	}
)
