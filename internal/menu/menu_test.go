package menu

import (
	"strings"
	"testing"

	"github.com/schowave/llmenu/internal/catalog"
)

func f(v float64) *float64 { return &v }

func TestPickWithoutModels(t *testing.T) {
	if i, err := Pick(nil, Options{}); err == nil || i != -1 {
		t.Errorf("Pick(nil) = %d, %v; want an error", i, err)
	}
}

func TestDetails(t *testing.T) {
	m := catalog.Model{
		Name:            "aws/claude-opus-5-5",
		Target:          "bedrock/eu.anthropic.claude-opus-5-5",
		ContextTokens:   1_000_000,
		MaxOutputTokens: 128_000,
		Vision:          true,
		Reasoning:       true,
		Endpoints:       []string{"/v1/messages", "/v1/responses"},
		InputPrice:      5,
		OutputPrice:     25,
		CacheReadPrice:  0.5,
		CacheWritePrice: 6.25,
		Bench:           &catalog.Bench{Slug: "claude-opus-5-5", Intelligence: f(57.6), TerminalBench: f(59.6)},
	}
	got := Details(m, "slot mismatch")
	for _, want := range []string{
		"aws/claude-opus-5-5\n→ bedrock/eu.anthropic.claude-opus-5-5\n\n! slot mismatch\n",
		"Context  1M · output 128k\n",
		"Vision yes · reasoning yes\n",
		"$ per 1M tokens\n  in 5.00 · out 25.00\n  cache read 0.50 · write 6.25\n",
		"Endpoints  /v1/messages, /v1/responses\n",
		"Benchmarks (claude-opus-5-5)\n",
		"  Intelligence    57.6\n",
		"  Terminal-Bench  59.6 %\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Details misses %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Coding") {
		t.Errorf("missing value shown:\n%s", got)
	}

	// Without thinking, reasoning is off; no hint, endpoints or benchmarks.
	m = catalog.Model{Name: "local/qwen3-next", Reasoning: true, NoThinking: true}
	got = Details(m, "")
	for _, unwanted := range []string{"!", "Endpoints", "Benchmarks", "→"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("Details shows %q:\n%s", unwanted, got)
		}
	}
	for _, want := range []string{"Vision no · reasoning no\n", "Context  - · output -\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("Details misses %q:\n%s", want, got)
		}
	}
}

func TestLegend(t *testing.T) {
	models := []catalog.Model{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	hints := map[string]string{
		"a": "not a Claude model, runs through the gateway's translation",
		"b": "not a Claude model, runs through the gateway's translation",
		"c": "gateway reports no Responses API; LiteLLM translates",
	}
	if got := legend(models, func(m catalog.Model) string { return hints[m.Name] }); got != "   ! not a Claude model / gateway reports no Responses API" {
		t.Errorf("legend = %q", got)
	}
	if got := legend(models, func(catalog.Model) string { return "" }); got != "" {
		t.Errorf("legend without hints = %q", got)
	}

}

func TestMoney(t *testing.T) {
	for v, want := range map[float64]string{0: "0.00", 4.4: "4.40", 0.068: "0.07", 0.0035: "0.0035", 22: "22.00"} {
		if got := money(v); got != want {
			t.Errorf("money(%v) = %q, want %q", v, got, want)
		}
	}
}
