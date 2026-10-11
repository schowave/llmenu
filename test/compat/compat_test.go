//go:build compat

// Package compat runs the real tools (pi, opencode, Codex CLI) against the
// configuration llmenu generates for them. The unit tests in internal/targets
// only cover formats we know; these tests catch a tool changing its format
// (ADR-0002, "Consequences").
//
// No real gateway or key is needed: a local fake serves /model/info, the key
// comes from LLMENU_API_KEY (so the OS keychain is never touched) and HOME
// points to a temporary directory.
//
//	go test -tags compat -v ./test/compat/
//
// A tool that is not on the PATH is skipped, unless LLMENU_COMPAT_REQUIRE=1.
package compat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	// provider is set as gateway.name so the provider id does not depend on
	// the fake server's address.
	provider = "compat"
	apiKey   = "sk-test"
	// codexModel reports /v1/responses in testdata/model_info.json.
	codexModel = "azure/gpt-6-sol"
	timeout    = 60 * time.Second
)

// chatModels are the chat models in testdata/model_info.json that every sync
// target must list; embeddingModel must not show up anywhere.
var (
	chatModels = []string{
		"aws/claude-sonnet-4-5",
		"aws/claude-opus-5-5",
		"azure/gpt-6-sol",
		"local/Qwen3-Next",
		"mistral/mistral-large-4",
	}
	embeddingModel = "azure/text-embedding"
)

// llmenuBin is the llmenu binary built in TestMain; its directory comes first
// on the PATH of every command, because pi calls `llmenu secrets get gateway`.
var llmenuBin string

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "llmenu-compat-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()

	root, err := moduleRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	llmenuBin = filepath.Join(dir, "llmenu")
	if runtime.GOOS == "windows" {
		llmenuBin += ".exe"
	}
	build := exec.Command("go", "build", "-o", llmenuBin, "./cmd/llmenu")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building llmenu: %v\n%s", err, out)
		return 1
	}
	return m.Run()
}

func moduleRoot() (string, error) {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return "", fmt.Errorf("go env GOMOD: %w", err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		return "", errors.New("not inside a Go module")
	}
	return filepath.Dir(gomod), nil
}

// sandbox is an isolated environment: fake gateway, temporary HOME and
// llmenu configuration.
type sandbox struct {
	dir string
	env []string
}

func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "model_info.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/model/info" || r.Header.Get("Authorization") != "Bearer "+apiKey {
			t.Errorf("fake gateway: unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	for _, d := range []string{home, filepath.Join(dir, "codex")} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}

	set := map[string]string{
		"PATH": filepath.Dir(llmenuBin) + string(os.PathListSeparator) + os.Getenv("PATH"),
		// The environment takes precedence over the keychain, so the OS
		// keychain is never touched; the service name is a safety net.
		"LLMENU_API_KEY":         apiKey,
		"LLMENU_KEYRING_SERVICE": "llmenu-compat-test",
		"LLMENU_BASE_URL":        srv.URL + "/",
		"LLMENU_CONFIG":          filepath.Join(dir, "config.toml"),
		"LLMENU_CACHE_DIR":       filepath.Join(dir, "cache"),
		// Safe only because nothing here reads the keychain (see above).
		"HOME":            home,
		"USERPROFILE":     home,
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"XDG_DATA_HOME":   filepath.Join(home, ".local", "share"),
		"XDG_CACHE_HOME":  filepath.Join(home, ".cache"),
		"XDG_STATE_HOME":  filepath.Join(home, ".local", "state"),
		"CODEX_HOME":      filepath.Join(dir, "codex"),
		// opencode: no models.dev download, no update check.
		"OPENCODE_DISABLE_MODELS_FETCH": "true",
		"OPENCODE_DISABLE_AUTOUPDATE":   "true",
		"NO_COLOR":                      "1",
	}
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if _, ok := set[k]; ok || strings.HasPrefix(k, "LLMENU_") || strings.HasPrefix(k, "OPENCODE") {
			continue
		}
		env = append(env, kv)
	}
	for k, v := range set {
		env = append(env, k+"="+v)
	}

	s := &sandbox{dir: dir, env: env}
	s.mustRun(t, llmenuBin, "config", "set", "gateway.name", provider)
	return s
}

