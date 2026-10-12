package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPathOverride(t *testing.T) {
	p := filepath.Join(t.TempDir(), "elsewhere.toml")
	t.Setenv("LLMENU_CONFIG", p)
	if got, err := Path(); err != nil || got != p {
		t.Errorf("Path() = %q, %v; want %q", got, err, p)
	}
	t.Setenv("LLMENU_CONFIG", "")
	if got, err := Path(); err == nil && !strings.HasSuffix(got, filepath.Join("llmenu", "config.toml")) {
		t.Errorf("Path() = %q", got)
	}
}

func TestLoadFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{"valid", "[gateway]\nurl = \"https://api.example.com/\"\n\n[bench.map]\n\"a/b\" = \"c\"\n", false},
		{"corrupt", "[gateway\nurl = ", true},
		{"wrong type", "[gateway]\nurl = 42\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.toml")
			t.Setenv("LLMENU_CONFIG", p)
			if err := os.WriteFile(p, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			c, err := Load()
			if tt.wantErr {
				if err == nil || c != nil || !strings.Contains(err.Error(), p) {
					t.Errorf("Load() = %+v, %v; want an error naming %s", c, err, p)
				}
				return
			}
			if err != nil || c.Gateway.URL != "https://api.example.com/" || c.Bench.Map["a/b"] != "c" {
				t.Errorf("Load() = %+v, %v", c, err)
			}
		})
	}
}

func TestLoadUnreadable(t *testing.T) {
	t.Setenv("LLMENU_CONFIG", t.TempDir()) // a directory, not a file
	if c, err := Load(); err == nil {
		t.Errorf("Load() = %+v, want an error", c)
	}
}

func TestList(t *testing.T) {
	keys := func(es []Entry) []string {
		var out []string
		for _, e := range es {
			out = append(out, e.Key)
		}
		return out
	}

	c := &Config{}
	got := c.List()
	if len(got) != len(Keys)+len(DefaultClaudeEnv) {
		t.Errorf("List() = %v", keys(got))
	}
	for i, k := range Keys {
		if got[i].Key != k.Name {
			t.Errorf("entry %d = %q, want %q", i, got[i].Key, k.Name)
		}
	}
	for _, e := range got[len(Keys):] {
		if !strings.HasPrefix(e.Key, envPrefix) || e.Value != "1" || e.Source != "default" {
			t.Errorf("default env entry %+v", e)
		}
	}

	for _, kv := range [][2]string{
		{"claude.env.ZZZ", "z"},
		{"claude.env.AAA", "a"},
		{"bench.map.x/m2", "s2"},
		{"bench.map.x/m1", "s1"},
	} {
		if err := c.Set(kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	tail := keys(c.List())[len(Keys):]
	want := []string{
		"claude.env.AAA", "claude.env.CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS",
		"claude.env.CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "claude.env.ZZZ",
		"bench.map.x/m1", "bench.map.x/m2",
	}
	if !slices.Equal(tail, want) {
		t.Errorf("List() ends with %v\nwant             %v", tail, want)
	}
	for _, e := range c.List() {
		if e.Key == "bench.map.x/m1" && (e.Value != "s1" || e.Source != "file") {
			t.Errorf("bench.map entry %+v", e)
		}
	}
}

func TestSetMissingName(t *testing.T) {
	c := &Config{}
	for _, k := range []string{"claude.env.", "bench.map."} {
		if err := c.Set(k, "x"); err == nil {
			t.Errorf("Set(%q) accepted", k)
		}
	}
}

func TestSaveRewritesOnChange(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("LLMENU_CONFIG", p)

	c := &Config{Gateway: Gateway{URL: "https://api.example.com/"}}
	if changed, err := c.Save(); err != nil || !changed {
		t.Fatalf("first save: changed=%v err=%v", changed, err)
	}

	c.Codex.Default = "azure/gpt-5.6-sol"
	if changed, err := c.Save(); err != nil || !changed {
		t.Fatalf("changed value not saved: changed=%v err=%v", changed, err)
	}
	b, err := os.ReadFile(p)
	if err != nil || !strings.Contains(string(b), "azure/gpt-5.6-sol") {
		t.Errorf("file misses the new value: %v\n%s", err, b)
	}

	// Same settings, but the file was edited by hand: rewritten.
	if err := os.WriteFile(p, append([]byte("# my comment\n"), b...), 0o600); err != nil { //nolint:gosec // G703: p is in t.TempDir
		t.Fatal(err)
	}
	if changed, err := c.Save(); err != nil || !changed {
		t.Fatalf("hand-edited file not rewritten: changed=%v err=%v", changed, err)
	}
	if es, _ := os.ReadDir(filepath.Dir(p)); len(es) != 1 {
		t.Errorf("temporary file left behind: %v", es)
	}
}

func TestSaveError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LLMENU_CONFIG", filepath.Join(file, "sub", "config.toml"))
	if changed, err := (&Config{}).Save(); err == nil || changed {
		t.Errorf("Save below a file: changed=%v err=%v", changed, err)
	}
}
