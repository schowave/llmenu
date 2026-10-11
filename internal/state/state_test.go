package state

import (
	"os"
	"path/filepath"
	"testing"
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
	if _, err := os.Stat(filepath.Join(dir, "state.json.tmp")); !os.IsNotExist(err) {
		t.Error("temporary file left behind")
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
