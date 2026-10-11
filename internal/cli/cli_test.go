package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/config"
	"github.com/schowave/llmenu/internal/menu"
	"github.com/schowave/llmenu/internal/state"
	"github.com/schowave/llmenu/internal/targets"
)

// gatewayModels mirrors the shapes of a real gateway: an alias, a provider
// duplicate, a non-Claude model and a variant without thinking.
var gatewayModels = []catalog.Model{
	{Name: "aws/claude-opus-latest", Target: "bedrock/eu.anthropic.claude-opus-5-5", ContextTokens: 1_000_000, OutputPrice: 22},
	{Name: "aws/claude-opus-5-5", Target: "bedrock/eu.anthropic.claude-opus-5-5", ContextTokens: 1_000_000, OutputPrice: 22},
	{Name: "aws/claude-sonnet-4-5", Target: "bedrock/eu.anthropic.claude-sonnet-4-5-20250929-v1:0", ContextTokens: 200_000, OutputPrice: 16.5},
	{Name: "gcp/claude-sonnet-4-5", Target: "vertex_ai/claude-sonnet-4-5@20250929", ContextTokens: 200_000, OutputPrice: 15},
	{Name: "azure/gpt-6-sol", Target: "azure/gpt-6-sol", ContextTokens: 922_000, OutputPrice: 10},
}

// stubPicker replaces the menu and records what it was shown.
type stubPicker struct {
	choose  func(models []catalog.Model) (int, error)
	shown   []catalog.Model
	options menu.Options
}

func (p *stubPicker) install(t *testing.T) {
	t.Helper()
	orig := pickModel
	pickModel = func(models []catalog.Model, opt menu.Options) (int, error) {
		p.shown, p.options = models, opt
		return p.choose(models)
	}
	t.Cleanup(func() { pickModel = orig })
}

func names(models []catalog.Model) []string {
	var out []string
	for _, m := range models {
		out = append(out, m.Name)
	}
	return out
}

func TestChooseModelInteractive(t *testing.T) {
	t.Setenv("LLMENU_CACHE_DIR", t.TempDir())
	cfg := &config.Config{}
	p := &stubPicker{choose: func(models []catalog.Model) (int, error) {
		return slices.IndexFunc(models, func(m catalog.Model) bool { return m.Name == "azure/gpt-6-sol" }), nil
	}}
	p.install(t)

	hint := func(m catalog.Model) (bool, string) {
		if strings.Contains(m.Name, "gpt") {
			return true, "not Claude"
		}
		return true, ""
	}
	got, err := chooseModel(gatewayModels, choice{
		config: cfg, key: "claude", defaultName: "aws/claude-opus-latest", supports: hint, interactive: true,
	})
	if err != nil || got == nil || got.Name != "azure/gpt-6-sol" {
		t.Fatalf("chooseModel = %v, %v", got, err)
	}

	// Aliases and duplicates are hidden, most expensive first.
	if want := []string{"aws/claude-opus-5-5", "gcp/claude-sonnet-4-5", "azure/gpt-6-sol"}; !slices.Equal(names(p.shown), want) {
		t.Errorf("shown %v, want %v", names(p.shown), want)
	}
	// The cursor starts on the default, resolved from the alias.
	if p.options.Preselect != 0 {
		t.Errorf("preselect %d, want 0 (opus)", p.options.Preselect)
	}
	if p.options.Hint(p.shown[2]) != "not Claude" || p.options.Hint(p.shown[0]) != "" {
		t.Error("hints not passed to the menu")
	}
	if !strings.Contains(p.options.Note, "Benchmarks: none") {
		t.Errorf("note %q", p.options.Note)
	}
	if st := state.Load(); st.LastModel["claude"] != "azure/gpt-6-sol" {
		t.Errorf("last choice not saved: %v", st.LastModel)
	}

	// Without a default the cursor starts on the last choice.
	if _, err := chooseModel(gatewayModels, choice{config: cfg, key: "claude", interactive: true}); err != nil {
		t.Fatal(err)
	}
	if p.shown[p.options.Preselect].Name != "azure/gpt-6-sol" {
		t.Errorf("preselect %d, want last choice", p.options.Preselect)
	}
}

