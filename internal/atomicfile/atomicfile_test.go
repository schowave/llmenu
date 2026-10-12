package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sub", "f.json")
	if err := Write(file, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(file, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(file); string(b) != "two" {
		t.Errorf("content %q", b)
	}
	if es, _ := os.ReadDir(filepath.Dir(file)); len(es) != 1 {
		t.Errorf("temporary files left behind: %v", es)
	}
}

func TestWriteDoesNotFollowSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges")
	}
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Predictable names an attacker could plant.
	for _, n := range []string{"f.tmp", "f.llmenu.tmp", "f.bak"} {
		if err := os.Symlink(victim, filepath.Join(dir, n)); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"f", "f.bak"} {
		if err := Write(filepath.Join(dir, n), []byte("new"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Errorf("symlink target overwritten: %q", b)
	}
}

func TestWriteSetsMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no Unix modes")
	}
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o666); err != nil { //nolint:gosec // the point of the test
		t.Fatal(err)
	}
	if err := Write(file, []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(file); fi.Mode().Perm() != 0o640 {
		t.Errorf("mode %v", fi.Mode())
	}
}
