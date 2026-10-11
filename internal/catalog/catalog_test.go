package catalog

import (
	"slices"
	"testing"
)

func TestKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"bedrock/eu.anthropic.claude-sonnet-4-5-20250929-v1:0", "4 5 claude sonnet"},
		{"vertex_ai/claude-sonnet-4-5@20250929", "4 5 claude sonnet"},
		{"aws/claude-sonnet-4-5", "4 5 claude sonnet"},
		{"aws/claude-opus-latest", "claude opus"},
		{"hosted_vllm/Qwen/Qwen3.8-Flash-Next-FP8", "3 8 flash next qwen"},
		{"local/Qwen3.8-Flash-Next-non-thinking", "3 8 flash next qwen"},
		{"bedrock/zai.glm-5", "5 glm"},
		{"azure/gpt-5.6-sol", "5 6 gpt sol"},
	}
	for _, tt := range tests {
		if got := Key(tt.in, PickerStopwords); got != tt.want {
			t.Errorf("Key(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestShortTarget(t *testing.T) {
	tests := []struct{ in, want string }{
		{"bedrock/eu.anthropic.claude-opus-4-6-v1", "claude-opus-4-6"},
		{"bedrock/eu.anthropic.claude-sonnet-4-5-20250929-v1:0", "claude-sonnet-4-5"},
		{"vertex_ai/claude-haiku-4-5@20251001", "claude-haiku-4-5"},
		{"hosted_vllm/Qwen/Qwen3.8-Flash-Next-FP8", "Qwen3.8-Flash-Next"},
		{"bedrock/zai.glm-5", "glm-5"},
		{"azure/gpt-6.1-sol", "gpt-6.1-sol"},
	}
	for _, tt := range tests {
		if got := ShortTarget(tt.in); got != tt.want {
			t.Errorf("ShortTarget(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsAlias(t *testing.T) {
	tests := []struct {
		m    Model
		want bool
	}{
		{Model{Name: "aws/claude-opus-latest", Target: "bedrock/eu.anthropic.claude-opus-5-5"}, true},
		{Model{Name: "local/qwen-latest", Target: "hosted_vllm/Qwen/Qwen3.8-Flash-Next-FP8"}, true},
		{Model{Name: "aws/claude-opus-5-5", Target: "bedrock/eu.anthropic.claude-opus-5-5"}, false},
		{Model{Name: "gcp/claude-sonnet-4-5", Target: "vertex_ai/claude-sonnet-4-5@20250929"}, false},
		{Model{Name: "local/Qwen3.8-Flash-Next-non-thinking", Target: "hosted_vllm/Qwen/Qwen3.8-Flash-Next-FP8"}, false},
	}
	for _, tt := range tests {
		if got := tt.m.IsAlias(); got != tt.want {
			t.Errorf("%s.IsAlias() = %v, want %v", tt.m.Name, got, tt.want)
		}
	}
}

func TestDedupe(t *testing.T) {
	models := []Model{
		{Name: "aws/claude-sonnet-4-5", Target: "bedrock/eu.anthropic.claude-sonnet-4-5-20250929-v1:0", ContextTokens: 200_000, OutputPrice: 16.5},
		{Name: "gcp/claude-sonnet-4-5", Target: "vertex_ai/claude-sonnet-4-5@20250929", ContextTokens: 200_000, OutputPrice: 15},
		// larger context beats lower price
		{Name: "aws/claude-sonnet-4-6", Target: "bedrock/eu.anthropic.claude-sonnet-4-6", ContextTokens: 1_000_000, OutputPrice: 16.5},
		{Name: "gcp/claude-sonnet-4-6", Target: "vertex_ai/claude-sonnet-4-6", ContextTokens: 200_000, OutputPrice: 15},
		// alias is dropped
		{Name: "aws/claude-opus-latest", Target: "bedrock/eu.anthropic.claude-opus-5-5", ContextTokens: 1_000_000},
		{Name: "aws/claude-opus-5-5", Target: "bedrock/eu.anthropic.claude-opus-5-5", ContextTokens: 1_000_000},
		// variant without thinking stays a separate entry
		{Name: "local/Qwen3-Next", Target: "hosted_vllm/Qwen/Qwen3-Next-FP8"},
		{Name: "local/Qwen3-Next-non-thinking", Target: "hosted_vllm/Qwen/Qwen3-Next-FP8", NoThinking: true},
	}
	var got []string
	for _, m := range Dedupe(models) {
		got = append(got, m.Name)
	}
	want := []string{"gcp/claude-sonnet-4-5", "aws/claude-sonnet-4-6", "aws/claude-opus-5-5", "local/Qwen3-Next", "local/Qwen3-Next-non-thinking"}
	if !slices.Equal(got, want) {
		t.Errorf("Dedupe = %v\nwant       %v", got, want)
	}
}

func TestSortByPrice(t *testing.T) {
	models := []Model{
		{Name: "b", OutputPrice: 10, InputPrice: 2},
		{Name: "a", OutputPrice: 10, InputPrice: 2},
		{Name: "expensive", OutputPrice: 30},
		{Name: "input", OutputPrice: 10, InputPrice: 3},
	}
	SortByPrice(models)
	var got []string
	for _, m := range models {
		got = append(got, m.Name)
	}
	if want := []string{"expensive", "input", "a", "b"}; !slices.Equal(got, want) {
		t.Errorf("SortByPrice = %v, want %v", got, want)
	}
}

func TestResolve(t *testing.T) {
	all := []Model{
		{Name: "aws/claude-opus-latest", Target: "bedrock/eu.anthropic.claude-opus-5-5"},
		{Name: "aws/claude-opus-5-5", Target: "bedrock/eu.anthropic.claude-opus-5-5"},
		{Name: "aws/claude-sonnet-4-5", Target: "bedrock/eu.anthropic.claude-sonnet-4-5-20250929-v1:0"},
		{Name: "gcp/claude-sonnet-4-5", Target: "vertex_ai/claude-sonnet-4-5@20250929"},
	}
	shown := []Model{all[1], all[3]}
	tests := []struct {
		name string
		want int
	}{
		{"aws/claude-opus-5-5", 0},
		{"aws/claude-opus-latest", 0}, // alias
		{"aws/claude-sonnet-4-5", 1},  // hidden duplicate
		{"does-not-exist", -1},
	}
	for _, tt := range tests {
		if got := Resolve(all, shown, tt.name); got != tt.want {
			t.Errorf("Resolve(%q) = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestSuggest(t *testing.T) {
	m := func(name, target string, ctx int, out float64) Model {
		return Model{Name: name, Target: target, ContextTokens: ctx, OutputPrice: out}
	}
	gateway := []Model{
		m("aws/claude-opus-4-6", "bedrock/eu.anthropic.claude-opus-4-6-v1", 1_000_000, 27.5),
		m("aws/claude-opus-5-5", "bedrock/eu.anthropic.claude-opus-5-5", 1_000_000, 22),
		m("aws/claude-opus-latest", "bedrock/eu.anthropic.claude-opus-5-5", 1_000_000, 22),
		m("aws/claude-sonnet-4-5", "bedrock/eu.anthropic.claude-sonnet-4-5-20250929-v1:0", 200_000, 16.5),
		m("gcp/claude-sonnet-5", "vertex_ai/claude-sonnet-5", 1_000_000, 10),
		m("aws/claude-sonnet-5", "bedrock/eu.anthropic.claude-sonnet-5", 1_000_000, 11),
		m("azure/gpt-luna-latest", "azure/gpt-6-luna", 922_000, 0.5),
	}
	tests := []struct{ slot, want string }{
		{"opus", "aws/claude-opus-latest"}, // latest alias beats concrete version
		{"sonnet", "gcp/claude-sonnet-5"},  // highest version, then cheaper
		{"haiku", ""},                      // no candidate
	}
	for _, tt := range tests {
		if got := Suggest(gateway, tt.slot); got != tt.want {
			t.Errorf("Suggest(%s) = %q, want %q", tt.slot, got, tt.want)
		}
	}

	// Different gateway, different names: version 4-10 beats 4-9.
	other := []Model{
		m("anthropic/claude-opus-4-9", "anthropic/claude-opus-4-9", 200_000, 25),
		m("anthropic/claude-opus-4-10", "anthropic/claude-opus-4-10", 200_000, 25),
	}
	if got := Suggest(other, "opus"); got != "anthropic/claude-opus-4-10" {
		t.Errorf("Suggest(other, opus) = %q", got)
	}

	if got := ForSlot(gateway, "sonnet")[0].Name; got != "gcp/claude-sonnet-5" {
		t.Errorf("ForSlot(sonnet)[0] = %q", got)
	}
}

func TestSortByBench(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	models := []Model{
		{Name: "none-expensive", OutputPrice: 30},
		{Name: "good", Bench: &Bench{Intelligence: f(50), TerminalBench: f(10)}},
		{Name: "none-cheap", OutputPrice: 1},
		{Name: "better", Bench: &Bench{Intelligence: f(60)}},
	}
	SortByBench(models, "intel")
	var got []string
	for _, m := range models {
		got = append(got, m.Name)
	}
	if want := []string{"better", "good", "none-expensive", "none-cheap"}; !slices.Equal(got, want) {
		t.Errorf("intel: %v, want %v", got, want)
	}
	SortByBench(models, "tb4")
	if models[0].Name != "good" {
		t.Errorf("tb4: first %s, want good", models[0].Name)
	}
}
