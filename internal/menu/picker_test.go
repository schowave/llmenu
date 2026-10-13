package menu

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/schowave/llmenu/internal/catalog"
)

// pickerModels are sorted by INTEL, as the caller does.
var pickerModels = []catalog.Model{
	{Name: "aws/claude-opus-5-5", Target: "bedrock/eu.anthropic.claude-opus-5-5", ContextTokens: 1_000_000, MaxOutputTokens: 128_000,
		InputPrice: 4.4, OutputPrice: 22, Bench: &catalog.Bench{Slug: "claude-opus-5-5", Intelligence: f(57.6), TerminalBench: f(59.6)}},
	{Name: "azure/gpt-6-sol", Target: "azure/gpt-6-sol", ContextTokens: 922_000, InputPrice: 2, OutputPrice: 10,
		Bench: &catalog.Bench{Intelligence: f(47.6), TerminalBench: f(43.9), Coding: f(80)}},
	{Name: "gcp/claude-sonnet-4-5", Target: "vertex_ai/claude-sonnet-4-5", ContextTokens: 200_000, InputPrice: 3, OutputPrice: 15,
		Bench: &catalog.Bench{Intelligence: f(42)}},
	{Name: "local/Qwen3-Next", Target: "hosted_vllm/Qwen/Qwen3-Next-FP8", ContextTokens: 229_376},
}

func notClaude(m catalog.Model) string {
	if strings.Contains(m.Name, "claude") {
		return ""
	}
	return "not a Claude model, runs through the gateway's translation"
}

func newTestPicker(opt Options) *picker {
	p := newPicker(pickerModels, opt, newStyles(lipgloss.NewRenderer(nil)))
	p.Update(tea.WindowSizeMsg{Width: 160, Height: 20})
	return p
}

