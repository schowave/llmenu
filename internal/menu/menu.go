// Package menu shows the interactive model picker: a table with search, best
// model on top, sortable with Tab, and a preview of the selected model.
package menu

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/table"
)

// ErrAborted reports that the selection was aborted with Esc or Ctrl-C.
var ErrAborted = errors.New("selection aborted")

// Options controls the menu.
type Options struct {
	Note      string                     // note in the preview, e.g. the benchmark source
	Preselect int                        // index of the model the cursor starts on; -1 for the first
	Hint      func(catalog.Model) string // per-model note, shown in the preview
	Metric    string                     // benchmark the models are sorted by (bench.metric)
	Project   string                     // active project, shown above the table; empty: the main key
}

// Pick lets the user choose a model and returns its index. The menu runs in
// the alternate screen, reads from the terminal and draws to stderr, so
// stdout stays free for `$(llmenu)`.
func Pick(models []catalog.Model, opt Options) (int, error) {
	if len(models) == 0 {
		return -1, errors.New("no models to choose from")
	}
	p := newPicker(models, opt, newStyles(lipgloss.NewRenderer(os.Stderr)))
	final, err := tea.NewProgram(p, tea.WithAltScreen(), tea.WithInputTTY(), tea.WithOutput(os.Stderr)).Run()
	if err != nil {
		return -1, err
	}
	if fp := final.(*picker); !fp.aborted && fp.chosen >= 0 {
		return fp.chosen, nil
	}
	return -1, ErrAborted
}

// Details describes a model for the preview.
func Details(m catalog.Model, hint string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", m.Name)
	if m.Target != "" && m.Target != m.Name {
		fmt.Fprintf(&b, "→ %s\n", m.Target)
	}
	if hint != "" {
		fmt.Fprintf(&b, "\nNote: %s\n", hint)
	}
	fmt.Fprintf(&b, "\nContext  %s · output %s\n", table.Tokens(m.ContextTokens), table.Tokens(m.MaxOutputTokens))
	fmt.Fprintf(&b, "Vision %s · reasoning %s\n", yesNo(m.Vision), yesNo(m.Reasoning && !m.NoThinking))
	fmt.Fprintf(&b, "\n$ per 1M tokens\n  in %s · out %s\n  cache read %s · write %s\n",
		money(m.InputPrice), money(m.OutputPrice), money(m.CacheReadPrice), money(m.CacheWritePrice))
	if len(m.Endpoints) > 0 {
		fmt.Fprintf(&b, "\nEndpoints  %s\n", strings.Join(m.Endpoints, ", "))
	}
	if m.Bench != nil {
		fmt.Fprintf(&b, "\nBenchmarks (%s)\n", m.Bench.Slug)
		for _, v := range []struct {
			label string
			value *float64
			unit  string
		}{
			{"Intelligence  ", m.Bench.Intelligence, ""},
			{"Terminal-Bench", m.Bench.TerminalBench, " %"},
			{"Coding        ", m.Bench.Coding, ""},
		} {
			if v.value != nil {
				fmt.Fprintf(&b, "  %s %5.1f%s\n", v.label, *v.value, v.unit)
			}
		}
	}
	return b.String()
}

// money formats a price; prices below a cent keep their precision.
func money(v float64) string {
	if v != 0 && v < 0.01 {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return fmt.Sprintf("%.2f", v)
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
