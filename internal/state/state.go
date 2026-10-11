// Package state remembers the last chosen model per target between runs.
// The state is a convenience: if it is missing or broken, nothing is lost.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/schowave/llmenu/internal/guard"
)

// State is stored as JSON in the user's cache directory.
type State struct {
	LastModel map[string]string `json:"last_model"` // target -> model name; "" stands for `llmenu` without a target
}

// Path returns the storage location, e.g. ~/Library/Caches/llmenu/state.json.
func Path() (string, error) {
	p, err := path()
	if err != nil {
		return "", err
	}
	return guard.Path(p)
}

func path() (string, error) {
	if dir := os.Getenv("LLMENU_CACHE_DIR"); dir != "" {
		return filepath.Join(dir, "state.json"), nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "llmenu", "state.json"), nil
}

// Load reads the state; on any error it returns an empty one.
func Load() State {
	s := State{LastModel: map[string]string{}}
	p, err := Path()
	if err != nil {
		return s
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(b, &s)
	if s.LastModel == nil {
		s.LastModel = map[string]string{}
	}
	return s
}

// Save writes the state atomically.
func (s State) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
