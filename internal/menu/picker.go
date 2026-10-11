package menu

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/sahilm/fuzzy"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/table"
)

// Sort keys, cycled with Tab. The benchmark keys are the values of
// bench.metric; "price" sorts by output price, most expensive first, like
// `llmenu list --sort price`.
var sortKeys = []string{"intel", "tb4", "code", "price"}

var sortLabels = map[string]string{"intel": "INTEL", "tb4": "TB4 %", "code": "CODE", "price": "$OUT/1M"}

// picker is the bubbletea model of the menu.
type picker struct {
	models []catalog.Model
	opt    Options
	st     styles

	sortKey  string
	order    []int         // indices into models, in the current sort order
	query    string        // what has been typed
	shown    []int         // indices into models that match the query, in sort order
	matches  map[int][]int // model index -> matched rune positions in its name
	cursor   int           // position in shown
	offset   int           // first visible row
	width    int
	height   int
	maxScore map[string]float64 // best value per benchmark, for highlighting

	chosen  int // model index once Enter was pressed, else -1
	aborted bool
}

func newPicker(models []catalog.Model, opt Options, st styles) *picker {
	p := &picker{models: models, opt: opt, st: st, chosen: -1, width: 120, height: 30, maxScore: map[string]float64{}}
	for _, key := range sortKeys[:3] {
		for _, m := range models {
			if v := m.Score(key); v != nil && *v > p.maxScore[key] {
				p.maxScore[key] = *v
			}
		}
	}
	// The caller already sorted the models; keep that order until Tab is
	// pressed, but show which column it corresponds to.
	p.sortKey = "price"
	if p.hasBench() {
		p.sortKey = cmp.Or(opt.Metric, "intel")
	}
	p.order = make([]int, len(models))
	for i := range models {
		p.order[i] = i
	}
	p.refilter()
	if opt.Preselect >= 0 && opt.Preselect < len(models) {
		p.moveTo(opt.Preselect)
	}
	return p
}

func (p *picker) hasBench() bool {
	return slices.ContainsFunc(p.models, func(m catalog.Model) bool { return m.Bench != nil })
}

func (p *picker) hint(m catalog.Model) string {
	if p.opt.Hint == nil {
		return ""
	}
	return p.opt.Hint(m)
}

// current is the model index under the cursor, or -1 without matches.
func (p *picker) current() int {
	if p.cursor < 0 || p.cursor >= len(p.shown) {
		return -1
	}
	return p.shown[p.cursor]
}

// moveTo puts the cursor on model index i if it is shown.
func (p *picker) moveTo(i int) {
	if pos := slices.Index(p.shown, i); pos >= 0 {
		p.cursor = pos
	}
	p.scroll()
}

// refilter applies the query and keeps the sort order (not the match score),
// so the table stays predictable while typing.
func (p *picker) refilter() {
	keep := p.current()
	p.matches = map[int][]int{}
	if p.query == "" {
		p.shown = slices.Clone(p.order)
	} else {
		names := make([]string, len(p.models))
		for i, m := range p.models {
			names[i] = m.Name
		}
		for _, f := range fuzzy.Find(p.query, names) {
			p.matches[f.Index] = f.MatchedIndexes
		}
		p.shown = p.shown[:0]
		for _, i := range p.order {
			if _, ok := p.matches[i]; ok {
				p.shown = append(p.shown, i)
			}
		}
	}
	p.cursor = 0
	if keep >= 0 {
		p.moveTo(keep)
	}
	p.scroll()
}

// resort orders by the current sort key, keeping the model under the cursor.
func (p *picker) resort() {
	sorted := slices.Clone(p.models)
	if p.sortKey == "price" {
		catalog.SortByPrice(sorted)
	} else {
		catalog.SortByBench(sorted, p.sortKey)
	}
	byName := map[string]int{}
	for i, m := range p.models {
		byName[m.Name] = i
	}
	p.order = p.order[:0]
	for _, m := range sorted {
		p.order = append(p.order, byName[m.Name])
	}
	p.refilter()
}

func (p *picker) Init() tea.Cmd { return nil }

func (p *picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width, p.height = msg.Width, msg.Height
		p.scroll()
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc, tea.KeyCtrlC:
			p.aborted = true
			return p, tea.Quit
		case tea.KeyEnter:
			if i := p.current(); i >= 0 {
				p.chosen = i
				return p, tea.Quit
			}
		case tea.KeyUp, tea.KeyCtrlP:
			p.step(-1)
		case tea.KeyDown, tea.KeyCtrlN:
			p.step(1)
		case tea.KeyPgUp:
			p.step(-p.rows())
		case tea.KeyPgDown:
			p.step(p.rows())
		case tea.KeyHome:
			p.step(-len(p.shown))
		case tea.KeyEnd:
			p.step(len(p.shown))
		case tea.KeyTab:
			p.cycleSort(1)
		case tea.KeyShiftTab:
			p.cycleSort(-1)
		case tea.KeyBackspace:
			if r := []rune(p.query); len(r) > 0 {
				p.query = string(r[:len(r)-1])
				p.refilter()
			}
		case tea.KeyCtrlU:
			p.query = ""
			p.refilter()
		case tea.KeyRunes, tea.KeySpace:
			p.query += string(msg.Runes)
			p.refilter()
		}
	}
	return p, nil
}

