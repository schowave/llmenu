// Package guard keeps tests away from the user's real files. While running
// under `go test`, every path llmenu reads or writes must lie inside the OS
// temp directory (where t.TempDir and testscript work). If an override such as
// LLMENU_CONFIG is lost — for example because mutation testing flipped the
// check — the test fails instead of touching ~/.pi, ~/.config or the real
// llmenu config. Outside of tests this is a no-op.
package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Path returns p unchanged, or an error if a test would touch a file outside
// the temp directory.
func Path(p string) (string, error) {
	if !testing.Testing() {
		return p, nil
	}
	if !inTemp(p) {
		return "", fmt.Errorf("guard: test tried to use %s outside the temp directory; set LLMENU_CONFIG, LLMENU_CACHE_DIR or HOME to a temp dir", p)
	}
	return p, nil
}

func inTemp(p string) bool {
	abs, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	tmp := resolve(os.TempDir())
	rel, err := filepath.Rel(tmp, resolve(abs))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolve follows symlinks of the longest existing prefix of p (on macOS the
// temp directory lives behind /var -> /private/var).
func resolve(p string) string {
	p = filepath.Clean(p)
	rest := ""
	for {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(p, rest)
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}
