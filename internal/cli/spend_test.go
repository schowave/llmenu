package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/schowave/llmenu/internal/gateway"
)

func TestFormatBudget(t *testing.T) {
	t0 := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		b    gateway.Budget
		want string
	}{
		{gateway.Budget{Spend: 0.214}, "$0.21 spent"},
		{gateway.Budget{Spend: 12.5, Max: 50}, "$12.50 / $50.00"},
		{gateway.Budget{Spend: 1234.5, Max: 5000}, "$1234 / $5000"},
		{gateway.Budget{Spend: 1, Max: 2, ResetAt: t0.Add(18*24*time.Hour + time.Hour)}, "$1.00 / $2.00 · resets in 18d"},
		{gateway.Budget{Spend: 1, Max: 2, ResetAt: t0.Add(30 * time.Hour)}, "$1.00 / $2.00 · resets in 30h"},
		{gateway.Budget{Spend: 1, Max: 2, ResetAt: t0.Add(20 * time.Second)}, "$1.00 / $2.00 · resets in 1m"},
		{gateway.Budget{Spend: 1, Max: 2, ResetAt: t0.Add(-time.Hour)}, "$1.00 / $2.00"},
		{gateway.Budget{Scope: "key", Spend: 1, Max: 2}, "$1.00 / $2.00"},
		{gateway.Budget{Scope: "user", Spend: 18, KeySpend: 12.45, Max: 50, ResetAt: t0.Add(19 * 24 * time.Hour)}, "user budget $18.00 / $50.00 · this key $12.45 · resets in 19d"},
		{gateway.Budget{Scope: "team", Spend: 1, Max: 2}, "team budget $1.00 / $2.00 · this key $0.00"},
	}
	for _, tt := range tests {
		if got := formatBudget(tt.b, t0); got != tt.want {
			t.Errorf("formatBudget(%+v) = %q, want %q", tt.b, got, tt.want)
		}
	}
}

// TestBudgetCache: a fresh value comes from the cache, an old one from the
// gateway; if the gateway fails, a value up to an hour old stands in.
func TestBudgetCache(t *testing.T) {
	t.Setenv("LLMENU_CACHE_DIR", t.TempDir())
	var calls, failing atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if failing.Load() == 1 {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"info":{"spend":2}}`))
	}))
	defer srv.Close()
	t0 := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	clock := t0
	defer func(old func() time.Time) { now = old }(now)
	now = func() time.Time { return clock }
	ctx := context.Background()

	check := func(step string, wantOK bool, wantCalls int32) {
		t.Helper()
		b, ok := budget(ctx, srv.URL, "sk")
		if ok != wantOK || (ok && b.Spend != 2) || calls.Load() != wantCalls {
			t.Errorf("%s: budget = %+v, %v after %d calls; want ok %v after %d", step, b, ok, calls.Load(), wantOK, wantCalls)
		}
	}
	check("first", true, 1)
	clock = t0.Add(30 * time.Second)
	check("cached", true, 1)
	failing.Store(1)
	clock = t0.Add(30 * time.Minute)
	check("stale while down", true, 2)
	clock = t0.Add(2 * time.Hour)
	check("too old while down", false, 3)
	if _, ok := budget(ctx, srv.URL, "other"); ok {
		t.Error("another key got this key's budget")
	}
}
