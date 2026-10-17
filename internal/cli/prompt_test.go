package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/config"
	"github.com/schowave/llmenu/internal/menu"
)

// stubTerminal makes isTerminal report term for the duration of the test.
func stubTerminal(t *testing.T, term bool) {
	t.Helper()
	orig := isTerminal
	isTerminal = func(*os.File) bool { return term }
	t.Cleanup(func() { isTerminal = orig })
}

// captureStderr returns what f writes to os.Stderr.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	tmp, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = tmp
	f()
	os.Stderr = orig
	// Close before reading and before cleanup: Windows cannot delete open files.
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// fakeGateway serves the gateway test data and points llmenu at it.
func fakeGateway(t *testing.T) {
	t.Helper()
	modelInfo, err := os.ReadFile(filepath.Join("..", "gateway", "testdata", "model_info.json"))
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(modelInfo)
	}))
	t.Cleanup(gw.Close)
	t.Setenv("LLMENU_BASE_URL", gw.URL+"/")
	t.Setenv("LLMENU_API_KEY", "sk-test")
	t.Setenv("LLMENU_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("LLMENU_CACHE_DIR", t.TempDir())
}

// huh starts a new scanner for every question; lineReader must not let one
// scanner swallow the next answer, and must notice when stdin runs out.
func TestLineReader(t *testing.T) {
	r := &lineReader{r: strings.NewReader("first\nlast")}
	for _, want := range []string{"first", "last"} {
		sc := bufio.NewScanner(r)
		if !sc.Scan() || sc.Text() != want {
			t.Fatalf("scan = %q, want %q", sc.Text(), want)
		}
		if r.ended {
			t.Fatalf("ended after answer %q", want)
		}
	}
	if bufio.NewScanner(r).Scan() || !r.ended {
		t.Errorf("no answer left: ended = %v, want true", r.ended)
	}

	r = &lineReader{r: strings.NewReader("only\n")}
	bufio.NewScanner(r).Scan()
	if r.ended {
		t.Error("ended after a complete line")
	}
	if n, err := r.Read(nil); n != 0 || err != nil {
		t.Errorf("Read(nil) = %d, %v", n, err)
	}
	bufio.NewScanner(r).Scan()
	if !r.ended {
		t.Error("EOF right after a newline not noticed")
	}
}

func TestCanPromptAndSecretEcho(t *testing.T) {
	for _, tc := range []struct {
		accessible string
		term       bool
		prompt     bool
		echo       huh.EchoMode
	}{
		{"", false, false, huh.EchoModePassword},
		{"", true, true, huh.EchoModePassword},
		{"1", false, true, huh.EchoModeNormal},
		{"true", true, true, huh.EchoModePassword},
		{"0", false, false, huh.EchoModePassword},
	} {
		t.Setenv("LLMENU_ACCESSIBLE", tc.accessible)
		stubTerminal(t, tc.term)
		if got := canPrompt(); got != tc.prompt {
			t.Errorf("accessible=%q term=%v: canPrompt = %v", tc.accessible, tc.term, got)
		}
		if got := secretEcho(); got != tc.echo {
			t.Errorf("accessible=%q term=%v: secretEcho = %v", tc.accessible, tc.term, got)
		}
	}
}

func TestTitledAndEnterKeeps(t *testing.T) {
	cur := "https://llm.example.com/"
	v := enterKeeps(&cur, validateURL)

	t.Setenv("LLMENU_ACCESSIBLE", "")
	if titled("T", "D") != "T" {
		t.Error("description added outside accessible mode")
	}
	if v("") == nil {
		t.Error("empty input accepted outside accessible mode")
	}

	t.Setenv("LLMENU_ACCESSIBLE", "1")
	if got := titled("T", "D"); got != "T\nD" {
		t.Errorf("titled = %q", got)
	}
	if err := v(""); err != nil {
		t.Errorf("Enter on a valid current value: %v", err)
	}
	if v("nope") == nil {
		t.Error("invalid input accepted")
	}
	cur = ""
	if v("") == nil {
		t.Error("Enter without a current value accepted")
	}
}

// fakePrompt records the reader runPrompt passes to huh.
type fakePrompt struct {
	r       io.Reader
	err     error
	runs    int
	tuiRuns int
}

func (p *fakePrompt) Run() error { p.tuiRuns++; return p.err }

func (p *fakePrompt) RunAccessible(_ io.Writer, r io.Reader) error {
	p.runs++
	p.r = r
	return p.err
}

