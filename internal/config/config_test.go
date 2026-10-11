package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNameFromURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://api.mygateway.example.com/", "mygateway"},
		{"https://llm.example.com", "example"},
		{"http://localhost:4000", "localhost"},
		{"https://gateway.ai.corp.example.org/v1", "corp"},
		{"broken", "gateway"},
	}
	for _, tt := range tests {
		if got := NameFromURL(tt.in); got != tt.want {
			t.Errorf("NameFromURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGetSet(t *testing.T) {
	c := &Config{}
	check := func(key, value, source string) {
		t.Helper()
		e, err := c.Get(key)
		if err != nil {
			t.Fatal(err)
		}
		if e.Value != value || e.Source != source {
			t.Errorf("%s = %q (%s), want %q (%s)", key, e.Value, e.Source, value, source)
		}
	}

	check("gateway.url", "", "")
	check("claude.default", "opus", "default")
	check("claude.auto_compact", "true", "default")
	check("claude.env.CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS", "1", "default")

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(c.Set("gateway.url", "https://api.example.com/"))
	check("gateway.name", "example", "default")
	must(c.Set("claude.default", "sonnet"))
	check("claude.default", "sonnet", "file")
	must(c.Set("claude.auto_compact", "false"))
	check("claude.auto_compact", "false", "file")
	if c.Claude.AutoCompactEnabled() {
		t.Error("auto_compact=false has no effect")
	}
	must(c.Set("sync.targets", "pi, opencode"))
	check("sync.targets", "pi,opencode", "file")

	// A custom variable takes over the defaults and adds to them.
	must(c.Set("claude.env.FOO", "bar"))
	check("claude.env.FOO", "bar", "file")
	check("claude.env.CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "1", "file")
	must(c.Set("claude.env.FOO", ""))
	check("claude.env.FOO", "", "")

	check("bench.metric", "intel", "default")
	must(c.Set("bench.metric", "tb4"))
	check("bench.metric", "tb4", "file")
	if err := c.Set("bench.metric", "speed"); err == nil {
		t.Error("unknown benchmark accepted")
	}
	must(c.Set("bench.map.mistral/mistral-large-2512", "mistral-large-3"))
	check("bench.map.mistral/mistral-large-2512", "mistral-large-3", "file")
	must(c.Set("bench.map.mistral/mistral-large-2512", ""))
	check("bench.map.mistral/mistral-large-2512", "", "")

	must(c.Set("claude.default", ""))
	check("claude.default", "opus", "default")

	if err := c.Set("claude.auto_compact", "maybe"); err == nil {
		t.Error("invalid boolean accepted")
	}
	if err := c.Set("does.not.exist", "x"); err == nil {
		t.Error("unknown key accepted")
	}
}

func TestSaveLoad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("LLMENU_CONFIG", p)

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	c.Gateway.URL = "https://api.example.com/"
	c.Claude.Opus = "anthropic/claude-opus-latest"
	if changed, err := c.Save(); err != nil || !changed {
		t.Fatalf("first save: changed=%v err=%v", changed, err)
	}
	if changed, err := c.Save(); err != nil || changed {
		t.Fatalf("saved although unchanged: changed=%v err=%v", changed, err)
	}

	b, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(b), "# Written by llmenu") {
		t.Errorf("header comment missing:\n%s", b)
	}
	if strings.Contains(string(b), "auto_compact") || strings.Contains(string(b), "[claude.env]") {
		t.Errorf("unset values written:\n%s", b)
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Gateway.URL != c.Gateway.URL || got.Claude.Opus != c.Claude.Opus {
		t.Errorf("loaded %+v, want %+v", got, c)
	}
}
