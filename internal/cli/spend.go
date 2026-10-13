package cli

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/schowave/llmenu/internal/config"
	"github.com/schowave/llmenu/internal/gateway"
	"github.com/schowave/llmenu/internal/secrets"
	"github.com/schowave/llmenu/internal/state"
	"github.com/schowave/llmenu/internal/targets"
)

// How long a cached budget counts as current, and how long a stale one is
// still shown when the gateway does not answer.
const (
	spendFresh = time.Minute
	spendStale = time.Hour
	// menuSpendWait bounds how long the menu waits for the budget; it is
	// asked while the benchmarks refresh, so it rarely waits at all.
	menuSpendWait = 2 * time.Second
	// listSpendWait bounds llmenu project list, which asks for all keys at once.
	listSpendWait = 5 * time.Second
)

// projectSpends asks for the budget of each project's key ("" is the main
// key) in parallel; an entry stays empty if it is unknown. Keys go only to
// the gateway they were stored for, and nothing is asked without a binding.
func projectSpends(ctx context.Context, cfg *config.Config, projects []string) []string {
	out := make([]string, len(projects))
	base := cmp.Or(os.Getenv("LLMENU_BASE_URL"), cfg.Gateway.URL)
	if stored, err := secrets.Origin(); base == "" || err != nil || stored == "" || stored != originOf(base) {
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, listSpendWait)
	defer cancel()
	var wg sync.WaitGroup
	for i, p := range projects {
		key, _, err := secrets.Get(secrets.ProjectKey(p))
		if err != nil || key == "" {
			continue
		}
		wg.Go(func() {
			if b, ok := budget(ctx, base, key); ok {
				out[i] = formatBudget(b, now())
			}
		})
	}
	wg.Wait()
	return out
}

// startBudget asks for the key's budget in the background; the returned
// function waits for the formatted result, "" if none came in time.
func startBudget(ctx context.Context, env targets.Env) func() string {
	ch := make(chan string, 1)
	go func() {
		ctx, cancel := context.WithTimeout(ctx, menuSpendWait)
		defer cancel()
		if b, ok := budget(ctx, env.BaseURL, env.APIKey); ok {
			ch <- formatBudget(b, now())
		}
		close(ch)
	}()
	return func() string { return <-ch }
}

// spend calls f if there is one.
func spend(f func() string) string {
	if f == nil {
		return ""
	}
	return f()
}

// budget returns the key's budget from the cache or, if that is older than
// spendFresh, from the gateway. On failure a cached value up to spendStale
// old is used; otherwise ok is false. The caller bounds the wait via ctx.
func budget(ctx context.Context, baseURL, key string) (b gateway.Budget, ok bool) {
	id := state.SpendID(baseURL, key)
	cached, have := state.LoadSpend(id)
	t := now()
	if have && t.Sub(cached.Fetched) < spendFresh {
		return cached.Budget, true
	}
	b, err := gateway.Client{BaseURL: baseURL, APIKey: key}.Spend(ctx)
	if err != nil {
		return cached.Budget, have && t.Sub(cached.Fetched) < spendStale
	}
	_ = state.SaveSpend(id, b, t) // only a cache
	return b, true
}

// formatBudget is "$12.34 / $50.00 · resets in 18d" for the key's own
// budget, "user budget $12.34 / $50.00 · this key $5.67 · …" for its user's, or "$12.34
// spent" without any.
func formatBudget(b gateway.Budget, t time.Time) string {
	if b.Max == 0 {
		return dollars(b.Spend) + " spent"
	}
	s := dollars(b.Spend) + " / " + dollars(b.Max)
	// A team's or user's budget covers more than this key.
	if b.Scope == "team" || b.Scope == "user" {
		s = b.Scope + " budget " + s + " · this key " + dollars(b.KeySpend)
	}
	if !b.ResetAt.IsZero() && b.ResetAt.After(t) {
		s += " · resets in " + until(b.ResetAt.Sub(t))
	}
	return s
}

func dollars(v float64) string {
	if v >= 1000 {
		return "$" + fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

// until is a short duration: 18d, 5h or 42m.
func until(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%dd", int(math.Round(d.Hours()/24)))
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dm", max(1, int(d.Minutes())))
}