func TestRunPrompt(t *testing.T) {
	t.Setenv("LLMENU_ACCESSIBLE", "")
	p := &fakePrompt{}
	if err := runPrompt(p); err != nil || p.tuiRuns != 1 || p.runs != 0 {
		t.Fatalf("form mode: err=%v tui=%d accessible=%d", err, p.tuiRuns, p.runs)
	}

	t.Setenv("LLMENU_ACCESSIBLE", "1")
	stubTerminal(t, true)
	p = &fakePrompt{err: errors.New("boom")}
	if err := runPrompt(p); err == nil || err.Error() != "boom" || p.runs != 1 || p.tuiRuns != 0 {
		t.Fatalf("accessible: err=%v tui=%d accessible=%d", err, p.tuiRuns, p.runs)
	}
	// Hidden input reads the terminal directly and needs its descriptor.
	if _, ok := p.r.(interface{ Fd() uintptr }); !ok {
		t.Error("terminal reader without Fd")
	}

	stubTerminal(t, false)
	_ = runPrompt(p)
	if _, ok := p.r.(interface{ Fd() uintptr }); ok {
		t.Error("pipe passed with Fd")
	}
}

func TestRootCmd(t *testing.T) {
	fakeGateway(t)
	stubTerminal(t, true)
	p := &stubPicker{choose: func(models []catalog.Model) (int, error) { return len(models) - 1, nil }}
	p.install(t)

	run := func() (string, error) {
		var out bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&out)
		cmd.SetArgs([]string{})
		err := cmd.ExecuteContext(context.Background())
		return out.String(), err
	}
	out, err := run()
	if err != nil || out != p.shown[len(p.shown)-1].Name+"\n" {
		t.Errorf("menu choice: %q, %v", out, err)
	}

	p.choose = func([]catalog.Model) (int, error) { return -1, menu.ErrAborted }
	out, err = run()
	var ec exitCodeError
	if !errors.As(err, &ec) || ec.code != 130 || out != "" {
		t.Errorf("aborted menu: %q, %v", out, err)
	}

	// Not set up and setup declined: the menu never opens.
	t.Setenv("LLMENU_BASE_URL", "")
	origConfirm := confirmSetup
	t.Cleanup(func() { confirmSetup = origConfirm })
	confirmSetup = func() (bool, error) { return false, nil }
	p.choose = func([]catalog.Model) (int, error) { t.Fatal("menu opened"); return 0, nil }
	if _, err := run(); !errors.Is(err, errNotConfigured) {
		t.Errorf("not set up: %v", err)
	}
}

func TestChooseModelStateNotSaved(t *testing.T) {
	p := &stubPicker{choose: func([]catalog.Model) (int, error) { return 0, nil }}
	p.install(t)

	t.Setenv("LLMENU_CACHE_DIR", t.TempDir())
	if got := captureStderr(t, func() {
		_, _ = chooseModel(gatewayModels, choice{config: &config.Config{}, key: "claude", interactive: true})
	}); got != "" {
		t.Errorf("stderr %q after a saved choice", got)
	}

	// The cache "directory" is a file: the choice still counts, with a note.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LLMENU_CACHE_DIR", file)
	var got *catalog.Model
	stderr := captureStderr(t, func() {
		got, _ = chooseModel(gatewayModels, choice{config: &config.Config{}, key: "claude", interactive: true})
	})
	if got == nil || !strings.Contains(stderr, "llmenu: selection not saved:") {
		t.Errorf("chosen %v, stderr %q", got, stderr)
	}
}

func TestWarnKeySource(t *testing.T) {
	// No llmenu on PATH, key only from the environment: both warnings.
	t.Setenv("PATH", t.TempDir())
	t.Setenv("LLMENU_API_KEY", "sk-env")
	got := captureStderr(t, warnKeySource)
	if !strings.Contains(got, "llmenu is not on the PATH") || !strings.Contains(got, "comes from LLMENU_API_KEY") {
		t.Errorf("warnings %q", got)
	}

	// llmenu on PATH and the key in the keyring: nothing to warn about.
	bin := t.TempDir()
	name := "llmenu"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o700); err != nil { //nolint:gosec // G306: must be executable for LookPath
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("LLMENU_API_KEY", "")
	keys := filepath.Join(t.TempDir(), "keyring.json")
	if err := os.WriteFile(keys, []byte(`{"llmenu-test/gateway": "sk-file"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LLMENU_KEYRING_FILE", keys)
	t.Setenv("LLMENU_KEYRING_SERVICE", "llmenu-test")
	if got := captureStderr(t, warnKeySource); got != "" {
		t.Errorf("unexpected warnings %q", got)
	}
}

func TestEscForm(t *testing.T) {
	esc := tea.KeyMsg{Type: tea.KeyEsc}
	m := &escForm{Form: formOf(huh.NewSelect[string]().Options(huh.NewOptions("a", "b")...))}
	m.Init()

	// While filtering, Esc ends the filter and stays in the dialog.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m.Update(esc)
	if m.back || m.View() == "" {
		t.Fatal("esc while filtering left the dialog")
	}
	m.Update(esc)
	if !m.back || m.View() != "" {
		t.Fatalf("esc: back=%v view=%q", m.back, m.View())
	}
	if !errors.Is(errBack, huh.ErrUserAborted) {
		t.Error("errBack is no abort")
	}
}
