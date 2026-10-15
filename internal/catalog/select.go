package catalog

import (
	"cmp"
	"slices"
	"strings"
)

// IsAlias reports whether the gateway forwards the name to another model,
// e.g. aws/claude-opus-latest to claude-opus-5-5.
func (m Model) IsAlias() bool {
	return Key(m.Target, PickerStopwords) != Key(m.Name, PickerStopwords)
}

// group identifies a model for deduplication: the same model at the same
// provider (the target's prefix, e.g. bedrock/ or vertex_ai/), with or without
// thinking.
type group struct {
	provider   string
	key        string
	noThinking bool
}

func groupOf(m Model) group {
	provider, _, _ := strings.Cut(m.Target, "/")
	return group{provider, Key(m.Target, PickerStopwords), m.NoThinking}
}

// Dedupe hides aliases and keeps one of each set of duplicates (the same model
// at the same provider, e.g. several regions or deployments): the one with the
// largest context, on a tie the cheapest. The same model at different
// providers (e.g. aws/ and gcp/) and variants without thinking stay separate
// entries. The input order is preserved.
func Dedupe(models []Model) []Model {
	best := map[group]int{}
	for i, m := range models {
		if m.IsAlias() {
			continue
		}
		g := groupOf(m)
		if j, ok := best[g]; !ok || preferred(m, models[j]) {
			best[g] = i
		}
	}
	var out []Model
	for i, m := range models {
		if j, ok := best[groupOf(m)]; ok && j == i {
			out = append(out, m)
		}
	}
	return out
}

func preferred(a, b Model) bool {
	return cmp.Or(
		cmp.Compare(b.ContextTokens, a.ContextTokens),
		cmp.Compare(a.OutputPrice, b.OutputPrice),
		cmp.Compare(a.InputPrice, b.InputPrice),
		cmp.Compare(a.Name, b.Name),
	) < 0
}

// SortByPrice sorts most expensive first (output, then input price), then by name.
func SortByPrice(models []Model) {
	slices.SortStableFunc(models, func(a, b Model) int {
		return cmp.Or(
			cmp.Compare(b.OutputPrice, a.OutputPrice),
			cmp.Compare(b.InputPrice, a.InputPrice),
			cmp.Compare(a.Name, b.Name),
		)
	})
}

// Resolve returns the index of name in shown. If name is not in there because
// it is an alias or a hidden duplicate, it returns the index of the model shown
// in its place. -1 if there is none.
func Resolve(all, shown []Model, name string) int {
	if i := slices.IndexFunc(shown, func(m Model) bool { return m.Name == name }); i >= 0 {
		return i
	}
	j := slices.IndexFunc(all, func(m Model) bool { return m.Name == name })
	if j < 0 {
		return -1
	}
	g := groupOf(all[j])
	return slices.IndexFunc(shown, func(m Model) bool { return groupOf(m) == g })
}

// Score returns the value of a benchmark (intel, tb4, code), otherwise nil.
func (m Model) Score(metric string) *float64 {
	if m.Bench == nil {
		return nil
	}
	switch metric {
	case "tb4":
		return m.Bench.TerminalBench
	case "code":
		return m.Bench.Coding
	default:
		return m.Bench.Intelligence
	}
}

// SortByBench sorts by benchmark, best first; models without a value go last,
// among themselves most expensive first like SortByPrice.
func SortByBench(models []Model, metric string) {
	SortByPrice(models)
	slices.SortStableFunc(models, func(a, b Model) int {
		sa, sb := a.Score(metric), b.Score(metric)
		switch {
		case sa == nil && sb == nil:
			return 0
		case sa == nil:
			return 1
		case sb == nil:
			return -1
		}
		return cmp.Compare(*sb, *sa)
	})
}