func TestChooseModelUnsupportedHidden(t *testing.T) {
	t.Setenv("LLMENU_CACHE_DIR", t.TempDir())
	p := &stubPicker{choose: func([]catalog.Model) (int, error) { return 0, nil }}
	p.install(t)
	onlyClaude := func(m catalog.Model) (bool, string) { return targets.IsClaude(m), "" }
	if _, err := chooseModel(gatewayModels, choice{config: &config.Config{}, supports: onlyClaude, interactive: true}); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(names(p.shown), "azure/gpt-6-sol") {
		t.Error("unsupported model shown")
	}
}

func TestChooseModelAbort(t *testing.T) {
	t.Setenv("LLMENU_CACHE_DIR", t.TempDir())
	p := &stubPicker{choose: func([]catalog.Model) (int, error) { return -1, menu.ErrAborted }}
	p.install(t)
	_, err := chooseModel(gatewayModels, choice{config: &config.Config{}, key: "claude", interactive: true})
	var ec exitCodeError
	if !errors.As(err, &ec) || ec.code != 130 {
		t.Fatalf("err = %v, want exit code 130", err)
	}
	if st := state.Load(); len(st.LastModel) != 0 {
		t.Error("aborted choice saved")
	}

	p.choose = func([]catalog.Model) (int, error) { return -1, errors.New("terminal broken") }
	if _, err := chooseModel(gatewayModels, choice{config: &config.Config{}, interactive: true}); err == nil || errors.As(err, &ec) {
		t.Errorf("menu error not returned as is: %v", err)
	}
}

func TestChooseModelNonInteractive(t *testing.T) {
	p := &stubPicker{choose: func([]catalog.Model) (int, error) {
		t.Fatal("menu opened without a terminal")
		return 0, nil
	}}
	p.install(t)

	got, err := chooseModel(gatewayModels, choice{config: &config.Config{}, defaultName: "aws/claude-opus-latest"})
	if err != nil || got == nil || got.Name != "aws/claude-opus-5-5" {
		t.Errorf("default = %v, %v; want aws/claude-opus-5-5", got, err)
	}
	// No default, or one the gateway does not know: the tool decides.
	for _, def := range []string{"", "gone/model"} {
		if got, err := chooseModel(gatewayModels, choice{config: &config.Config{}, defaultName: def}); got != nil || err != nil {
			t.Errorf("default %q: %v, %v; want nil", def, got, err)
		}
	}
}

func TestLoadEnvAsksForSetup(t *testing.T) {
	t.Setenv("LLMENU_CONFIG", t.TempDir()+"/config.toml")
	t.Setenv("LLMENU_BASE_URL", "")
	// The key comes from the environment, so the keyring is never touched;
	// only the gateway URL is missing.
	t.Setenv("LLMENU_API_KEY", "sk-test")

	origTerm, origConfirm := isTerminal, confirmSetup
	t.Cleanup(func() { isTerminal, confirmSetup = origTerm, origConfirm })
	isTerminal = func(*os.File) bool { return true }

	asked := false
	confirmSetup = func() (bool, error) { asked = true; return false, nil }
	if _, err := loadEnv(context.Background()); !errors.Is(err, errNotConfigured) || !asked {
		t.Errorf("declined setup: err = %v, asked = %v", err, asked)
	}

	confirmSetup = func() (bool, error) { return false, errors.New("ctrl-c") }
	if _, err := loadEnv(context.Background()); !errors.Is(err, errNotConfigured) {
		t.Errorf("aborted prompt: err = %v", err)
	}

	isTerminal = func(*os.File) bool { return false }
	confirmSetup = func() (bool, error) { t.Fatal("asked without a terminal"); return false, nil }
	if _, err := loadEnv(context.Background()); !errors.Is(err, errNotConfigured) {
		t.Errorf("no terminal: err = %v", err)
	}
}

func TestLaunchToolMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	tg, _ := targets.Get("claude")
	err := launch(context.Background(), tg.(targets.Launcher), nil)
	if err == nil || !strings.Contains(err.Error(), "claude not found on PATH") {
		t.Errorf("err = %v", err)
	}
}

func TestRunChildExitCode(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$") //nolint:gosec // G702: re-runs the test binary as the child
	cmd.Env = append(os.Environ(), "LLMENU_TEST_EXIT=7")
	var ec exitCodeError
	if err := runChild(cmd); !errors.As(err, &ec) || ec.code != 7 {
		t.Errorf("err = %v, want exit code 7", err)
	}

	cmd = exec.Command(os.Args[0], "-test.run=^$") //nolint:gosec // G702: re-runs the test binary as the child
	cmd.Env = append(os.Environ(), "LLMENU_TEST_EXIT=0")
	if err := runChild(cmd); err != nil {
		t.Errorf("exit 0: %v", err)
	}

	if err := runChild(exec.Command(t.TempDir() + "/does-not-exist")); err == nil || errors.As(err, &ec) {
		t.Errorf("start failure: %v", err)
	}
}

func TestExecuteExitCodes(t *testing.T) {
	t.Setenv("LLMENU_CONFIG", t.TempDir()+"/config.toml")
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"--version"}, 0},
		{[]string{"config", "get", "claude.default"}, 0},
		{[]string{"config", "get", "nope"}, 1},
		{[]string{"no-such-command"}, 1},
	} {
		os.Args = append([]string{"llmenu"}, tc.args...)
		if got := Execute(); got != tc.want {
			t.Errorf("llmenu %v = %d, want %d", tc.args, got, tc.want)
		}
	}
}

func TestApplyGatewayName(t *testing.T) {
	cfg := &config.Config{Gateway: config.Gateway{URL: "https://api.mygateway.example.com/", Name: "old"}}
	applyGatewayName(cfg, "mygateway")
	if cfg.Gateway.Name != "" {
		t.Errorf("derived name stored: %q", cfg.Gateway.Name)
	}
	applyGatewayName(cfg, "team-gw")
	if cfg.Gateway.Name != "team-gw" {
		t.Errorf("custom name not stored: %q", cfg.Gateway.Name)
	}
}

func TestApplyClaude(t *testing.T) {
	c := &config.Claude{}
	slots := map[string]string{
		"opus":   "aws/claude-opus-latest", // the suggestion: stays unset
		"sonnet": "aws/claude-sonnet-4-5",  // differs from the suggestion: stored
		"haiku":  "azure/gpt-6-sol",        // not Claude: stored with a warning
	}
	warnings := applyClaude(c, gatewayModels, slots, "opus", true)
	if c.Opus != "" || c.Sonnet != "aws/claude-sonnet-4-5" || c.Haiku != "azure/gpt-6-sol" {
		t.Errorf("slots %+v", c)
	}
	if c.Default != "" || c.AutoCompact != nil {
		t.Errorf("defaults pinned: default=%q autoCompact=%v", c.Default, c.AutoCompact)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "azure/gpt-6-sol") || !strings.Contains(warnings[0], "haiku") {
		t.Errorf("warnings %v", warnings)
	}

	// The warning also applies to the first model of the list.
	first := append([]catalog.Model{gatewayModels[4]}, gatewayModels[:4]...)
	if w := applyClaude(&config.Claude{}, first, map[string]string{"opus": "azure/gpt-6-sol"}, "opus", true); len(w) != 1 {
		t.Errorf("warnings for the first model: %v", w)
	}

	// A slot that was set before keeps being stored, even if it now equals the
	// suggestion; non-default answers are stored.
	c = &config.Claude{Opus: "aws/claude-opus-5-5"}
	applyClaude(c, gatewayModels, map[string]string{"opus": "aws/claude-opus-latest"}, "sonnet", false)
	if c.Opus != "aws/claude-opus-latest" || c.Default != "sonnet" || c.AutoCompact == nil || *c.AutoCompact {
		t.Errorf("explicit answers lost: %+v", c)
	}
}

