package cli

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/schowave/llmenu/internal/config"
	"github.com/schowave/llmenu/internal/targets"
)

const (
	// statusLineWait bounds the user's own status line command.
	statusLineWait = 3 * time.Second
	// statusSpendWait bounds the gateway request; a cached value is shown
	// when it does not answer in time. A project key's budget needs
	// /user/info, which takes over a second on large teams; the cache keeps
	// the wait to about once a minute.
	statusSpendWait = 3 * time.Second
	// statusLineEnv is set for the user's own command, so a script that
	// calls llmenu statusline again neither loops nor repeats the line.
	statusLineEnv = "LLMENU_STATUSLINE"
)

// newStatusLineCmd prints the status line for Claude Code (claude.statusline).
// It adds to the user's own status line: that command runs with the same
// session JSON, and llmenu appends one line with the project and the key's
// spend. Without an own status line, llmenu's line also names the model.
func newStatusLineCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "statusline",
		Short:  "Status line for Claude Code: your own, plus project and spend",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Called again from the user's own status line: the outer
			// llmenu statusline adds the line, so nothing twice.
			if os.Getenv(statusLineEnv) != "" {
				return nil
			}
			// A status line never fails: whatever is missing is left out.
			in, _ := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 1<<20))
			var session struct {
				Model struct {
					DisplayName string `json:"display_name"`
				} `json:"model"`
				Cwd       string `json:"cwd"`
				Workspace struct {
					CurrentDir string `json:"current_dir"`
					ProjectDir string `json:"project_dir"`
				} `json:"workspace"`
			}
			_ = json.Unmarshal(in, &session)
			cfg, err := config.Load()
			if err != nil {
				cfg = &config.Config{}
			}
			dir := cmp.Or(session.Workspace.CurrentDir, session.Cwd)

			var own, spent string
			var wg sync.WaitGroup
			if c := ownStatusLine(cmp.Or(session.Workspace.ProjectDir, dir)); c != "" {
				wg.Go(func() { own = runStatusLine(cmd.Context(), c, in, dir) })
			}
			wg.Go(func() { spent = sessionSpend(cmd.Context()) })
			wg.Wait()

			var parts []string
			if p := currentProject(cfg); p != "" {
				parts = append(parts, "project "+p)
			} else if spent != "" {
				parts = append(parts, "main key")
			}
			if m := session.Model.DisplayName; m != "" && own == "" {
				parts = append(parts, m)
			}
			if spent != "" {
				parts = append(parts, spent)
			}
			var lines []string
			if own != "" {
				lines = append(lines, own)
			}
			if len(parts) > 0 {
				lines = append(lines, strings.Join(parts, " · "))
			}
			fmt.Fprintln(cmd.OutOrStdout(), strings.Join(lines, "\n"))
			return nil
		},
	}
}

// sessionSpend is the spend of the session's key. Claude Code passes the
// environment llmenu started it with to the status line, so the key comes
// from there, not from the keyring, and goes to the gateway the session
// talks to anyway. Outside a session llmenu started, nothing is asked.
func sessionSpend(ctx context.Context) string {
	base, key := os.Getenv("ANTHROPIC_BASE_URL"), os.Getenv("ANTHROPIC_AUTH_TOKEN")
	if os.Getenv(targets.ProjectEnv) == "" || base == "" || key == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, statusSpendWait)
	defer cancel()
	if b, ok := budget(ctx, base, key); ok {
		return formatBudget(b, now())
	}
	return ""
}

// ownStatusLine is the status line command Claude Code would run without
// llmenu: the first statusLine in the project's local and shared settings,
// then the user's. llmenu's own status line there counts as none.
func ownStatusLine(projectDir string) string {
	var files []string
	if projectDir != "" {
		files = append(files,
			filepath.Join(projectDir, ".claude", "settings.local.json"),
			filepath.Join(projectDir, ".claude", "settings.json"))
	}
	if d := claudeConfigDir(); d != "" {
		files = append(files, filepath.Join(d, "settings.json"))
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var s struct {
			StatusLine *struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"statusLine"`
		}
		if json.Unmarshal(b, &s) != nil || s.StatusLine == nil {
			continue
		}
		if s.StatusLine.Type != "command" || strings.Contains(s.StatusLine.Command, "llmenu statusline") {
			return ""
		}
		return s.StatusLine.Command
	}
	return ""
}

func claudeConfigDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// runStatusLine runs the user's status line command the way Claude Code
// does: through the shell, with the session JSON on stdin. Its output
// without the trailing newline; empty on any failure.
func runStatusLine(ctx context.Context, command string, in []byte, dir string) string {
	ctx, cancel := context.WithTimeout(ctx, statusLineWait)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(in)
	cmd.Env = append(os.Environ(), statusLineEnv+"=1")
	// Background children of the command (e.g. a tee to curl) may hold the
	// output open; do not wait for them.
	cmd.WaitDelay = 500 * time.Millisecond
	out, _ := cmd.Output()
	return strings.TrimRight(string(out), "\r\n")
}
