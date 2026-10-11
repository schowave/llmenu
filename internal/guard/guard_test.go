package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPath(t *testing.T) {
	tmp := t.TempDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	for _, tc := range []struct {
		path string
		ok   bool
	}{
		{filepath.Join(tmp, "config.toml"), true},
		{filepath.Join(tmp, "not", "yet", "created.json"), true},
		{filepath.Join(os.TempDir(), "x"), true},
		{filepath.Join(home, ".pi", "agent", "models.json"), false},
		{filepath.Join(tmp, "..", "..", "..", "..", "..", "..", "..", "etc", "passwd"), false},
		{strings.TrimRight(os.TempDir(), `/\`) + "-sibling", false},
		{"relative/inside/cwd", false}, // the package dir, not temp
	} {
		got, err := Path(tc.path)
		if tc.ok && (err != nil || got != tc.path) {
			t.Errorf("Path(%q) = %q, %v; want allowed", tc.path, got, err)
		}
		if !tc.ok && (err == nil || !strings.Contains(err.Error(), "outside the temp directory")) {
			t.Errorf("Path(%q) = %q, %v; want refused", tc.path, got, err)
		}
	}
}

func TestResolveFollowsSymlinks(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	real, _ := filepath.EvalSymlinks(target)
	if got := resolve(filepath.Join(link, "missing", "file")); got != filepath.Join(real, "missing", "file") {
		t.Errorf("resolve = %q", got)
	}
}
