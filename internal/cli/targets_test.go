package cli

import (
	"slices"
	"testing"

	"github.com/schowave/llmenu/internal/targets"
)

// TestTargetTables fails for a target missing from the hand-maintained
// tables; a new target needs an entry in each of them.
func TestTargetTables(t *testing.T) {
	for _, tg := range targets.All() {
		name := tg.Name()
		if _, ok := toolLabels[name]; !ok {
			t.Errorf("%s: no toolLabels entry (tools.go)", name)
		}
		if !isLauncher(tg) {
			continue
		}
		if _, ok := launchLabels[name]; !ok {
			t.Errorf("%s: no launchLabels entry (default.go)", name)
		}
		if _, ok := launchExamples[name]; !ok {
			t.Errorf("%s: no launchExamples entry (run.go)", name)
		}
		if _, ok := modelFlags[name]; !ok {
			t.Errorf("%s: no modelFlags entry (complete.go)", name)
		}
		if !slices.Contains(standIns, name) {
			t.Errorf("%s: no stand-in for the scripts (standIns in script_test.go)", name)
		}
	}
}
