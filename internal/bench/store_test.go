package bench

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/schowave/llmenu/internal/catalog"
)

func TestStoreSaveLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	t.Setenv("LLMENU_CACHE_DIR", dir)

	got, err := Load()
	if err != nil || got != nil {
		t.Fatalf("Load without cache = %+v, %v; want nil, nil", got, err)
	}

	s := &Store{
		FetchedAt: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC),
		Models:    map[string]catalog.Bench{"aws/claude-opus-5-5": {Slug: "claude-opus-5-5", Intelligence: f(57.6)}},
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if p, _ := Path(); p != filepath.Join(dir, "bench.json") {
		t.Errorf("Path() = %q", p)
	}
	got, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	b := got.Models["aws/claude-opus-5-5"]
	if !got.FetchedAt.Equal(s.FetchedAt) || len(got.Models) != 1 || b.Slug != "claude-opus-5-5" ||
		b.Intelligence == nil || *b.Intelligence != 57.6 || b.Coding != nil {
		t.Errorf("Load() = %+v, want %+v", got, s)
	}
}

func TestPathDefault(t *testing.T) {
	t.Setenv("LLMENU_CACHE_DIR", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", t.TempDir())
	dir, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("no cache directory: %v", err)
	}
	if p, err := Path(); err != nil || p != filepath.Join(dir, "llmenu", "bench.json") {
		t.Errorf("Path() = %q, %v; want it in %s", p, err, dir)
	}

	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		return // the cache directory does not depend on HOME there
	}
	t.Setenv("HOME", "")
	if p, err := Path(); err == nil {
		t.Errorf("Path() without HOME = %q, want an error", p)
	}
}

func TestLoadCorrupt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LLMENU_CACHE_DIR", dir)
	p := filepath.Join(dir, "bench.json")
	if err := os.WriteFile(p, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s, err := Load(); err == nil || s != nil || !strings.Contains(err.Error(), p) {
		t.Errorf("Load() = %+v, %v; want an error naming %s", s, err, p)
	}
}

func TestApply(t *testing.T) {
	models := []catalog.Model{{Name: "a"}, {Name: "b"}}
	var none *Store
	none.Apply(models) // nil-safe
	if models[0].Bench != nil {
		t.Fatal("nil store attached values")
	}

	s := &Store{Models: map[string]catalog.Bench{"a": {Slug: "slug-a", Coding: f(70)}, "c": {Slug: "slug-c"}}}
	s.Apply(models)
	if b := models[0].Bench; b == nil || b.Slug != "slug-a" || *b.Coding != 70 {
		t.Errorf("a: %+v", b)
	}
	if models[1].Bench != nil {
		t.Errorf("b: %+v, want none", models[1].Bench)
	}
	// Each model gets its own copy.
	models[0].Bench.Slug = "changed"
	if s.Models["a"].Slug != "slug-a" {
		t.Error("Apply shares the store's values")
	}
}

func TestFetchUnexpected(t *testing.T) {
	tests := []struct{ name, body string }{
		{"malformed JSON", `{"data":[`},
		{"no data array", `{"models":[]}`},
		{"data null", `{"data":null}`},
		{"data not an array", `{"data":{"slug":"a"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			got, err := Fetch(context.Background(), nil, srv.URL, "k")
			if err == nil || !strings.Contains(err.Error(), "no data array") {
				t.Errorf("Fetch = %+v, %v", got, err)
			}
		})
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()
	if got, err := Fetch(context.Background(), nil, srv.URL, "k"); err != nil || got == nil || len(got) != 0 {
		t.Errorf("empty data array: %+v, %v", got, err)
	}
}
