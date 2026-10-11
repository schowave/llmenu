//go:build linux || darwin

package menu

// go-fuzzyfinder's mocked terminal can only be used from inside its own
// package, so these tests run Pick in a child process (this test binary) on a
// pseudo terminal and type into it like a user.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestPickHelper is the child process; it does nothing in a normal run.
func TestPickHelper(t *testing.T) {
	if os.Getenv("LLMENU_MENU_HELPER") != "1" {
		t.Skip("child process for the terminal tests")
	}
	preselect, _ := strconv.Atoi(os.Getenv("LLMENU_MENU_PRESELECT"))
	i, err := Pick(testModels, Options{Preselect: preselect})
	result := fmt.Sprintf("%d %v", i, err)
	// The parent test passes a file in its t.TempDir.
	if err := os.WriteFile(os.Getenv("LLMENU_MENU_RESULT"), []byte(result), 0o600); err != nil { //nolint:gosec // G703: see above
		t.Fatal(err)
	}
}

func TestPickInTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a child process on a pseudo terminal")
	}
	tests := []struct {
		name      string
		preselect int
		keys      []string // written one after the other, the screen settles in between
		want      string
	}{
		{"enter takes the preselected model", 2, []string{"\r"}, "2 <nil>"},
		// Without a preselection the cursor is on index 0, the best model,
		// which go-fuzzyfinder draws next to the prompt.
		{"no preselection starts on the first model", -1, []string{"\r"}, "0 <nil>"},
		{"query filters", 0, []string{"gpt", "\r"}, "1 <nil>"},
		{"cursor moves up", 0, []string{"\x1b[A", "\r"}, "1 <nil>"},
		{"esc aborts", 1, []string{"\x1b"}, "-1 " + ErrAborted.Error()},
		{"enter without a match aborts", 0, []string{"zzzz", "\r"}, "-1 " + ErrAborted.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickInTerminal(t, tt.preselect, tt.keys); got != tt.want {
				t.Errorf("Pick = %q, want %q", got, tt.want)
			}
		})
	}
}

// pickInTerminal runs TestPickHelper on a new pseudo terminal, types keys and
// returns the child's result "index error".
func pickInTerminal(t *testing.T, preselect int, keys []string) string {
	t.Helper()
	master, slavePath, err := openPTY()
	if err != nil {
		t.Skipf("no pseudo terminal: %v", err)
	}
	defer func() { _ = master.Close() }()
	slave, err := os.OpenFile(slavePath, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pseudo terminal: %v", err)
	}
	defer func() { _ = slave.Close() }()

	result := filepath.Join(t.TempDir(), "result")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPickHelper$", "-test.count=1") //nolint:gosec // G702: this test binary
	cmd.Env = append(os.Environ(),
		"LLMENU_MENU_HELPER=1",
		"LLMENU_MENU_PRESELECT="+strconv.Itoa(preselect),
		"LLMENU_MENU_RESULT="+result,
		"TERM=xterm-256color", "LINES=30", "COLUMNS=120",
	)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	// The pseudo terminal becomes the child's controlling terminal, /dev/tty.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	screen := &output{}
	go screen.drain(master)

	if !screen.waitFor("model>", 20*time.Second) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("menu did not appear:\n%q", screen.String())
	}
	for _, k := range keys {
		screen.settle()
		if _, err := master.WriteString(k); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.Wait(); err != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Logf("child: %v", err)
	}
	b, err := os.ReadFile(result)
	if err != nil {
		t.Fatalf("no result: %v\nterminal:\n%q", err, screen.String())
	}
	return strings.TrimSpace(string(b))
}

// output collects what the child draws.
type output struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	last time.Time
}

func (o *output) drain(r *os.File) {
	b := make([]byte, 4096)
	for {
		n, err := r.Read(b)
		o.mu.Lock()
		o.buf.Write(b[:n])
		o.last = time.Now()
		o.mu.Unlock()
		if err != nil {
			return
		}
	}
}

func (o *output) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

func (o *output) waitFor(s string, timeout time.Duration) bool {
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if strings.Contains(o.String(), s) {
			return true
		}
	}
	return false
}

// settle waits until the child has drawn nothing for a moment, at most 5 s.
func (o *output) settle() {
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		o.mu.Lock()
		quiet := time.Since(o.last) > 200*time.Millisecond
		o.mu.Unlock()
		if quiet {
			return
		}
	}
}