func key(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }
func typeText(p *picker, s string) {
	for _, r := range s {
		p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

// plain is the view without colors, line by line.
func plain(p *picker) []string { return strings.Split(ansi.Strip(p.View()), "\n") }

func selectedName(p *picker) string { return p.models[p.current()].Name }

func TestPickerLayout(t *testing.T) {
	p := newTestPicker(Options{Hint: notClaude, Note: "Benchmarks: test data", Metric: "intel"})
	p.Update(tea.WindowSizeMsg{Width: 160, Height: 40}) // room for the whole preview
	lines := plain(p)

	// Search on top, then the header, best model first.
	if !strings.HasPrefix(lines[0], "Search: ") || !strings.Contains(lines[0], "4/4") {
		t.Errorf("search line: %q", lines[0])
	}
	for _, col := range []string{"MODEL", "CONTEXT", "$IN/1M", "$OUT/1M", "▼INTEL", "TB4 %", "CODE"} {
		if !strings.Contains(lines[1], col) {
			t.Errorf("header %q lacks %q", lines[1], col)
		}
	}
	if !strings.Contains(lines[2], "▶ ") || !strings.Contains(lines[2], "aws/claude-opus-5-5") || !strings.Contains(lines[2], "1M") {
		t.Errorf("first row: %q", lines[2])
	}
	// Hints are no longer marked in the rows, only noted in the preview.
	if !strings.HasPrefix(lines[3], "  azure/gpt-6-sol") || strings.Contains(lines[3], "!") || !strings.Contains(lines[3], "922k") {
		t.Errorf("row with a hint: %q", lines[3])
	}
	view := strings.Join(lines, "\n")
	for _, want := range []string{"→ bedrock/eu.anthropic.claude-opus-5-5", "Columns", "Terminal-Bench v4", "Benchmarks: test data", "tab sort (INTEL)"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q", want)
		}
	}
}

func TestPickerHintInPreviewOnly(t *testing.T) {
	p := newTestPicker(Options{Hint: notClaude})
	p.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	p.Update(key(tea.KeyDown)) // azure/gpt-6-sol
	view := strings.Join(plain(p), "\n")
	if !strings.Contains(view, "Note: not a Claude model") {
		t.Errorf("hint missing from the preview:\n%s", view)
	}
	p.Update(key(tea.KeyUp)) // a Claude model: no note
	if strings.Contains(strings.Join(plain(p), "\n"), "Note:") {
		t.Error("note shown for a model without a hint")
	}
}

func TestPickerPreselectAndMove(t *testing.T) {
	p := newTestPicker(Options{Preselect: 2})
	if selectedName(p) != "gcp/claude-sonnet-4-5" {
		t.Fatalf("cursor on %s, want the preselected model", selectedName(p))
	}
	p.Update(key(tea.KeyUp))
	if selectedName(p) != "azure/gpt-6-sol" {
		t.Errorf("up: %s", selectedName(p))
	}
	p.Update(key(tea.KeyEnd))
	if selectedName(p) != "local/Qwen3-Next" {
		t.Errorf("end: %s", selectedName(p))
	}
	p.Update(key(tea.KeyDown)) // stays at the end
	p.Update(key(tea.KeyHome))
	if selectedName(p) != "aws/claude-opus-5-5" {
		t.Errorf("home: %s", selectedName(p))
	}
	p.Update(key(tea.KeyPgDown))
	if selectedName(p) != "local/Qwen3-Next" {
		t.Errorf("page down: %s", selectedName(p))
	}
	// Out-of-range preselection starts on the first (best) model.
	if p := newTestPicker(Options{Preselect: 99}); selectedName(p) != "aws/claude-opus-5-5" {
		t.Errorf("invalid preselect: %s", selectedName(p))
	}
}

func TestPickerSearch(t *testing.T) {
	p := newTestPicker(Options{})
	typeText(p, "gpt")
	lines := plain(p)
	// Fuzzy: "gpt" also matches g-c-p…t in gcp/claude-sonnet-4-5.
	if !strings.Contains(lines[0], "Search: gpt") || !strings.Contains(lines[0], "2/4") {
		t.Errorf("search line: %q", lines[0])
	}
	if selectedName(p) != "azure/gpt-6-sol" {
		t.Errorf("selected %s", selectedName(p))
	}
	if _, cmd := p.Update(key(tea.KeyEnter)); cmd == nil || p.chosen != 1 {
		t.Errorf("enter: chosen %d, cmd %v", p.chosen, cmd)
	}

	p = newTestPicker(Options{})
	typeText(p, "zzz")
	if !strings.Contains(strings.Join(plain(p), "\n"), `no model matches "zzz"`) {
		t.Error("no empty state")
	}
	if _, cmd := p.Update(key(tea.KeyEnter)); cmd != nil || p.chosen != -1 {
		t.Error("enter without a match selected something")
	}
	p.Update(key(tea.KeyBackspace))
	p.Update(key(tea.KeyCtrlU))
	if len(p.shown) != len(pickerModels) || p.query != "" {
		t.Errorf("clear: %d shown, query %q", len(p.shown), p.query)
	}

	// The cursor stays on the same model while typing, if it still matches.
	p = newTestPicker(Options{Preselect: 2})
	typeText(p, "claude")
	if selectedName(p) != "gcp/claude-sonnet-4-5" {
		t.Errorf("cursor jumped to %s", selectedName(p))
	}
}

func TestPickerSort(t *testing.T) {
	p := newTestPicker(Options{Preselect: 2, Metric: "intel"})
	p.Update(key(tea.KeyTab)) // TB4
	lines := plain(p)
	if !strings.Contains(lines[1], "▼TB4 %") || !strings.Contains(lines[2], "aws/claude-opus-5-5") || !strings.Contains(lines[3], "azure/gpt-6-sol") {
		t.Errorf("TB4 sort:\n%s\n%s\n%s", lines[1], lines[2], lines[3])
	}
	if selectedName(p) != "gcp/claude-sonnet-4-5" {
		t.Errorf("cursor lost its model: %s", selectedName(p))
	}
	p.Update(key(tea.KeyTab)) // CODE
	if lines := plain(p); !strings.Contains(lines[2], "azure/gpt-6-sol") {
		t.Errorf("CODE sort: %q", lines[2])
	}
	p.Update(key(tea.KeyTab)) // price
	if lines := plain(p); !strings.Contains(lines[1], "▼$OUT/1M") || !strings.Contains(lines[2], "aws/claude-opus-5-5") {
		t.Errorf("price sort: %q / %q", lines[1], lines[2])
	}
	p.Update(key(tea.KeyShiftTab)) // back to CODE
	if p.sortKey != "code" {
		t.Errorf("shift-tab: %s", p.sortKey)
	}

	// Without benchmarks only price exists, and Tab changes nothing.
	plainModels := []catalog.Model{{Name: "b", OutputPrice: 1}, {Name: "a", OutputPrice: 2}}
	q := newPicker(plainModels, Options{}, newStyles(lipgloss.NewRenderer(nil)))
	q.Update(key(tea.KeyTab))
	if q.sortKey != "price" || q.models[q.shown[0]].Name != "a" {
		t.Errorf("no benchmarks: %s, first %s", q.sortKey, q.models[q.shown[0]].Name)
	}
	if strings.Contains(ansi.Strip(q.View()), "tab sort") {
		t.Error("sort hint shown without benchmarks")
	}
}

func TestPickerFitsTheScreen(t *testing.T) {
	// A long preview must not push the table off a short terminal.
	for _, h := range []int{8, 12, 24} {
		p := newTestPicker(Options{Hint: notClaude, Note: "Benchmarks: test data"})
		p.Update(tea.WindowSizeMsg{Width: 160, Height: h})
		lines := plain(p)
		if len(lines) > h {
			t.Errorf("height %d: view has %d lines", h, len(lines))
		}
		if !strings.HasPrefix(lines[0], "Search: ") || !strings.Contains(lines[1], "MODEL") {
			t.Errorf("height %d: top lines %q / %q", h, lines[0], lines[1])
		}
	}
}

func TestPickerAbort(t *testing.T) {
	for _, k := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC} {
		p := newTestPicker(Options{})
		if _, cmd := p.Update(key(k)); cmd == nil || !p.aborted {
			t.Errorf("%v did not abort", k)
		}
	}
}