func (p *picker) step(n int) {
	if len(p.shown) == 0 {
		return
	}
	p.cursor = min(max(p.cursor+n, 0), len(p.shown)-1)
	p.scroll()
}

func (p *picker) cycleSort(dir int) {
	keys := sortKeys
	if !p.hasBench() {
		keys = []string{"price"}
	}
	i := slices.Index(keys, p.sortKey)
	p.sortKey = keys[(i+dir+len(keys))%len(keys)]
	p.resort()
}

// Layout: search line, header, rows, footer; the preview sits to the right
// when there is room, otherwise below the rows.
const (
	previewMinWidth = 44
	belowPreview    = 4
)

func (p *picker) sidePreview() bool { return p.width-tableWidth(p.nameWidth()) >= previewMinWidth }

// rows is the number of visible table rows.
func (p *picker) rows() int {
	n := p.height - 3 // search, header, footer
	if !p.sidePreview() {
		n -= belowPreview + 1
	}
	return max(n, 1)
}

func (p *picker) scroll() {
	rows := p.rows()
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+rows {
		p.offset = p.cursor - rows + 1
	}
	p.offset = max(min(p.offset, len(p.shown)-rows), 0)
}

const nameMax = 44

func (p *picker) nameWidth() int {
	w := runewidth.StringWidth("MODEL")
	for _, m := range p.models {
		w = max(w, runewidth.StringWidth(m.Name))
	}
	return min(w, nameMax)
}

// tableWidth: cursor (2) + name + context, prices, benchmarks.
func tableWidth(name int) int { return 2 + name + 8 + 9 + 9 + 7 + 7 + 7 }

func (p *picker) View() string {
	nw := p.nameWidth()
	var b strings.Builder

	count := fmt.Sprintf("%d/%d", len(p.shown), len(p.models))
	search := p.st.prompt.Render("Search: ") + p.query + p.st.cursorBlock.Render(" ")
	b.WriteString(padRight(search, tableWidth(nw)-runewidth.StringWidth(count)) + p.st.dim.Render(count) + "\n")
	b.WriteString(p.header(nw) + "\n")

	rows := p.rows()
	var lines []string
	if len(p.shown) == 0 {
		lines = append(lines, p.st.dim.Render("  no model matches "+fmt.Sprintf("%q", p.query)+" (Ctrl-U clears)"))
	}
	for pos := p.offset; pos < len(p.shown) && pos < p.offset+rows; pos++ {
		lines = append(lines, p.row(p.shown[pos], pos == p.cursor, nw))
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}

	var detail string
	if i := p.current(); i >= 0 {
		detail = p.details(i)
	}
	if p.sidePreview() {
		left := lipgloss.NewStyle().Width(tableWidth(nw)).Render(strings.Join(lines, "\n"))
		// Height is only a minimum in lipgloss; MaxHeight keeps a long preview
		// from pushing the table off the screen.
		right := p.st.preview.Width(p.width - tableWidth(nw) - 3).Height(rows).MaxHeight(rows).Render(detail)
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, right) + "\n")
	} else {
		b.WriteString(strings.Join(lines, "\n") + "\n")
		b.WriteString(p.st.dim.Render(strings.Repeat("─", max(p.width, 10))) + "\n")
		b.WriteString(lipgloss.NewStyle().Height(belowPreview).MaxHeight(belowPreview).Render(p.compactDetails()) + "\n")
	}
	b.WriteString(p.footer())
	return b.String()
}

func (p *picker) header(nw int) string {
	col := func(label, key string, width int) string {
		if p.sortKey == key {
			label = "▼" + label
		}
		return fmt.Sprintf("%*s", width, label)
	}
	h := "  " + fmt.Sprintf("%-*s", nw, "MODEL") +
		fmt.Sprintf("%8s", "CONTEXT") + fmt.Sprintf("%9s", "$IN/1M") + col("$OUT/1M", "price", 9) +
		col("INTEL", "intel", 7) + col("TB4 %", "tb4", 7) + col("CODE", "code", 7)
	return p.st.header.Render(h)
}

