package targets

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/config"
)

func TestRegistry(t *testing.T) {
	var names []string
	for _, tg := range All() {
		names = append(names, tg.Name())
	}
	want := []string{"claude", "codex", "opencode", "pi"}
	if !slices.Equal(names, want) {
		t.Fatalf("targets = %v, want %v", names, want)
	}

	caps := map[string][2]bool{ // Sync, Launch
		"claude":   {false, true},
		"codex":    {false, true},
		"opencode": {true, true},
		"pi":       {true, false},
	}
	for name, c := range caps {
		tg, _ := Get(name)
		_, s := tg.(Syncer)
		_, l := tg.(Launcher)
		if s != c[0] || l != c[1] {
			t.Errorf("%s: Sync=%v Launch=%v, want Sync=%v Launch=%v", name, s, l, c[0], c[1])
		}
	}
}

func TestHasModelArg(t *testing.T) {
	tests := []struct {
		target string
		args   []string
		want   bool
	}{
		{"claude", nil, false},
		{"claude", []string{"-c"}, false},
		{"claude", []string{"--model", "sonnet"}, true},
		{"claude", []string{"--model=sonnet", "-p", "hi"}, true},
		{"claude", []string{"--", "--model"}, false},
		{"claude", []string{"--models"}, false},
		{"codex", []string{"-v"}, false}, // as long as -m, but a different flag
		{"codex", []string{"-m", "gpt-5"}, true},
		{"codex", []string{"--model=gpt-5"}, true},
		{"codex", []string{"exec", "hello"}, false},
	}
	for _, tt := range tests {
		tg, _ := Get(tt.target)
		if got := tg.(Launcher).HasModelArg(tt.args); got != tt.want {
			t.Errorf("%s.HasModelArg(%q) = %v, want %v", tt.target, tt.args, got, tt.want)
		}
	}
}

func TestClaudeCommand(t *testing.T) {
	tg, _ := Get("claude")
	l := tg.(Launcher)
	models := []catalog.Model{
		{Name: "gcp/claude-sonnet-4-6", Target: "vertex_ai/claude-sonnet-4-6", ContextTokens: 200_000},
		{Name: "aws/claude-opus-4-8", Target: "bedrock/eu.anthropic.claude-opus-4-8", ContextTokens: 1_000_000},
		{Name: "aws/claude-opus-latest", Target: "bedrock/eu.anthropic.claude-opus-4-8", ContextTokens: 1_000_000},
		{Name: "aws/claude-sonnet-latest", Target: "bedrock/eu.anthropic.claude-sonnet-4-6", ContextTokens: 1_000_000},
		{Name: "edge", Target: "edge", ContextTokens: claude1MThreshold},
	}
	off := false // the picker has its own test
	env := Env{BaseURL: "https://gateway.example/", APIKey: "secret", Config: &config.Config{Claude: config.Claude{Picker: &off}}, Models: models}

	tests := []struct {
		name  string
		model *catalog.Model
		args  []string
		want  []string
	}{
		{"without model", nil, []string{"-c"}, []string{"claude", "-c"}},
		{"200k", &models[0], nil, []string{"claude", "--model", "gcp/claude-sonnet-4-6"}},
		{"1M", &models[1], []string{"-r"}, []string{"claude", "--model", "aws/claude-opus-4-8[1m]", "-r"}},
		{"at the 1M threshold", &catalog.Model{Name: "edge"}, nil, []string{"claude", "--model", "edge[1m]"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := l.Command(context.Background(), tt.model, tt.args, env)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(cmd.Args, tt.want) {
				t.Errorf("Args = %q, want %q", cmd.Args, tt.want)
			}
			for _, a := range cmd.Args {
				if strings.Contains(a, "secret") {
					t.Errorf("key appears in the arguments: %q", cmd.Args)
				}
			}
		})
	}
}