func TestPickerNarrowAndScroll(t *testing.T) {
	p := newTestPicker(Options{Hint: notClaude, Note: "Benchmarks: test data"})
	p.Update(tea.WindowSizeMsg{Width: 80, Height: 11}) // too narrow for the side preview
	view := strings.Join(plain(p), "\n")
	if strings.Contains(view, "Columns") || !strings.Contains(view, "context 1M") || !strings.Contains(view, "Benchmarks: test data") {
		t.Errorf("narrow view:\n%s", view)
	}
	if p.rows() != 3 {
		t.Fatalf("rows = %d", p.rows())
	}
	p.Update(key(tea.KeyEnd)) // scrolls so the last model is visible
	view = strings.Join(plain(p), "\n")
	if !strings.Contains(view, "local/Qwen3-Next") || strings.Contains(view, "aws/claude-opus-5-5 ") {
		t.Errorf("scrolled view:\n%s", view)
	}
	if strings.Contains(view, "!") {
		t.Errorf("marker shown:\n%s", view)
	}
}

func TestPickerFooterFits(t *testing.T) {
	for _, w := range []int{160, 90, 60, 30} {
		p := newTestPicker(Options{Hint: notClaude})
		p.Update(tea.WindowSizeMsg{Width: w, Height: 20})
		lines := plain(p)
		footer := lines[len(lines)-1]
		if ansiWidth(footer) > w {
			t.Errorf("width %d: footer %q is %d wide", w, footer, ansiWidth(footer))
		}
		if !strings.Contains(footer, "enter") {
			t.Errorf("width %d: keys lost: %q", w, footer)
		}
	}
}

func ansiWidth(s string) int { return lipgloss.Width(s) }

func TestPickerBenchTip(t *testing.T) {
	// Without benchmarks the footer suggests llmenu bench, as long as it fits.
	plainModels := []catalog.Model{{Name: "a/x", OutputPrice: 2}, {Name: "b/y", OutputPrice: 1}}
	p := newPicker(plainModels, Options{}, newStyles(lipgloss.NewRenderer(nil)))
	p.Update(tea.WindowSizeMsg{Width: 120, Height: 10})
	lines := plain(p)
	if footer := lines[len(lines)-1]; !strings.Contains(footer, "no quality scores yet: run llmenu bench") {
		t.Errorf("footer without tip: %q", footer)
	}
	p.Update(tea.WindowSizeMsg{Width: 20, Height: 10})
	if lines := plain(p); lipgloss.Width(lines[len(lines)-1]) > 20 {
		t.Errorf("narrow footer too wide: %q", lines[len(lines)-1])
	}
	// With benchmarks there is no tip.
	q := newTestPicker(Options{})
	if lines := plain(q); strings.Contains(lines[len(lines)-1], "llmenu bench") {
		t.Errorf("tip despite benchmarks: %q", lines[len(lines)-1])
	}
}

func TestPickerSelectedRowStyle(t *testing.T) {
	// With colors: the selected row keeps the green top values, and its
	// background runs through the padding after the name.
	r := lipgloss.NewRenderer(nil)
	r.SetColorProfile(termenv.ANSI256)
	r.SetHasDarkBackground(true)
	st := newStyles(r)
	p := newPicker(pickerModels, Options{}, st)
	p.Update(tea.WindowSizeMsg{Width: 160, Height: 20})
	row := p.row(p.shown[0], true, p.nameWidth()) // aws/claude-opus-5-5: best INTEL and TB4

	green := st.good.GetForeground()
	want := st.selected.Foreground(green).Render("   57.6")
	if !strings.Contains(row, want) {
		t.Errorf("selected row loses the green:\n%q\nwant part %q", row, want)
	}
	pad := st.selected.Render(strings.Repeat(" ", p.nameWidth()-len("aws/claude-opus-5-5")))
	if !strings.Contains(row, pad) {
		t.Errorf("padding not styled like the row:\n%q", row)
	}
}

func TestTruncateAndPad(t *testing.T) {
	if got := truncate("abcdefgh", 5); got != "abcd…" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("abc", 5); got != "abc" {
		t.Errorf("truncate short = %q", got)
	}
	if got := padRight("ab", 4); got != "ab  " {
		t.Errorf("pad = %q", got)
	}
	if got := padRight("abcdef", 4); got != "abcdef" {
		t.Errorf("pad long = %q", got)
	}
}

func TestPickerProjectAndSpend(t *testing.T) {
	tests := []struct {
		opt  Options
		want string
	}{
		{Options{}, "4/4"},
		{Options{Spend: "$1.00 / $5.00"}, "$1.00 / $5.00 · 4/4"},
		{Options{Project: "acme", Spend: "$1.00 spent"}, "project acme · $1.00 spent · 4/4"},
		{Options{Project: "acme"}, "project acme · 4/4"},
	}
	for _, tt := range tests {
		line := plain(newTestPicker(tt.opt))[0]
		if !strings.HasSuffix(strings.TrimRight(line, " "), tt.want) {
			t.Errorf("search line %q, want it to end in %q", line, tt.want)
		}
	}
}
