package catalog

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var reNumber = regexp.MustCompile(`[0-9]+`)

// version returns the numbers in the shortened target model in order:
// "claude-opus-4-6" yields [4 6], "claude-opus-5" yields [5].
func version(m Model) []int {
	var v []int
	for _, s := range reNumber.FindAllString(ShortTarget(m.Target), -1) {
		n, _ := strconv.Atoi(s)
		v = append(v, n)
	}
	return v
}

func hasToken(name, token string) bool {
	return slices.Contains(strings.Fields(Key(name, nil)), token)
}

// IsLatestAlias reports an alias that follows new versions, e.g. *-latest.
func (m Model) IsLatestAlias() bool { return m.IsAlias() && hasToken(m.Name, "latest") }

// ForSlot sorts the models for choosing a slot (e.g. "opus"): first -latest
// aliases for the slot, then the slot's other models, highest version first,
// then all others in their previous order.
func ForSlot(models []Model, slot string) []Model {
	rank := func(m Model) int {
		switch {
		case !hasToken(m.Target, slot):
			return 2
		case m.IsLatestAlias():
			return 0
		default:
			return 1
		}
	}
	out := slices.Clone(models)
	slices.SortStableFunc(out, func(a, b Model) int {
		ra, rb := rank(a), rank(b)
		if ra != rb || ra == 2 {
			return cmp.Compare(ra, rb)
		}
		if c := slices.Compare(version(b), version(a)); c != 0 {
			return c
		}
		if a.NoThinking != b.NoThinking {
			if a.NoThinking {
				return 1
			}
			return -1
		}
		if preferred(a, b) {
			return -1
		}
		if preferred(b, a) {
			return 1
		}
		return 0
	})
	return out
}

// Suggest proposes a model for a slot, or "" if the gateway offers none whose
// name contains the slot (ADR-0003).
func Suggest(models []Model, slot string) string {
	sorted := ForSlot(models, slot)
	if len(sorted) == 0 || !hasToken(sorted[0].Target, slot) {
		return ""
	}
	return sorted[0].Name
}
