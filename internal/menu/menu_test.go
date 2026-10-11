package menu

import (
	"strings"
	"testing"

	"github.com/schowave/llmenu/internal/catalog"
)

func f(v float64) *float64 { return &v }

var testModels = []catalog.Model{
	{Name: "aws/claude-opus-5-5", Target: "bedrock/eu.anthropic.claude-opus-5-5", ContextTokens: 1_000_000, InputPrice: 5, OutputPrice: 25},
	{Name: "azure/gpt-5.6-sol", Target: "azure/gpt-5.6-sol", ContextTokens: 400_000, InputPrice: 1.25, OutputPrice: 10},
	{Name: "local/qwen3-next", Target: "hosted_vllm/Qwen/Qwen3-Next-FP8", ContextTokens: 128_000},
}

func TestPickWithoutModels(t *testing.T) {
	if i, err := Pick(nil, Options{}); err == nil || i != -1 {
		t.Errorf("Pick(nil) = %d, %v; want an error", i, err)
	}
}

func TestViewHint(t *testing.T) {
	hint := func(m catalog.Model) string {
		if strings.HasPrefix(m.Name, "azure/") {
			return "no /v1/messages"
		}
		return ""
	}
	tests := []struct {
		name  string
		hint  func(catalog.Model) string
		marks []bool // per row: starts with "!"
	}{
		{"no hint function", nil, []bool{false, false, false}},
		{"hint for one model", hint, []bool{false, true, false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header, rows, preview := view(testModels, Options{Hint: tt.hint})
			if !strings.Contains(header, "MODEL") {
				t.Errorf("header = %q", header)
			}
			for i, row := range rows {
				if got := strings.HasPrefix(row, "!"); got != tt.marks[i] {
					t.Errorf("row %d = %q, mark %v, want %v", i, row, got, tt.marks[i])
				}
				if got := strings.Contains(preview(i), "! no /v1/messages"); got != tt.marks[i] {
					t.Errorf("preview %d shows hint: %v, want %v", i, got, tt.marks[i])
				}
			}
		})
	}
}

func TestViewPreview(t *testing.T) {
	_, _, preview := view(testModels, Options{Note: "Benchmarks: test source"})
	if got := preview(-1); got != "" {
		t.Errorf("preview(-1) = %q, want empty", got)
	}
	if got := preview(len(testModels)); got != "" {
		t.Errorf("preview out of range = %q, want empty", got)
	}
	if got := preview(0); !strings.HasPrefix(got, testModels[0].Name+"\n") {
		t.Errorf("preview(0) does not describe the first model:\n%s", got)
	}
	got := preview(1)
	for _, want := range []string{"azure/gpt-5.6-sol\n", "Target model azure/gpt-5.6-sol", "Benchmarks: test source\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("preview misses %q:\n%s", want, got)
		}
	}
	_, _, preview = view(testModels, Options{})
	if strings.Contains(preview(1), "Benchmarks:") {
		t.Errorf("note without Options.Note:\n%s", preview(1))
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
		"aws/claude-opus-5-5\n\n! slot mismatch\n",
		"Target model bedrock/eu.anthropic.claude-opus-5-5\n",
		"Context      1000000 tokens\n",
		"Max. output  128000 tokens\n",
		"Vision       yes\n",
		"Reasoning    yes\n",
		"  Input           5.000\n",
		"  Output         25.000\n",
		"  Cache-Read      0.500\n",
		"  Cache-Write     6.250\n",
		"Endpoints\n  /v1/messages\n  /v1/responses\n",
		"Benchmarks (claude-opus-5-5)\n",
		"  Intelligence    57.6\n",
		"  Terminal-B. 4   59.6 %\n",
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
	for _, unwanted := range []string{"!", "Endpoints", "Benchmarks"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("Details shows %q:\n%s", unwanted, got)
		}
	}
	for _, want := range []string{"Vision       no\n", "Reasoning    no\n"} {
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

	header, _, _ := view(models, Options{Hint: func(m catalog.Model) string { return hints[m.Name] }})
	if !strings.HasSuffix(header, "   ! not a Claude model / gateway reports no Responses API") {
		t.Errorf("header without legend: %q", header)
	}
}
