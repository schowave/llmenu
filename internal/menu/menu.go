// Package menu shows the interactive model picker. The menu draws directly to
// the terminal (/dev/tty or the console), so stdout stays free for `$(llmenu)`.
package menu

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ktr0731/go-fuzzyfinder"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/table"
)

// ErrAborted reports that the selection was aborted with Esc or Ctrl-C.
var ErrAborted = errors.New("selection aborted")

// Options controls the menu.
type Options struct {
	Note      string                     // note at the bottom of the preview, e.g. the benchmark source
	Preselect int                        // index of the model the cursor starts on; -1 for none
	Hint      func(catalog.Model) string // per-model hint, marks the row with "!"
}

// Pick lets the user choose a model and returns its index.
func Pick(models []catalog.Model, opt Options) (int, error) {
	if len(models) == 0 {
		return -1, errors.New("no models to choose from")
	}
	header, rows, preview := view(models, opt)

	i, err := fuzzyfinder.Find(models,
		func(i int) string { return rows[i] },
		fuzzyfinder.WithPromptString("model> "),
		fuzzyfinder.WithHeader(header), // single line, go-fuzzyfinder cannot do more
		fuzzyfinder.WithPreselected(func(i int) bool { return i == opt.Preselect }),
		fuzzyfinder.WithPreviewWindow(func(i, _, _ int) string { return preview(i) }),
	)
	if errors.Is(err, fuzzyfinder.ErrAbort) {
		return -1, ErrAborted
	}
	return i, err
}

// view returns what the menu shows: the table header, one row per model and
// the preview for the model at index i (-1: none). It needs no terminal.
func view(models []catalog.Model, opt Options) (header string, rows []string, preview func(i int) string) {
	hint := func(catalog.Model) string { return "" }
	if opt.Hint != nil {
		hint = opt.Hint
	}
	header, rows = table.Format(models, table.Options{Mark: func(m catalog.Model) string {
		if hint(m) != "" {
			return "!"
		}
		return ""
	}})
	preview = func(i int) string {
		if i < 0 || i >= len(models) {
			return ""
		}
		d := Details(models[i], hint(models[i]))
		if opt.Note != "" {
			d += "\n" + opt.Note + "\n"
		}
		return d
	}
	return header, rows, preview
}

// Details describes a model for the preview.
func Details(m catalog.Model, hint string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", m.Name)
	if hint != "" {
		fmt.Fprintf(&b, "! %s\n\n", hint)
	}
	fmt.Fprintf(&b, "Target model %s\n", m.Target)
	fmt.Fprintf(&b, "Context      %d tokens\n", m.ContextTokens)
	fmt.Fprintf(&b, "Max. output  %d tokens\n", m.MaxOutputTokens)
	fmt.Fprintf(&b, "Vision       %s\n", yesNo(m.Vision))
	fmt.Fprintf(&b, "Reasoning    %s\n", yesNo(m.Reasoning && !m.NoThinking))
	fmt.Fprintf(&b, "\n$ per million tokens\n")
	fmt.Fprintf(&b, "  Input        %8.3f\n  Output       %8.3f\n", m.InputPrice, m.OutputPrice)
	fmt.Fprintf(&b, "  Cache-Read   %8.3f\n  Cache-Write  %8.3f\n", m.CacheReadPrice, m.CacheWritePrice)
	if len(m.Endpoints) > 0 {
		fmt.Fprintf(&b, "\nEndpoints\n  %s\n", strings.Join(m.Endpoints, "\n  "))
	}
	if m.Bench != nil {
		fmt.Fprintf(&b, "\nBenchmarks (%s)\n", m.Bench.Slug)
		for _, v := range []struct {
			label string
			value *float64
			unit  string
		}{
			{"Intelligence", m.Bench.Intelligence, ""},
			{"Terminal-B. 4", m.Bench.TerminalBench, " %"},
			{"Coding", m.Bench.Coding, ""},
		} {
			if v.value != nil {
				fmt.Fprintf(&b, "  %-13s %6.1f%s\n", v.label, *v.value, v.unit)
			}
		}
	}
	return b.String()
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