func (p *picker) row(i int, selected bool, nw int) string {
	m := p.models[i]
	// Hints (e.g. "not a Claude model") only appear in the preview: every model
	// is a valid choice, so rows are neither marked nor dimmed.
	marker := "  "
	base := p.st.normal
	if selected {
		marker = p.st.marker.Render("▶ ")
		base = p.st.selected
	}
	name := p.highlight(truncate(m.Name, nw), p.matches[i], base)
	nums := base.Render(fmt.Sprintf("%8s%9.2f%9.2f", table.Tokens(m.ContextTokens), m.InputPrice, m.OutputPrice))
	return marker + padRight(name, nw) + nums +
		p.score(m, "intel", base) + p.score(m, "tb4", base) + p.score(m, "code", base)
}

// score renders a benchmark value; the top values per column are green.
func (p *picker) score(m catalog.Model, key string, base lipgloss.Style) string {
	v := m.Score(key)
	if v == nil {
		return base.Render(fmt.Sprintf("%7s", "-"))
	}
	s := fmt.Sprintf("%7.1f", *v)
	if p.maxScore[key] > 0 && *v >= 0.9*p.maxScore[key] {
		return base.Inherit(p.st.good).Render(s)
	}
	return base.Render(s)
}

// highlight marks the matched characters of the name.
func (p *picker) highlight(name string, matched []int, base lipgloss.Style) string {
	if len(matched) == 0 {
		return base.Render(name)
	}
	var b strings.Builder
	for i, r := range []rune(name) {
		if slices.Contains(matched, i) {
			b.WriteString(base.Inherit(p.st.match).Render(string(r)))
		} else {
			b.WriteString(base.Render(string(r)))
		}
	}
	return b.String()
}

func (p *picker) details(i int) string {
	m := p.models[i]
	d := Details(m, p.hint(m))
	// Most important first: a short terminal cuts the preview at the bottom.
	if p.opt.Note != "" {
		d += "\n" + p.opt.Note + "\n"
	}
	d += "\nColumns\n" +
		"  CONTEXT  context window\n" +
		"  $/1M     price per million input/output tokens\n" +
		"  INTEL    Intelligence Index\n" +
		"  TB4 %    Terminal-Bench v4, tasks solved\n" +
		"  CODE     Coding Index\n"
	return d
}

// compactDetails is the preview for narrow terminals.
func (p *picker) compactDetails() string {
	i := p.current()
	if i < 0 {
		return ""
	}
	m := p.models[i]
	lines := []string{
		p.st.header.Render(m.Name) + p.st.dim.Render("  → "+m.Target),
		fmt.Sprintf("context %s · output %s · $%.2f in / $%.2f out per 1M tokens",
			table.Tokens(m.ContextTokens), table.Tokens(m.MaxOutputTokens), m.InputPrice, m.OutputPrice),
	}
	if h := p.hint(m); h != "" {
		lines = append(lines, "Note: "+h)
	}
	if p.opt.Note != "" {
		lines = append(lines, p.st.dim.Render(p.opt.Note))
	}
	return strings.Join(lines, "\n")
}

func (p *picker) footer() string {
	sortHint := ""
	if p.hasBench() {
		sortHint = " · tab sort (" + sortLabels[p.sortKey] + ")"
	}
	// Drop the least important hints first when the terminal is narrow.
	for _, keys := range []string{
		"↑↓ move · enter select · esc cancel · type to search" + sortHint,
		"↑↓ move · enter select · esc cancel" + sortHint,
		"↑↓ enter esc" + strings.Replace(sortHint, " · tab sort", " · tab", 1),
	} {
		if f := p.st.dim.Render(keys); lipgloss.Width(f) <= p.width {
			return f
		}
	}
	return ansi.Truncate(p.st.dim.Render("↑↓ enter esc"), p.width, "…")
}

func truncate(s string, w int) string {
	if runewidth.StringWidth(s) <= w {
		return s
	}
	return runewidth.Truncate(s, w, "…")
}

// padRight pads a possibly styled string to a visible width.
func padRight(s string, w int) string {
	if n := w - lipgloss.Width(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// styles are created from a renderer bound to the menu's output, so colors
// match that terminal even when stdout is a pipe (`$(llmenu)`).
type styles struct {
	normal, selected, dim, header, prompt, cursorBlock, marker, good, match, preview lipgloss.Style
}

func newStyles(r *lipgloss.Renderer) styles {
	return styles{
		normal:      r.NewStyle(),
		selected:    r.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#ffffff"}).Background(lipgloss.AdaptiveColor{Light: "#d0d7ff", Dark: "#3b3f6b"}),
		dim:         r.NewStyle().Faint(true),
		header:      r.NewStyle().Bold(true),
		prompt:      r.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		cursorBlock: r.NewStyle().Reverse(true),
		marker:      r.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		good:        r.NewStyle().Foreground(lipgloss.Color("10")),
		match:       r.NewStyle().Underline(true).Bold(true),
		preview:     r.NewStyle().PaddingLeft(2).BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(lipgloss.Color("8")),
	}
}
