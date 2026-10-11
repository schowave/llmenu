package targets

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tailscale/hujson"

	"github.com/schowave/llmenu/internal/catalog"
)

var syncModels = []catalog.Model{
	{Name: "aws/claude-opus-5-5", Target: "bedrock/eu.anthropic.claude-opus-5-5", ContextTokens: 1_000_000,
		MaxOutputTokens: 128_000, Vision: true, Reasoning: true, InputPrice: 4.4, OutputPrice: 22, CacheReadPrice: 0.44, CacheWritePrice: 5.5},
	{Name: "local/Qwen3-Next-non-thinking", Target: "hosted_vllm/Qwen/Qwen3-Next-FP8", ContextTokens: 229_376,
		MaxOutputTokens: 32_768, Reasoning: true, NoThinking: true},
}

var syncOpt = SyncOptions{BaseURL: "https://llm.example.com/", Provider: "example"}

// readJSON reads a (HuJSON) file as a map.
func readJSON(t *testing.T, file string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	std, err := hujson.Standardize(b)
	if err != nil {
		t.Fatalf("invalid JSON in %s: %v\n%s", file, err, b)
	}
	var m map[string]any
	if err := json.Unmarshal(std, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func get(m any, path ...string) any {
	for _, p := range path {
		m = asMap(m)[p]
	}
	return m
}

func TestPiSync(t *testing.T) {
	file := filepath.Join(t.TempDir(), "models.json")
	writeFile(t, file, `{
  "providers": {
    "foreign": {"baseUrl": "https://other/", "models": []},
    "example": {
      "name": "My gateway",
      "baseUrl": "https://old/v1",
      "apiKey": "!old-command",
      "headers": {"X-Team": "a"},
      "models": [
        {"id": "aws/claude-opus-5-5", "name": "Opus (nice)", "compat": {"x": 1}},
        {"id": "no-longer-exists", "name": "gone"}
      ]
    }
  }
}
`)

	p := pi{path: file}
	d, err := p.Sync(context.Background(), syncModels, syncOpt)
	if err != nil || !d.Changed {
		t.Fatalf("Sync: %+v %v", d, err)
	}
	cfg := readJSON(t, file)
	prov := get(cfg, "providers", "example")

	if get(cfg, "providers", "foreign", "baseUrl") != "https://other/" {
		t.Error("foreign provider changed")
	}
	for key, want := range map[string]any{
		"name": "My gateway", "baseUrl": "https://llm.example.com/v1",
		"apiKey": "!llmenu secrets get gateway", "api": "openai-completions",
	} {
		if got := get(prov, key); got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	if get(prov, "headers", "X-Team") != "a" {
		t.Error("provider's custom field lost")
	}

	list := asMap(prov)["models"].([]any)
	if len(list) != 2 {
		t.Fatalf("%d models, want 2: %v", len(list), list)
	}
	opus, qwen := asMap(list[0]), asMap(list[1])
	if opus["name"] != "Opus (nice)" || get(opus, "compat", "x") != 1.0 {
		t.Errorf("model customizations lost: %v", opus)
	}
	if opus["contextWindow"] != 1e6 || get(opus, "cost", "cacheWrite") != 5.5 || opus["reasoning"] != true {
		t.Errorf("Opus wrong: %v", opus)
	}
	if in := opus["input"].([]any); len(in) != 2 || in[1] != "image" {
		t.Errorf("vision missing: %v", opus["input"])
	}
	if qwen["name"] != "local/Qwen3-Next-non-thinking" || qwen["reasoning"] != false {
		t.Errorf("Qwen wrong: %v", qwen)
	}

	if d, err := p.Sync(context.Background(), syncModels, syncOpt); err != nil || d.Changed {
		t.Errorf("second sync changes: %+v %v", d, err)
	}
}

func TestPiSyncUnknownLimits(t *testing.T) {
	file := filepath.Join(t.TempDir(), "models.json")
	writeFile(t, file, `{"providers": {"example": {"models": [{"id": "x", "contextWindow": 5, "maxTokens": 5}]}}}`)
	// The gateway no longer reports limits: stale values are removed.
	if _, err := (pi{path: file}).Sync(context.Background(), []catalog.Model{{Name: "x"}}, syncOpt); err != nil {
		t.Fatal(err)
	}
	e := asMap(asMap(get(readJSON(t, file), "providers", "example"))["models"].([]any)[0])
	if _, ok := e["contextWindow"]; ok {
		t.Errorf("contextWindow kept: %v", e)
	}
	if _, ok := e["maxTokens"]; ok {
		t.Errorf("maxTokens kept: %v", e)
	}
}

func TestOpencodeSync(t *testing.T) {
	file := filepath.Join(t.TempDir(), "opencode.jsonc")
	writeFile(t, file, `{
  "$schema": "https://opencode.ai/config.json",
  // please leave the theme alone
  "theme": "tokyonight",
  "provider": {
    "example": {
      "options": {"timeout": 600000},
      "models": {"aws/claude-opus-5-5": {"name": "Opus", "temperature": true}},
    },
  },
}
`)

	o := opencode{path: file}
	if _, err := o.Sync(context.Background(), syncModels, syncOpt); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(file)
	if !strings.Contains(string(b), "// please leave the theme alone") {
		t.Errorf("comment lost:\n%s", b)
	}
	cfg := readJSON(t, file)
	prov := get(cfg, "provider", "example")
	for _, c := range []struct {
		path []string
		want any
	}{
		{[]string{"npm"}, "@ai-sdk/openai-compatible"},
		{[]string{"options", "baseURL"}, "https://llm.example.com/v1"},
		{[]string{"options", "apiKey"}, "{env:LLMENU_API_KEY}"},
		{[]string{"options", "timeout"}, 600000.0},
		{[]string{"models", "aws/claude-opus-5-5", "name"}, "Opus"},
		{[]string{"models", "aws/claude-opus-5-5", "temperature"}, true},
		{[]string{"models", "aws/claude-opus-5-5", "limit", "context"}, 1e6},
		{[]string{"models", "aws/claude-opus-5-5", "cost", "cache_read"}, 0.44},
		{[]string{"models", "aws/claude-opus-5-5", "attachment"}, true},
		{[]string{"models", "local/Qwen3-Next-non-thinking", "reasoning"}, false},
	} {
		if got := get(prov, c.path...); got != c.want {
			t.Errorf("%v = %v, want %v", c.path, got, c.want)
		}
	}
	if get(cfg, "theme") != "tokyonight" {
		t.Error("theme changed")
	}
	if d, err := o.Sync(context.Background(), syncModels, syncOpt); err != nil || d.Changed {
		t.Errorf("second sync changes: %+v %v", d, err)
	}

	// Without a file, it is created with $schema.
	fresh := opencode{path: filepath.Join(t.TempDir(), "opencode.json")}
	if _, err := fresh.Sync(context.Background(), syncModels, syncOpt); err != nil {
		t.Fatal(err)
	}
	if get(readJSON(t, fresh.path), "$schema") != "https://opencode.ai/config.json" {
		t.Error("$schema missing in new file")
	}
}

func writeFile(t *testing.T, file, content string) {
	t.Helper()
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
