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
		// the same model at different providers stays selectable
		{Name: "aws/claude-haiku-5-5", Target: "bedrock/eu.anthropic.claude-haiku-5-5", ContextTokens: 200_000, OutputPrice: 5.5},
		{Name: "gcp/claude-haiku-5-5", Target: "vertex_ai/claude-haiku-5-5", ContextTokens: 200_000, OutputPrice: 5},
		// at the same provider the cheaper one wins
		{Name: "aws/claude-sonnet-4-5", Target: "bedrock/eu.anthropic.claude-sonnet-4-5-20250929-v1:0", ContextTokens: 200_000, OutputPrice: 16.5},
		{Name: "aws-us/claude-sonnet-4-5", Target: "bedrock/us.anthropic.claude-sonnet-4-5-20250929-v1:0", ContextTokens: 200_000, OutputPrice: 15},
		// larger context beats lower price
		{Name: "aws/claude-sonnet-4-6", Target: "bedrock/eu.anthropic.claude-sonnet-4-6", ContextTokens: 1_000_000, OutputPrice: 16.5},
		{Name: "aws-us/claude-sonnet-4-6", Target: "bedrock/us.anthropic.claude-sonnet-4-6", ContextTokens: 200_000, OutputPrice: 15},
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
	want := []string{"aws/claude-haiku-5-5", "gcp/claude-haiku-5-5", "aws-us/claude-sonnet-4-5", "aws/claude-sonnet-4-6", "aws/claude-opus-5-5", "local/Qwen3-Next", "local/Qwen3-Next-non-thinking"}
	if !slices.Equal(got, want) {
		t.Errorf("Dedupe = %v\nwant       %v", got, want)
	}
}

func TestDedupeTieKeepsFirst(t *testing.T) {
	// LiteLLM balances load over deployments with the same model_name; if
	// nothing tells them apart, the first one is kept.
	models := []Model{
		{Name: "azure/gpt-6", Target: "azure/gpt-6", ContextTokens: 400_000, MaxOutputTokens: 1},
		{Name: "azure/gpt-6", Target: "azure/gpt-6", ContextTokens: 400_000, MaxOutputTokens: 2},
	}
	if got := Dedupe(models); len(got) != 1 || got[0].MaxOutputTokens != 1 {
		t.Errorf("Dedupe = %+v, want only the first deployment", got)
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
		{Name: "aws-us/claude-sonnet-4-5", Target: "bedrock/us.anthropic.claude-sonnet-4-5-20250929-v1:0"},
		{Name: "gcp/claude-sonnet-4-5", Target: "vertex_ai/claude-sonnet-4-5@20250929"},
	}
	shown := []Model{all[1], all[3], all[4]}
	tests := []struct {
		name string
		want int
	}{
		{"aws/claude-opus-5-5", 0},
		{"aws/claude-opus-latest", 0}, // alias
		{"aws/claude-sonnet-4-5", 1},  // hidden duplicate at the same provider
		{"gcp/claude-sonnet-4-5", 2},  // other provider, shown itself
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
		m("aws/claude-sonnet-5", "bedrock/eu.anthropic.claude-sonnet-5", 1_000_000, 11),
		m("gcp/claude-sonnet-5", "vertex_ai/claude-sonnet-5", 1_000_000, 10),
		m("azure/gpt-luna-latest", "azure/gpt-6-luna", 922_000, 0.5),
	}
	tests := []struct{ slot, want string }{
		{"opus", "aws/claude-opus-latest"}, // latest alias beats concrete version
		{"sonnet", "gcp/claude-sonnet-5"},  // highest version, then cheaper
		{"haiku", ""},                      // no candidate
	}
	// The suggestion must not depend on the gateway's order.
	reversed := slices.Clone(gateway)
	slices.Reverse(reversed)
	for _, tt := range tests {
		for _, in := range [][]Model{gateway, reversed} {
			if got := Suggest(in, tt.slot); got != tt.want {
				t.Errorf("Suggest(%s) = %q, want %q (first model %s)", tt.slot, got, tt.want, in[0].Name)
			}
		}
	}

	// Different gateway, different names: version 4-10 beats 4-9, even if
	// 4-9 is cheaper.
	other := []Model{
		m("anthropic/claude-opus-4-9", "anthropic/claude-opus-4-9", 200_000, 25),
		m("anthropic/claude-opus-4-10", "anthropic/claude-opus-4-10", 200_000, 26),
	}
	if got := Suggest(other, "opus"); got != "anthropic/claude-opus-4-10" {
		t.Errorf("Suggest(other, opus) = %q", got)
	}

	if got := ForSlot(gateway, "sonnet")[0].Name; got != "gcp/claude-sonnet-5" {
		t.Errorf("ForSlot(sonnet)[0] = %q", got)
	}
}

func TestForSlotThinkingFirst(t *testing.T) {
	thinking := Model{Name: "local/qwen-sonnet", Target: "vllm/qwen-sonnet"}
	plain := Model{Name: "local/qwen-sonnet-nt", Target: "vllm/qwen-sonnet", NoThinking: true}
	// Same version: the variant with thinking comes first, whatever the input order.
	for _, in := range [][]Model{{thinking, plain}, {plain, thinking}} {
		if got := ForSlot(in, "sonnet"); got[0].Name != thinking.Name {
			t.Errorf("ForSlot(%s, %s)[0] = %s, want %s", in[0].Name, in[1].Name, got[0].Name, thinking.Name)
		}
	}
}

func TestSortByBench(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	models := []Model{
		{Name: "none-expensive", OutputPrice: 30},
		{Name: "good", Bench: &Bench{Intelligence: f(50), TerminalBench: f(10)}},
		{Name: "none-cheap", OutputPrice: 1},
		// more expensive than the models without a value, still ahead of them
		{Name: "better", OutputPrice: 40, Bench: &Bench{Intelligence: f(60)}},
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
