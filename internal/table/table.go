// Package table formats models as text rows, shared by `llmenu list` and
// the menu.
package table

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/schowave/llmenu/internal/catalog"
)

// Options controls the rendering.
type Options struct {
	ShowAliasTarget bool                       // show aliases as "name -> target"
	Mark            func(catalog.Model) string // optional one-character mark before the name
}

// Format returns the header and one row per model with equal-width columns.
// The benchmark columns only appear if at least one model has values.
func Format(models []catalog.Model, opt Options) (header string, rows []string) {
	labels := make([]string, len(models))
	marks := make([]string, len(models))
	width, withBench, withMark := len("MODEL"), false, false
	for i, m := range models {
		labels[i] = m.Name
		if opt.ShowAliasTarget && m.IsAlias() {
			labels[i] += " -> " + catalog.ShortTarget(m.Target)
		}
		if opt.Mark != nil {
			marks[i] = opt.Mark(m)
			withMark = withMark || marks[i] != ""
		}
		width = max(width, utf8.RuneCountInString(labels[i]))
		withBench = withBench || m.Bench != nil
	}

	var b strings.Builder
	if withMark {
		b.WriteString("  ")
	}
	fmt.Fprintf(&b, "%-*s %8s %9s %9s", width, "MODEL", "CONTEXT", "$ IN", "$ OUT")
	if withBench {
		fmt.Fprintf(&b, " %6s %6s %6s", "INTEL", "TB4", "CODE")
	}
	header = b.String()

	rows = make([]string, len(models))
	for i, m := range models {
		b.Reset()
		if withMark {
			fmt.Fprintf(&b, "%-1s ", marks[i])
		}
		fmt.Fprintf(&b, "%-*s %8s %9.2f %9.2f", width, labels[i], Tokens(m.ContextTokens), m.InputPrice, m.OutputPrice)
		if withBench {
			var bench catalog.Bench
			if m.Bench != nil {
				bench = *m.Bench
			}
			fmt.Fprintf(&b, " %6s %6s %6s", score(bench.Intelligence), score(bench.TerminalBench), score(bench.Coding))
		}
		rows[i] = b.String()
	}
	return header, rows
}

func score(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%.1f", *v)
}

// Tokens formats a token count compactly: 200000 -> "200k", 1000000 -> "1M",
// 1048576 -> "1.05M", unknown (0) -> "-".
func Tokens(n int) string {
	switch {
	case n <= 0:
		return "-"
	case n < 1_000_000:
		return fmt.Sprintf("%dk", n/1000)
	}
	s := strconv.FormatFloat(float64(n)/1e6, 'f', 2, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s + "M"
}