func TestClaudeSettings(t *testing.T) {
	tg, _ := Get("claude")
	l := tg.(Launcher)
	intel := 70.0
	models := []catalog.Model{
		{Name: "gcp/claude-sonnet-4-6", Target: "vertex_ai/claude-sonnet-4-6", ContextTokens: 200_000, InputPrice: 3, OutputPrice: 15,
			Tiers: []catalog.Tier{{AboveTokens: 200_000, InputPrice: 6, OutputPrice: 22.5}}},
		{Name: "aws/claude-opus-4-8", Target: "bedrock/eu.anthropic.claude-opus-4-8", ContextTokens: 1_000_000, InputPrice: 5, OutputPrice: 25,
			Bench: &catalog.Bench{Intelligence: &intel}},
		{Name: "aws/claude-opus-latest", Target: "bedrock/eu.anthropic.claude-opus-4-8", ContextTokens: 1_000_000, InputPrice: 5, OutputPrice: 25},
		{Name: "azure/gpt-6-luna", Target: "azure/gpt-6-luna", ContextTokens: 922_000, NoPrice: true},
		{Name: "local/qwen", Target: "hosted_vllm/qwen", ContextTokens: 128_000},
		{Name: "gcp/claude-3-5-sonnet", Target: "vertex_ai/claude-3-5-sonnet-v2@20241022", InputPrice: 3, OutputPrice: 15},
	}
	t.Setenv("LLMENU_CACHE_DIR", t.TempDir())
	type option struct{ Model, Label, Description, BehavesAs string }
	type settings struct {
		StatusLine  *struct{ Command string }
		ModelPicker *struct {
			ReplaceBuiltInOptions bool
			Options               []option
		}
	}
	parse := func(t *testing.T, cfg config.Claude, args ...string) (*settings, []string) {
		t.Helper()
		env := Env{Config: &config.Config{Claude: cfg, Bench: config.Bench{Metric: "unknown"}}, Models: models}
		cmd, err := l.Command(context.Background(), nil, args, env)
		if err != nil {
			t.Fatal(err)
		}
		i := slices.Index(cmd.Args, "--settings")
		if i < 0 {
			return nil, cmd.Args
		}
		// A file, not inline: the list could exceed Windows' command line limit.
		b, err := os.ReadFile(cmd.Args[i+1])
		if err != nil {
			return nil, cmd.Args // not llmenu's
		}
		var s settings
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatal(err)
		}
		return &s, cmd.Args
	}

	t.Run("picker by default", func(t *testing.T) {
		s, args := parse(t, config.Claude{})
		if s == nil || s.ModelPicker == nil {
			t.Fatalf("no model picker: %q", args)
		}
		if s.StatusLine != nil {
			t.Errorf("status line without claude.statusline")
		}
		if !s.ModelPicker.ReplaceBuiltInOptions {
			t.Errorf("built-in options are kept")
		}
		// Benchmark first (an unknown metric is intelligence, as in
		// Model.Score), then most expensive; no alias. behavesAs only where the
		// shortened target is a valid Anthropic ID: claude-3-5-sonnet needs its
		// date. Unknown context is left out.
		want := []option{
			{"aws/claude-opus-4-8[1m]", "aws/claude-opus-4-8", "1M · $5.00 in / $25.00 out · intelligence 70.0", "claude-opus-4-8"},
			{"gcp/claude-3-5-sonnet", "gcp/claude-3-5-sonnet", "$3.00 in / $15.00 out", ""},
			{"gcp/claude-sonnet-4-6", "gcp/claude-sonnet-4-6", "200k · $3.00 in / $15.00 out, more above 200k", "claude-sonnet-4-6"},
			{"azure/gpt-6-luna[1m]", "azure/gpt-6-luna", "922k · price unknown", ""},
			{"local/qwen", "local/qwen", "128k · free", ""},
		}
		if !slices.Equal(s.ModelPicker.Options, want) {
			t.Errorf("options =\n%q\nwant\n%q", s.ModelPicker.Options, want)
		}
	})
	t.Run("with status line", func(t *testing.T) {
		s, args := parse(t, config.Claude{StatusLine: true})
		if s == nil || s.StatusLine == nil || s.StatusLine.Command != "llmenu statusline" || s.ModelPicker == nil {
			t.Errorf("want status line and picker in one --settings: %q", args)
		}
	})
	t.Run("picker off", func(t *testing.T) {
		off := false
		if _, args := parse(t, config.Claude{Picker: &off}); slices.Contains(args, "--settings") {
			t.Errorf("--settings without picker and status line: %q", args)
		}
		s, _ := parse(t, config.Claude{Picker: &off, StatusLine: true})
		if s == nil || s.ModelPicker != nil || s.StatusLine == nil {
			t.Errorf("want only the status line: %+v", s)
		}
	})
	t.Run("own settings win", func(t *testing.T) {
		_, args := parse(t, config.Claude{StatusLine: true}, "--settings", "my.json")
		if want := []string{"claude", "--settings", "my.json"}; !slices.Equal(args, want) {
			t.Errorf("Args = %q, want %q", args, want)
		}
	})
}