func TestSyncPreselection(t *testing.T) {
	installed := func(t targets.Target) bool { return t.Name() == "pi" }
	if got := syncPreselection(&config.Config{}, installed); !slices.Equal(got, []string{"pi"}) {
		t.Errorf("unconfigured: %v, want installed targets", got)
	}
	cfg := &config.Config{}
	cfg.Sync.Set([]string{"opencode"})
	if got := syncPreselection(cfg, installed); !slices.Equal(got, []string{"opencode"}) {
		t.Errorf("configured: %v", got)
	}
	// Deliberately none stays none instead of falling back to installed tools.
	cfg.Sync.Set([]string{})
	if got := syncPreselection(cfg, installed); len(got) != 0 {
		t.Errorf("configured as none: %v", got)
	}
	if got := targetNames(syncTargets()); !slices.Equal(got, []string{"opencode", "pi"}) {
		t.Errorf("sync targets %v", got)
	}
}

func targetNames(ts []targets.Target) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Name())
	}
	return out
}

func TestSlotOptions(t *testing.T) {
	opts := slotOptions(gatewayModels, "opus", "gone/opus-old")
	var labels []string
	for _, o := range opts {
		labels = append(labels, o.Key)
	}
	if !strings.Contains(labels[0], "gone/opus-old") || !strings.Contains(labels[0], "no longer") {
		t.Errorf("unknown current value not kept first: %q", labels[0])
	}
	if !strings.Contains(labels[1], "aws/claude-opus-latest -> claude-opus-5-5") || !strings.Contains(labels[1], "follows new versions") {
		t.Errorf("latest alias not second: %q", labels[1])
	}
	if !slices.ContainsFunc(labels, func(l string) bool {
		return strings.Contains(l, "gpt-6-sol") && strings.Contains(l, "not a Claude model")
	}) {
		t.Errorf("non-Claude model not marked: %v", labels)
	}
	if len(slotOptions(gatewayModels, "opus", "aws/claude-opus-5-5")) != len(gatewayModels) {
		t.Error("known current value added twice")
	}

	// No haiku model in the gateway: "not set" comes first so Enter keeps it unset.
	haiku := slotOptions(gatewayModels, "haiku", "")
	if haiku[0].Value != "" || !strings.Contains(haiku[0].Key, "not set") {
		t.Errorf("first haiku option %+v, want \"not set\"", haiku[0])
	}
	if len(haiku) != len(gatewayModels)+1 {
		t.Errorf("%d haiku options, want all models plus \"not set\"", len(haiku))
	}
	if o := slotOptions(gatewayModels, "haiku", "azure/gpt-6-sol"); o[0].Value == "" {
		t.Error("\"not set\" offered although a model is set")
	}
	c := &config.Claude{}
	applyClaude(c, gatewayModels, map[string]string{"opus": "aws/claude-opus-latest", "sonnet": "gcp/claude-sonnet-4-5", "haiku": ""}, "opus", true)
	if c.Haiku != "" {
		t.Errorf("unset haiku stored as %q", c.Haiku)
	}
}

func TestValidateURL(t *testing.T) {
	for in, ok := range map[string]bool{
		"https://llm.example.com/": true, "http://localhost:4000": true,
		"ftp://x": false, "llm.example.com": false, "https://": false, "": false,
	} {
		if err := validateURL(in); (err == nil) != ok {
			t.Errorf("validateURL(%q) = %v", in, err)
		}
	}
}

func TestBuildVersion(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })

	version = "1.2.3"
	if got := buildVersion(); got != "1.2.3" {
		t.Errorf("goreleaser version: %q", got)
	}
	// Test binaries carry no module version ("(devel)" or empty), so "dev" stays.
	version = "dev"
	if got := buildVersion(); got != "dev" {
		t.Errorf("dev build: %q", got)
	}
}
