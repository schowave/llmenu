package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/config"
)

// llmenu without a command asks once for the default tool, then starts it
// after the menu; captured output keeps getting only the model name.
func TestRootDefaultTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake claude is a shell script")
	}
	fakeGateway(t)
	bin := t.TempDir()
	ran := filepath.Join(t.TempDir(), "claude-args")
	script := "#!/bin/sh\necho \"$@\" > " + ran + "\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o700); err != nil { //nolint:gosec // G306: must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("LLMENU_ACCESSIBLE", "1")
	cfg := &config.Config{}
	cfg.SetTools([]string{"claude", "pi"})
	if _, err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	p := &stubPicker{choose: func(models []catalog.Model) (int, error) { return 0, nil }}
	p.install(t)

	stdoutTerminal := true
	orig := isTerminal
	isTerminal = func(f *os.File) bool { return f != os.Stdout || stdoutTerminal }
	t.Cleanup(func() { isTerminal = orig })

	run := func(answers string) (string, string, error) {
		t.Helper()
		in, err := os.CreateTemp(t.TempDir(), "stdin")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = in.WriteString(answers)
		_, _ = in.Seek(0, 0)
		origIn := os.Stdin
		os.Stdin = in
		defer func() { os.Stdin = origIn; _ = in.Close() }()
		_ = os.Remove(ran)
		var out bytes.Buffer
		stderr := captureStderr(t, func() {
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{})
			err = cmd.ExecuteContext(context.Background())
		})
		return out.String(), stderr, err
	}
	launched := func() string {
		b, _ := os.ReadFile(ran)
		return string(b)
	}

	// Unset: asked once, pi cannot be started, the first tool is preselected.
	_, stderr, err := run("\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "without a command do?") || !strings.Contains(stderr, "Start Claude Code") || strings.Contains(stderr, "Start pi") {
		t.Errorf("question: %q", stderr)
	}
	if got, _ := config.Load(); got.Launch != "claude" {
		t.Errorf("launch = %q", got.Launch)
	}
	if !strings.HasPrefix(launched(), "--model ") {
		t.Errorf("claude not started after the question: %q", launched())
	}

	// Chosen: no question (stdin is empty), straight to the menu and claude.
	if _, stderr, err := run(""); err != nil || strings.Contains(stderr, "without a command do?") {
		t.Fatalf("second run: %v, %q", err, stderr)
	}
	if !strings.HasPrefix(launched(), "--model ") {
		t.Errorf("claude not started: %q", launched())
	}

	// Captured output, as in "$(llmenu)": only the name.
	stdoutTerminal = false
	out, _, err := run("")
	if err != nil || out != p.shown[0].Name+"\n" || launched() != "" {
		t.Errorf("captured: %q, %v, launched %q", out, err, launched())
	}
	stdoutTerminal = true

	// none: only the name, and no question either.
	if err := setDefault(mustLoad(t), config.NoLaunch); err != nil {
		t.Fatal(err)
	}
	out, _, err = run("")
	if err != nil || out != p.shown[0].Name+"\n" || launched() != "" {
		t.Errorf("none: %q, %v, launched %q", out, err, launched())
	}
}

func mustLoad(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