// run executes a command in the sandbox and returns its combined output.
func (s *sandbox) run(t *testing.T, name string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = s.env
	cmd.Dir = s.dir
	cmd.Stdin = nil // not a terminal: llmenu never opens a menu
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	if ctx.Err() != nil {
		err = fmt.Errorf("timed out after %s: %w", timeout, err)
	}
	t.Logf("$ %s %s\n%s", filepath.Base(name), strings.Join(args, " "), truncate(out.String()))
	return out.String(), err
}

// truncate shortens long output (codex features list, pi's built-in models)
// in the test log.
func truncate(s string) string {
	const limit = 3000
	if len(s) <= limit {
		return s
	}
	return s[:limit] + fmt.Sprintf("\n… (%d more bytes)\n", len(s)-limit)
}

func (s *sandbox) mustRun(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := s.run(t, name, args...)
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", filepath.Base(name), strings.Join(args, " "), err, out)
	}
	return out
}

// requireTool returns the tool's path, or skips (fails with
// LLMENU_COMPAT_REQUIRE=1) if it is not installed.
func requireTool(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		if os.Getenv("LLMENU_COMPAT_REQUIRE") == "1" {
			t.Fatalf("%s not on PATH, but LLMENU_COMPAT_REQUIRE=1", name)
		}
		t.Skipf("%s not on PATH", name)
	}
	return p
}

// listedModels collects the models the tool lists under our provider. It
// understands both pi's table ("<provider>  <model>  …") and opencode's
// "<provider>/<model>" lines; rows of other providers are ignored.
func listedModels(out string) map[string]bool {
	got := map[string]bool{}
	for line := range strings.Lines(out) {
		f := strings.Fields(line)
		switch {
		case len(f) >= 2 && f[0] == provider:
			got[f[1]] = true
		case len(f) >= 1 && strings.HasPrefix(f[0], provider+"/"):
			got[strings.TrimPrefix(f[0], provider+"/")] = true
		}
	}
	return got
}

// assertModels checks that every chat model of the fake gateway is listed
// under our provider, and the embedding model is not.
func assertModels(t *testing.T, out string) {
	t.Helper()
	got := listedModels(out)
	if len(got) == 0 {
		t.Fatalf("no models listed under provider %q", provider)
	}
	for _, m := range chatModels {
		if !got[m] {
			t.Errorf("model %q not listed under provider %q", m, provider)
		}
	}
	if got[embeddingModel] {
		t.Errorf("embedding model %q listed, sync must only write chat models", embeddingModel)
	}
	if strings.Contains(out, apiKey) {
		t.Error("the tool printed the API key")
	}
}

// TestPi: `llmenu sync pi` writes ~/.pi/agent/models.json; `pi --list-models`
// must load it, resolve the key via `!llmenu secrets get gateway` and list
// every chat model under our provider.
func TestPi(t *testing.T) {
	pi := requireTool(t, "pi")
	s := newSandbox(t)
	s.mustRun(t, llmenuBin, "sync", "pi")

	assertModels(t, s.mustRun(t, pi, "--list-models"))
}

// TestOpencode: `llmenu sync opencode` writes ~/.config/opencode/opencode.json;
// `opencode models <provider>` must accept the configuration and list every
// chat model as <provider>/<model>.
func TestOpencode(t *testing.T) {
	oc := requireTool(t, "opencode")
	s := newSandbox(t)
	s.mustRun(t, llmenuBin, "sync", "opencode")

	assertModels(t, s.mustRun(t, oc, "models", provider))
}

// TestCodex: there is no gateway that could answer, so this checks that Codex
// accepts the -c overrides llmenu generates (model_provider, model_providers.*,
// model, model_context_window, model_max_output_tokens) with a subcommand that
// loads the configuration and exits.
func TestCodex(t *testing.T) {
	cx := requireTool(t, "codex")
	s := newSandbox(t)

	// Control: `codex features list` really validates the configuration,
	// otherwise the checks below would pass vacuously.
	if out, err := s.run(t, cx, "-c", "model_provider=llmenu-compat-missing", "features", "list"); err == nil {
		t.Fatalf("codex accepted an unknown model_provider; features list no longer validates the config:\n%s", out)
	}

	t.Run("default model", func(t *testing.T) {
		// Without a terminal llmenu uses codex.default and passes the model with
		// context window and max output as -c overrides.
		s.mustRun(t, llmenuBin, "config", "set", "codex.default", codexModel)
		s.mustRun(t, llmenuBin, "codex", "features", "list")
	})
	t.Run("-m passed through", func(t *testing.T) {
		s.mustRun(t, llmenuBin, "codex", "-m", codexModel, "features", "list")
	})
}
