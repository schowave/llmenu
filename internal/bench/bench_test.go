package bench

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/schowave/llmenu/internal/catalog"
)

func f(v float64) *float64 { return &v }

func entry(slug string, intel, tb *float64) Entry {
	e := Entry{Slug: slug}
	e.Evaluations.Intelligence, e.Evaluations.TerminalBench = intel, tb
	return e
}

func withLCR(e Entry, lcr float64) Entry {
	e.Evaluations.LongContext = &lcr
	return e
}

func withSpeed(e Entry, tps float64) Entry {
	e.Performance.OutputSpeed = &tps
	return e
}

var aa = []Entry{
	withSpeed(entry("claude-opus-5-5", f(57.6), f(0.5959)), 96.575),
	withLCR(withSpeed(entry("claude-opus-5-5-xhigh", f(56), f(0.6)), 0), 0.846666), // 0: not measured
	entry("claude-opus-5-5-low", f(42.3), f(0.31)),
	entry("qwen3-8-flash-next", f(40), nil),
	entry("qwen3-8-flash-next-non-reasoning", f(30), nil),
	entry("mistral-large-3", f(25), nil),
	entry("no-values", nil, nil),
	// A single benchmark is enough to count.
	entry("only-terminal", nil, f(0.5)),
	withSpeed(entry("only-speed", nil, nil), 42.4),
	withLCR(entry("only-lcr", nil, nil), 0.5),
	withSpeed(entry("speed-zero", nil, nil), 0), // 0: not measured
}

func TestMatch(t *testing.T) {
	models := []catalog.Model{
		{Name: "aws/claude-opus-5-5", Target: "bedrock/eu.anthropic.claude-opus-5-5"},
		{Name: "aws/claude-opus-latest", Target: "bedrock/eu.anthropic.claude-opus-5-5"},
		{Name: "local/Qwen3.8-Flash-Next", Target: "hosted_vllm/Qwen/Qwen3.8-Flash-Next-FP8"},
		{Name: "local/Qwen3.8-Flash-Next-non-thinking", Target: "hosted_vllm/Qwen/Qwen3.8-Flash-Next-FP8", NoThinking: true},
		{Name: "mistral/mistral-large-2512", Target: "mistral/mistral-large-2512"},
		{Name: "unknown/x-1", Target: "unknown/x-1"},
		{Name: "x/no-values", Target: "no-values"},
		{Name: "x/only-terminal", Target: "only-terminal"},
		{Name: "x/only-speed", Target: "only-speed"},
		{Name: "x/only-lcr", Target: "only-lcr"},
		{Name: "x/speed-zero", Target: "speed-zero"},
	}
	got := Match(aa, models, map[string]string{"mistral/mistral-large-2512": "mistral-large-3"})

	opus, ok := got["aws/claude-opus-5-5"]
	if !ok {
		t.Fatal("Opus has no match")
	}
	// Variants merged: best value per benchmark, shortest slug.
	if opus.Slug != "claude-opus-5-5" || *opus.Intelligence != 57.6 || *opus.TerminalBench != 60 ||
		opus.OutputSpeed == nil || *opus.OutputSpeed != 97 || opus.LongContext == nil || *opus.LongContext != 84.7 {
		t.Errorf("Opus = %+v", show(opus))
	}
	if _, ok := got["aws/claude-opus-latest"]; !ok {
		t.Error("alias gets no values")
	}
	if q := got["local/Qwen3.8-Flash-Next"]; q.Intelligence == nil || *q.Intelligence != 40 {
		t.Errorf("Qwen = %v", show(q))
	}
	if q := got["local/Qwen3.8-Flash-Next-non-thinking"]; q.Intelligence == nil || *q.Intelligence != 30 {
		t.Errorf("Qwen without thinking does not use the non-reasoning measurement: %v", show(q))
	}
	if m := got["mistral/mistral-large-2512"]; m.Slug != "mistral-large-3" {
		t.Errorf("fixed mapping ignored: %v", show(m))
	}
	if _, ok := got["unknown/x-1"]; ok {
		t.Error("match without a matching model")
	}
	if _, ok := got["x/no-values"]; ok {
		t.Error("entry without any benchmark matched")
	}
	if tb := got["x/only-terminal"]; tb.TerminalBench == nil || *tb.TerminalBench != 50 {
		t.Errorf("entry with only Terminal-Bench ignored: %v", show(tb))
	}
	if s := got["x/only-speed"]; s.OutputSpeed == nil || *s.OutputSpeed != 42 {
		t.Errorf("entry with only a speed ignored: %+v", s)
	}
	if l := got["x/only-lcr"]; l.LongContext == nil || *l.LongContext != 50 {
		t.Errorf("entry with only Long Context ignored: %+v", l)
	}
	if _, ok := got["x/speed-zero"]; ok {
		t.Error("entry with an unmeasured speed matched")
	}
}

