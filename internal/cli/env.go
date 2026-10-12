package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
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

	if err := trustGateway(env); err != nil {
		return env, err
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
	// Config and LLMENU_BASE_URL are not checked like setup does.
	if err := validateURL(env.BaseURL); env.BaseURL != "" && err != nil && !warnedURL {
		warnedURL = true
		fmt.Fprintf(os.Stderr, "llmenu: warning: gateway URL %s: %v\n", env.BaseURL, err)
	}
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

// originOf is scheme://host[:port] of a URL, the unit a key is bound to; empty
// if there is none.
func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}

// trustGateway makes sure the key is only sent to the gateway it was stored
// for. Whoever can change config.toml or LLMENU_BASE_URL could otherwise point
// llmenu at a server of their choice and read the key from the request. The
// origin is recorded in the keyring when the key is stored (setup, config set
// gateway.url); keys stored by older versions are bound on first use. A
// different origin needs a confirmation on a terminal, otherwise it is an
// error. A key from the environment is not bound: its owner sets both.
func trustGateway(env targets.Env) error {
	if env.APIKey == "" || os.Getenv(secrets.Names[secrets.Gateway]) != "" {
		return nil
	}
	cur := originOf(env.BaseURL)
	stored, err := secrets.Origin()
	if err != nil {
		return err
	}
	switch stored {
	case cur:
		return nil
	case "":
		if err := secrets.SetOrigin(cur); err != nil {
			fmt.Fprintln(os.Stderr, "llmenu: cannot record the gateway for the key:", err)
		}
		return nil
	}
	if !canPrompt() {
		return fmt.Errorf("the gateway key was stored for %s, but the gateway URL now points to %s: run llmenu config set gateway.url %s to send it there", stored, cur, env.BaseURL)
	}
	if ok, err := confirmOrigin(stored, cur); err != nil || !ok {
		return errors.New("gateway key not sent")
	}
	return secrets.SetOrigin(cur)
}

// warnedURL keeps the warning to one per run.
var warnedURL bool

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

	confirmOrigin = func(stored, cur string) (bool, error) {
		var ok bool
		err := runPrompt(huh.NewConfirm().
			Title("The gateway key was stored for " + stored + ". Send it to " + cur + " instead?").
			Affirmative("Yes").Negative("No").
			Value(&ok))
		return ok, err
	}

	confirmSetup = func() (bool, error) {
		start := true
		err := runPrompt(huh.NewConfirm().
			Title("llmenu is not set up yet. Set it up now?").
			Affirmative("Yes").Negative("No").
			Value(&start))
		return start, err
	}
)
