package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/zalando/go-keyring"
)

// TestMain registers llmenu and fake tools as commands for the scripts in
// testdata/script. testscript runs them by re-executing the test binary, so
// this works on every OS without shell scripts.
func TestMain(m *testing.M) {
	// Child process for TestRunChildExitCode.
	if code := os.Getenv("LLMENU_TEST_EXIT"); code != "" {
		n, _ := strconv.Atoi(code)
		os.Exit(n)
	}
	// In-process tests never reach the real keychain either.
	keyring.MockInit()
	testscript.Main(m, map[string]func(){
		"llmenu": func() {
			keyring.MockInit() // never touch the real keyring from tests
			os.Exit(Execute())
		},
		"claude":   fakeTool("claude"),
		"codex":    fakeTool("codex"),
		"opencode": fakeTool("opencode"),
	})
}

// fakeTool prints its arguments and the environment llmenu sets for it, and
// exits with $FAKE_EXIT.
func fakeTool(name string) func() {
	return func() {
		fmt.Printf("%s args: %s\n", name, strings.Join(os.Args[1:], " "))
		var env []string
		for _, kv := range os.Environ() {
			k, _, _ := strings.Cut(kv, "=")
			if strings.HasPrefix(k, "ANTHROPIC_") || strings.HasPrefix(k, "CLAUDE_CODE_DISABLE_") ||
				k == "DISABLE_AUTO_COMPACTION" || k == "LLMENU_API_KEY" || k == "LLMENU_PROJECT" || k == "FOO" {
				env = append(env, kv)
			}
		}
		slices.Sort(env)
		for _, kv := range env {
			fmt.Println(kv)
		}
		code, _ := strconv.Atoi(os.Getenv("FAKE_EXIT"))
		os.Exit(code)
	}
}

func TestScripts(t *testing.T) {
	modelInfo, err := os.ReadFile(filepath.Join("..", "gateway", "testdata", "model_info.json"))
	if err != nil {
		t.Fatal(err)
	}
	testscript.Run(t, testscript.Params{
		Dir: filepath.Join("testdata", "script"),
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			// expandenv file… replaces $VAR in the files, e.g. the fake
			// gateway's URL in answers for the setup dialogs.
			"expandenv": func(ts *testscript.TestScript, neg bool, args []string) {
				if neg || len(args) == 0 {
					ts.Fatalf("usage: expandenv file…")
				}
				for _, f := range args {
					ts.Check(os.WriteFile(ts.MkAbs(f), []byte(os.Expand(ts.ReadFile(f), ts.Getenv)), 0o600))
				}
			},
		},
		Setup: func(env *testscript.Env) error {
			gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// sk-acme is a project's key (project.txtar).
				if auth := r.Header.Get("Authorization"); auth != "Bearer sk-test" && auth != "Bearer sk-acme" {
					http.Error(w, `{"error":"invalid key"}`, http.StatusUnauthorized)
					return
				}
				if r.URL.Path != "/model/info" {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write(modelInfo)
			}))
			aa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-api-key") != "aa-test" {
					http.Error(w, `{"error":"invalid key"}`, http.StatusUnauthorized)
					return
				}
				_, _ = w.Write([]byte(aaResponse))
			}))
			env.Defer(gw.Close)
			env.Defer(aa.Close)

			home := filepath.Join(env.WorkDir, "home")
			// Only the registered commands on PATH, so installed tools on the
			// machine cannot leak into the results.
			bin, _, _ := strings.Cut(env.Getenv("PATH"), string(os.PathListSeparator))
			for k, v := range map[string]string{
				"PATH":                   bin,
				"HOME":                   home,
				"USERPROFILE":            home,
				"LLMENU_BASE_URL":        gw.URL + "/",
				"LLMENU_API_KEY":         "sk-test",
				"LLMENU_AA_URL":          aa.URL,
				"LLMENU_AA_API_KEY":      "",
				"LLMENU_CONFIG":          filepath.Join(env.WorkDir, "config.toml"),
				"LLMENU_CACHE_DIR":       filepath.Join(env.WorkDir, "cache"),
				"LLMENU_KEYRING_SERVICE": "llmenu-test",
				// testscript points TMPDIR at $WORK/.tmp; the path guard
				// (internal/guard) must see $WORK as inside the temp dir.
				"TMPDIR": os.TempDir(),
				"TMP":    os.TempDir(),
				"TEMP":   os.TempDir(),
			} {
				env.Setenv(k, v)
			}
			return os.MkdirAll(home, 0o700)
		},
	})
}

const aaResponse = `{"data": [
  {"slug": "claude-opus-5-5", "evaluations": {"artificial_analysis_intelligence_index": 57.6, "terminalbench_v4_0": 0.596}},
  {"slug": "claude-opus-5-5-xhigh", "evaluations": {"artificial_analysis_intelligence_index": 56, "artificial_analysis_coding_index": 80}},
  {"slug": "claude-sonnet-4-5", "evaluations": {"artificial_analysis_intelligence_index": 42}},
  {"slug": "gpt-6-sol", "evaluations": {"artificial_analysis_intelligence_index": 47.6, "terminalbench_v4_0": 0.439}},
  {"slug": "qwen3-next", "evaluations": {"artificial_analysis_intelligence_index": 30}}
]}`