func show(b catalog.Bench) string {
	s := b.Slug
	for _, v := range []*float64{b.Intelligence, b.TerminalBench} {
		if v == nil {
			s += " -"
		} else {
			s += fmt.Sprintf(" %g", *v)
		}
	}
	return s
}

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "k" {
			http.Error(w, `{"error":"invalid key"}`, http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"slug":"a","evaluations":{"artificial_analysis_intelligence_index":50}}]}`))
	}))
	defer srv.Close()

	res, err := Fetch(context.Background(), nil, srv.URL, "k")
	if err != nil || len(res.Entries) != 1 || *res.Entries[0].Evaluations.Intelligence != 50 || res.Tier != "pro" || res.Truncated {
		t.Fatalf("Fetch = %+v, %v", res, err)
	}
	if _, err := Fetch(context.Background(), nil, srv.URL, "wrong"); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Errorf("wrong key: %v", err)
	}
}

// aaServer serves the full list only to key "pro" and the free list in
// pages of one model; requests are counted per URI.
func aaServer(t *testing.T, pages int, totalPages bool) (*httptest.Server, func() map[string]int) {
	t.Helper()
	var mu sync.Mutex
	seen := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.RequestURI()]++
		mu.Unlock()
		switch r.URL.Path {
		case "/m":
			if r.Header.Get("x-api-key") != "pro" {
				http.Error(w, `{"error":"Language models list requires a Pro subscription"}`, http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(`{"tier":"pro","data":[{"slug":"p","evaluations":{"terminalbench_v4_0":0.5}}]}`))
		case "/m/free":
			n := 1
			if v := r.URL.Query().Get("page"); v != "" {
				n, _ = strconv.Atoi(v)
			}
			total := ""
			if totalPages {
				total = fmt.Sprintf(`"total_pages":%d,`, pages)
			}
			fmt.Fprintf(w, `{"tier":"free","pagination":{"page":%d,%s"has_more":%t},
				"data":[{"slug":"m%d","performance":{"median_output_tokens_per_second":80.07}}]}`, n, total, n < pages, n)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() map[string]int {
		mu.Lock()
		defer mu.Unlock()
		return maps.Clone(seen)
	}
}

func slugs(es []Entry) string {
	var s []string
	for _, e := range es {
		s = append(s, e.Slug)
	}
	return strings.Join(s, ",")
}

// TestFetchFree: a free key gets 403 for the full list and falls back to the
// free list; its pages come in order whether fetched in parallel (the API
// names total_pages) or one after another.
func TestFetchFree(t *testing.T) {
	for _, total := range []bool{true, false} {
		t.Run(fmt.Sprintf("total_pages=%t", total), func(t *testing.T) {
			srv, seen := aaServer(t, 7, total)
			res, err := Fetch(context.Background(), nil, srv.URL+"/m", "k")
			if err != nil || res.Tier != TierFree || res.Truncated || slugs(res.Entries) != "m1,m2,m3,m4,m5,m6,m7" {
				t.Fatalf("Fetch = %s %+v, %v", slugs(res.Entries), res, err)
			}
			if v := res.Entries[0].Performance.OutputSpeed; v == nil || *v != 80.07 {
				t.Errorf("output speed = %v", v)
			}
			if s := seen(); s["/m"] != 1 || s["/m/free"] != 1 || s["/m/free?page=7"] != 1 || len(s) != 8 {
				t.Errorf("requests = %v", s)
			}
		})
	}
}

func TestFetchPro(t *testing.T) {
	srv, _ := aaServer(t, 1, true)
	res, err := Fetch(context.Background(), nil, srv.URL+"/m", "pro")
	if err != nil || res.Tier != "pro" || slugs(res.Entries) != "p" {
		t.Errorf("Fetch = %+v, %v", res, err)
	}
}

// TestFetchTruncated: beyond maxPages the result is kept and marked.
func TestFetchTruncated(t *testing.T) {
	for _, total := range []bool{true, false} {
		srv, seen := aaServer(t, maxPages+10, total)
		res, err := Fetch(context.Background(), nil, srv.URL+"/m", "k")
		// maxPages free pages plus the full list's 403.
		if err != nil || !res.Truncated || len(res.Entries) != maxPages || len(seen()) != maxPages+1 {
			t.Errorf("total_pages=%t: %d entries, truncated %t, %d requests, %v", total, len(res.Entries), res.Truncated, len(seen()), err)
		}
	}
}

// TestFetchErrors: a failing free list reports the first error too; a
// failing page fails the fetch; a query in the base URL is kept.
func TestFetchErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/blocked" || r.URL.Path == "/blocked/free":
			http.Error(w, "blocked "+r.URL.Path, http.StatusForbidden)
		case r.URL.Query().Get("x") != "1":
			http.Error(w, "query lost: "+r.URL.RequestURI(), http.StatusBadRequest)
		case r.URL.Query().Get("page") == "2":
			http.Error(w, "page 2 broken", http.StatusInternalServerError)
		default:
			_, _ = w.Write([]byte(`{"pagination":{"total_pages":3,"has_more":true},"data":[]}`))
		}
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), nil, srv.URL+"/blocked", "k")
	if err == nil || !strings.HasPrefix(err.Error(), "HTTP 403 from "+srv.URL+"/blocked/free: blocked /blocked/free") ||
		!strings.HasSuffix(err.Error(), "(before that, the full list: HTTP 403 from "+srv.URL+"/blocked: blocked /blocked)") {
		t.Errorf("both errors: %v", err)
	}
	if _, err := Fetch(context.Background(), nil, srv.URL+"/m?x=1", "k"); err == nil || !strings.Contains(err.Error(), "page 2 broken") {
		t.Errorf("broken page: %v", err)
	}
}

// TestFetchFirstPageError: when the pages fail (e.g. a rate limit), the
// first error cancels the rest and is the only one reported.
func TestFetchFirstPageError(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("page") != "" {
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"pagination":{"total_pages":40,"has_more":true},"data":[]}`))
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), nil, srv.URL, "k")
	if err == nil || strings.Count(err.Error(), "HTTP 429") != 1 {
		t.Errorf("error: %v", err)
	}
	// Page 1, then at most the pages already running when the first failed.
	if n := calls.Load(); n > 1+2*parallelPages {
		t.Errorf("%d requests after the first error, want at most %d", n, 1+2*parallelPages)
	}
}

func TestNote(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.Local)
	var none *Store
	if !strings.Contains(none.Note(now), "none") {
		t.Error("no note without a cache")
	}
	fresh := &Store{FetchedAt: now.Add(-24 * time.Hour)}
	if n := fresh.Note(now); !strings.Contains(n, "2026-10-10") || strings.Contains(n, "old") {
		t.Errorf("fresh: %q", n)
	}
	// Exactly StaleAfter is not yet stale.
	limit := &Store{FetchedAt: now.Add(-StaleAfter)}
	if n := limit.Note(now); strings.Contains(n, "old") {
		t.Errorf("at the limit: %q", n)
	}
	old := &Store{FetchedAt: now.Add(-20 * 24 * time.Hour)}
	if n := old.Note(now); !strings.HasSuffix(n, ", 20 days old: run llmenu bench") {
		t.Errorf("old: %q", n)
	}
}