func TestClaudeEnv(t *testing.T) {
	tg, _ := Get("claude")
	l := tg.(Launcher)
	models := []catalog.Model{
		{Name: "aws/claude-opus-latest", Target: "bedrock/eu.anthropic.claude-opus-5-5", ContextTokens: 1_000_000},
		{Name: "aws/claude-sonnet-5-5", Target: "bedrock/eu.anthropic.claude-sonnet-5-5", ContextTokens: 1_000_000},
		{Name: "azure/gpt-6-luna", Target: "azure/gpt-6-luna", ContextTokens: 922_000},
	}
	off := false
	tests := []struct {
		name   string
		cfg    config.Claude
		want   []string
		absent []string
	}{
		{
			name: "gateway suggestions",
			want: []string{
				"ANTHROPIC_DEFAULT_OPUS_MODEL=aws/claude-opus-latest[1m]",
				"ANTHROPIC_DEFAULT_SONNET_MODEL=aws/claude-sonnet-5-5[1m]",
				"ANTHROPIC_MODEL=opus",
				"CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1",
				"ANTHROPIC_BASE_URL=https://gateway.example/",
				"ANTHROPIC_AUTH_TOKEN=secret",
			},
			absent: []string{"ANTHROPIC_DEFAULT_HAIKU_MODEL=", "DISABLE_AUTO_COMPACTION="},
		},
		{
			name: "configured",
			cfg: config.Claude{Default: "sonnet", Haiku: "azure/gpt-6-luna", AutoCompact: &off,
				Env: map[string]string{"FOO": "bar"}},
			want: []string{
				"ANTHROPIC_DEFAULT_HAIKU_MODEL=azure/gpt-6-luna[1m]",
				"ANTHROPIC_MODEL=sonnet",
				"DISABLE_AUTO_COMPACTION=1",
				"FOO=bar",
			},
			absent: []string{"CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS="},
		},
		{
			name: "default as model name",
			cfg:  config.Claude{Default: "aws/claude-sonnet-5-5"},
			want: []string{"ANTHROPIC_MODEL=aws/claude-sonnet-5-5[1m]"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := Env{BaseURL: "https://gateway.example/", APIKey: "secret",
				Config: &config.Config{Claude: tt.cfg}, Models: models}
			cmd, err := l.Command(context.Background(), nil, nil, env)
			if err != nil {
				t.Fatal(err)
			}
			own := cmd.Env[len(os.Environ()):]
			for _, w := range tt.want {
				if !slices.Contains(own, w) {
					t.Errorf("missing: %s\nEnv: %q", w, own)
				}
			}
			for _, a := range tt.absent {
				if slices.ContainsFunc(own, func(kv string) bool { return strings.HasPrefix(kv, a) }) {
					t.Errorf("should be absent: %s\nEnv: %q", a, own)
				}
			}
		})
	}
}

func TestClaudeDefaultModel(t *testing.T) {
	tg, _ := Get("claude")
	d := tg.(Defaulter)
	models := []catalog.Model{{Name: "aws/claude-opus-latest", Target: "bedrock/eu.anthropic.claude-opus-5-5"}}
	tests := []struct {
		cfg  config.Claude
		want string
	}{
		{config.Claude{}, "aws/claude-opus-latest"},
		{config.Claude{Opus: "x/opus-fixed"}, "x/opus-fixed"},
		{config.Claude{Default: "haiku"}, ""},
		{config.Claude{Default: "x/model"}, "x/model"},
	}
	for _, tt := range tests {
		env := Env{Config: &config.Config{Claude: tt.cfg}, Models: models}
		if got := d.DefaultModel(env); got != tt.want {
			t.Errorf("DefaultModel(%+v) = %q, want %q", tt.cfg, got, tt.want)
		}
	}
}

