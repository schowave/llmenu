package state

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LLMENU_CACHE_DIR", dir)
	p, err := Path()
	if err != nil || p != filepath.Join(dir, "state.json") {
		t.Errorf("Path() = %q, %v", p, err)
	}

	t.Setenv("LLMENU_CACHE_DIR", "")
	p, err = Path()
	if err == nil && filepath.Base(filepath.Dir(p)) != "llmenu" {
		t.Errorf("Path() = %q, want .../llmenu/state.json", p)
	}
}

func TestLoadBroken(t *testing.T) {
	tests := []struct {
		name    string
		content *string // nil: no file
	}{
		{"missing file", nil},
		{"corrupt file", ptr("{not json")},
		{"null map", ptr(`{"last_model": null}`)},
		{"wrong type", ptr(`{"last_model": 42}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("LLMENU_CACHE_DIR", dir)
			if tt.content != nil {
				if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(*tt.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			s := Load()
			if s.LastModel == nil || len(s.LastModel) != 0 {
				t.Errorf("Load() = %+v, want an empty state", s)
			}
			s.LastModel["x"] = "y" // must be writable
		})
	}
}

func TestSaveLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "cache")
	t.Setenv("LLMENU_CACHE_DIR", dir)

	s := Load()
	s.LastModel[""] = "aws/claude-opus-5-5"
	s.LastModel["codex"] = "azure/gpt-5.6-sol"
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if es, _ := os.ReadDir(dir); len(es) != 1 {
		t.Errorf("temporary file left behind: %v", es)
	}
	got := Load()
	if len(got.LastModel) != 2 || got.LastModel[""] != "aws/claude-opus-5-5" || got.LastModel["codex"] != "azure/gpt-5.6-sol" {
		t.Errorf("Load() = %+v, want %+v", got, s)
	}
}

func TestSaveError(t *testing.T) {
	// The cache directory is a file, so it cannot be created.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LLMENU_CACHE_DIR", filepath.Join(file, "sub"))
	if err := (State{}).Save(); err == nil {
		t.Error("Save into a file succeeded")
	}
}

func ptr(s string) *string { return &s }

func TestModelsCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LLMENU_CACHE_DIR", dir)
	if got := LoadModels(); got != nil {
		t.Errorf("empty cache: %v", got)
	}
	names := []string{"a/x", "b/y"}
	if err := SaveModels(names); err != nil {
		t.Fatal(err)
	}
	if got := LoadModels(); !slices.Equal(got, names) {
		t.Errorf("LoadModels = %v, want %v", got, names)
	}

	// Unchanged names are not written again.
	p := filepath.Join(dir, "models.json")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if err := SaveModels(names); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); !fi.ModTime().Equal(old) {
		t.Error("unchanged model list rewritten")
	}

	if err := os.WriteFile(p, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadModels(); got != nil {
		t.Errorf("corrupt cache: %v", got)
	}

	// A cache "directory" that is a file: saving fails, loading returns nil.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LLMENU_CACHE_DIR", file)
	if err := SaveModels(names); err == nil {
		t.Error("save into a file path succeeded")
	}
	if got := LoadModels(); got != nil {
		t.Errorf("load from a file path: %v", got)
	}
}
