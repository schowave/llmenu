package bench

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/schowave/llmenu/internal/catalog"
)

func f(v float64) *float64 { return &v }

func entry(slug string, intel, coding, tb *float64) Entry {
	e := Entry{Slug: slug}
	e.Evaluations.Intelligence, e.Evaluations.Coding, e.Evaluations.TerminalBench = intel, coding, tb
	return e
}

var aa = []Entry{
	entry("claude-opus-5-5", f(57.6), nil, f(0.5959)),
	entry("claude-opus-5-5-xhigh", f(56), f(80), f(0.6)),
	entry("claude-opus-5-5-low", f(42.3), nil, f(0.31)),
	entry("qwen3-8-flash-next", f(40), nil, nil),
	entry("qwen3-8-flash-next-non-reasoning", f(30), nil, nil),
	entry("mistral-large-3", f(25), nil, nil),
	entry("no-values", nil, nil, nil),
}

func TestMatch(t *testing.T) {
	models := []catalog.Model{
		{Name: "aws/claude-opus-5-5", Target: "bedrock/eu.anthropic.claude-opus-5-5"},
		{Name: "aws/claude-opus-latest", Target: "bedrock/eu.anthropic.claude-opus-5-5"},
		{Name: "local/Qwen3.8-Flash-Next", Target: "hosted_vllm/Qwen/Qwen3.8-Flash-Next-FP8"},
		{Name: "local/Qwen3.8-Flash-Next-non-thinking", Target: "hosted_vllm/Qwen/Qwen3.8-Flash-Next-FP8", NoThinking: true},
		{Name: "mistral/mistral-large-2512", Target: "mistral/mistral-large-2512"},
		{Name: "unknown/x-1", Target: "unknown/x-1"},
	}
	got := Match(aa, models, map[string]string{"mistral/mistral-large-2512": "mistral-large-3"})

	opus, ok := got["aws/claude-opus-5-5"]
	if !ok {
		t.Fatal("Opus has no match")
	}
	// Variants merged: best value per benchmark, shortest slug.
	if opus.Slug != "claude-opus-5-5" || *opus.Intelligence != 57.6 || *opus.Coding != 80 || *opus.TerminalBench != 60 {
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
}

func show(b catalog.Bench) string {
	s := b.Slug
	for _, v := range []*float64{b.Intelligence, b.Coding, b.TerminalBench} {
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

	got, err := Fetch(context.Background(), nil, srv.URL, "k")
	if err != nil || len(got) != 1 || *got[0].Evaluations.Intelligence != 50 {
		t.Fatalf("Fetch = %+v, %v", got, err)
	}
	if _, err := Fetch(context.Background(), nil, srv.URL, "wrong"); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Errorf("wrong key: %v", err)
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
	old := &Store{FetchedAt: now.Add(-20 * 24 * time.Hour)}
	if n := old.Note(now); !strings.Contains(n, "20 days old") {
		t.Errorf("old: %q", n)
	}
}