func TestCodexCommand(t *testing.T) {
	tg, _ := Get("codex")
	l := tg.(Launcher)
	cfg := &config.Config{Gateway: config.Gateway{URL: "https://gw.example.com/"}}
	env := Env{BaseURL: "https://gw.example.com/", APIKey: "secret", Config: cfg}
	m := &catalog.Model{Name: "azure/gpt-6-luna", ContextTokens: 922_000, MaxOutputTokens: 128_000}

	cmd, err := l.Command(context.Background(), m, []string{"exec", "hello"}, env)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"codex",
		"-c", "model_provider=gw",
		"-c", `model_providers.gw={name="gw", base_url="https://gw.example.com/v1", env_key="LLMENU_API_KEY", wire_api="responses"}`,
		"-c", `model="azure/gpt-6-luna"`,
		"-c", "model_context_window=922000",
		"-c", "model_max_output_tokens=128000",
		"exec", "hello"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args =\n%q\nwant\n%q", cmd.Args, want)
	}
	if !slices.Contains(cmd.Env, "LLMENU_API_KEY=secret") || slices.ContainsFunc(cmd.Args, func(a string) bool { return strings.Contains(a, "secret") }) {
		t.Error("key not only in the environment")
	}

	// Unknown limits (0) are left to codex.
	cmd, _ = l.Command(context.Background(), &catalog.Model{Name: "x"}, nil, env)
	if slices.ContainsFunc(cmd.Args, func(a string) bool {
		return strings.HasPrefix(a, "model_context_window") || strings.HasPrefix(a, "model_max_output")
	}) {
		t.Errorf("unknown limits passed: %q", cmd.Args)
	}

	// A provider name with a dot would be a nested TOML key.
	cfg.Gateway.Name = "my.gw"
	cmd, _ = l.Command(context.Background(), nil, []string{"-m", "x"}, env)
	if cmd.Args[2] != "model_provider=my_gw" {
		t.Errorf("provider ID not sanitized: %q", cmd.Args[2])
	}

	if _, err := l.Command(context.Background(), nil, nil, env); err == nil {
		t.Error("started without model")
	}

	if ok, hint := l.Supports(catalog.Model{Endpoints: []string{"/v1/responses"}}); !ok || hint != "" {
		t.Error("Responses model flagged")
	}
	if _, hint := l.Supports(catalog.Model{}); hint == "" {
		t.Error("model without Responses API not flagged")
	}
}

func TestOpencodeCommand(t *testing.T) {
	tg, _ := Get("opencode")
	cmd, err := tg.(Launcher).Command(context.Background(), nil, []string{"-m", "x"}, Env{APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cmd.Args, []string{"opencode", "-m", "x"}) || !slices.Contains(cmd.Env, "LLMENU_API_KEY=secret") {
		t.Errorf("Args = %q, key in environment: %v", cmd.Args, slices.Contains(cmd.Env, "LLMENU_API_KEY=secret"))
	}
}

func TestInstalled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executables need an extension")
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if _, ok := Installed(pi{}); ok {
		t.Error("pi found in an empty PATH")
	}
	bin := filepath.Join(dir, "pi")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o700); err != nil { //nolint:gosec // G306: must be executable
		t.Fatal(err)
	}
	if p, ok := Installed(pi{}); !ok || p != bin {
		t.Errorf("Installed = %q, %v; want %q, true", p, ok, bin)
	}
}

func TestConfigPathDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows
	tests := []struct {
		name   string
		target interface{ ConfigPath() (string, error) }
		create string // file created beforehand
		want   string
	}{
		{"pi", pi{}, "", filepath.Join(home, ".pi", "agent", "models.json")},
		{"opencode", opencode{}, "", filepath.Join(home, ".config", "opencode", "opencode.json")},
		{"opencode prefers jsonc", opencode{}, filepath.Join(home, ".config", "opencode", "opencode.jsonc"),
			filepath.Join(home, ".config", "opencode", "opencode.jsonc")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.create != "" {
				if err := os.MkdirAll(filepath.Dir(tt.create), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(tt.create, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if got, err := tt.target.ConfigPath(); err != nil || got != tt.want {
				t.Errorf("ConfigPath() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}

	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		return
	}
	t.Setenv("HOME", "")
	for _, tg := range []interface{ ConfigPath() (string, error) }{pi{}, opencode{}} {
		if p, err := tg.ConfigPath(); err == nil {
			t.Errorf("%T: ConfigPath() without HOME = %q, want an error", tg, p)
		}
	}
}
