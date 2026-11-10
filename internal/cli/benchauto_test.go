package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/schowave/llmenu/internal/bench"
	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/config"
	"github.com/schowave/llmenu/internal/state"
	"github.com/schowave/llmenu/internal/targets"
)

// fakeAA counts requests and answers like Artificial Analysis (or fails).
func fakeAA(t *testing.T, fail bool) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if fail {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"slug":"claude-opus-5-5","evaluations":{"artificial_analysis_intelligence_index":57.6}}]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LLMENU_AA_URL", srv.URL)
	return &calls
}

func autoEnv() targets.Env {
	models := []catalog.Model{{Name: "aws/claude-opus-5-5", Target: "bedrock/eu.anthropic.claude-opus-5-5"}}
	return targets.Env{Config: &config.Config{}, Models: models}
}

func setNow(t *testing.T, at time.Time) {
	orig := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = orig })
}

func TestAutoRefreshBench(t *testing.T) {
	t0 := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		key       string
		fail      bool
		fetchedAt time.Time // zero: no benchmarks yet
		attempt   time.Time
		wantCalls int32
		wantData  bool
	}{
		{name: "no benchmarks yet, key set", key: "aa", wantCalls: 1, wantData: true},
		{name: "no key", wantCalls: 0},
		{name: "fresh benchmarks", key: "aa", fetchedAt: t0.Add(-24 * time.Hour), wantCalls: 0, wantData: true},
		{name: "stale benchmarks", key: "aa", fetchedAt: t0.Add(-20 * 24 * time.Hour), wantCalls: 1, wantData: true},
		{name: "stale, but tried an hour ago", key: "aa", fetchedAt: t0.Add(-20 * 24 * time.Hour), attempt: t0.Add(-time.Hour), wantCalls: 0, wantData: true},
		{name: "stale, last try two days ago", key: "aa", fetchedAt: t0.Add(-20 * 24 * time.Hour), attempt: t0.Add(-48 * time.Hour), wantCalls: 1, wantData: true},
		{name: "exactly stale", key: "aa", fetchedAt: t0.Add(-bench.StaleAfter), wantCalls: 1, wantData: true},
		{name: "stale, last try exactly a day ago", key: "aa", fetchedAt: t0.Add(-20 * 24 * time.Hour), attempt: t0.Add(-autoBenchInterval), wantCalls: 1, wantData: true},
		{name: "server down", key: "aa", fail: true, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LLMENU_CACHE_DIR", t.TempDir())
			t.Setenv("LLMENU_AA_API_KEY", tc.key)
			t.Setenv("LLMENU_KEYRING_FILE", filepath.Join(t.TempDir(), "keys.json"))
			calls := fakeAA(t, tc.fail)
			setNow(t, t0)
			if !tc.fetchedAt.IsZero() {
				if err := (&bench.Store{FetchedAt: tc.fetchedAt, Models: map[string]catalog.Bench{"aws/claude-opus-5-5": {Slug: "old"}}}).Save(); err != nil {
					t.Fatal(err)
				}
			}
			if !tc.attempt.IsZero() {
				if err := (state.State{LastModel: map[string]string{}, BenchAttempt: tc.attempt}).Save(); err != nil {
					t.Fatal(err)
				}
			}

			env := autoEnv()
			autoRefreshBench(context.Background(), env)

			if got := calls.Load(); got != tc.wantCalls {
				t.Errorf("requests = %d, want %d", got, tc.wantCalls)
			}
			st, _ := bench.Load()
			if (st != nil) != tc.wantData {
				t.Errorf("benchmarks present = %v, want %v", st != nil, tc.wantData)
			}
			if tc.wantCalls == 1 && !tc.fail {
				if env.Models[0].Bench == nil || env.Models[0].Bench.Slug != "claude-opus-5-5" {
					t.Errorf("fresh values not applied: %+v", env.Models[0].Bench)
				}
				if !st.FetchedAt.After(t0.Add(-time.Hour)) {
					t.Errorf("fetched_at %v not refreshed", st.FetchedAt)
				}
			}
			// Every real attempt is remembered, so a failure is not retried at once.
			if a := state.Load().BenchAttempt; tc.wantCalls == 1 && !a.Equal(t0) {
				t.Errorf("attempt %v not recorded", a)
			}
		})
	}

	// A failed attempt is not repeated on the next start.
	t.Run("no retry after a failure", func(t *testing.T) {
		t.Setenv("LLMENU_CACHE_DIR", t.TempDir())
		t.Setenv("LLMENU_AA_API_KEY", "aa")
		calls := fakeAA(t, true)
		setNow(t, t0)
		autoRefreshBench(context.Background(), autoEnv())
		setNow(t, t0.Add(time.Hour))
		autoRefreshBench(context.Background(), autoEnv())
		if got := calls.Load(); got != 1 {
			t.Errorf("requests = %d, want 1", got)
		}
	})
}
