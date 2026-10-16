package targets

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"

	"github.com/schowave/llmenu/internal/atomicfile"
	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/config"
	"github.com/schowave/llmenu/internal/state"
	"github.com/schowave/llmenu/internal/table"
)

func init() { register(claude{}) }

// claude launches Claude Code against the gateway. Claude Code has no gateway
// configuration that could be kept separate from the subscription, hence
// launch only.
type claude struct{}

func (claude) Name() string        { return "claude" }
func (claude) Description() string { return "Pick a model and start Claude Code" }
func (claude) Binary() string      { return "claude" }

func (claude) HasModelArg(args []string) bool { return hasFlag(args, "--model") }

func (claude) Supports(m catalog.Model) (bool, string) {
	if IsClaude(m) {
		return true, ""
	}
	return true, "not a Claude model, runs via the gateway's translation"
}

// IsClaude reports whether the target model is a Claude model.
func IsClaude(m catalog.Model) bool {
	return strings.Contains(strings.ToLower(m.Target), "claude")
}

// SlotModel returns a slot's model: the configured one or, if empty, the
// gateway suggestion (ADR-0003).
func SlotModel(env Env, slot string) string {
	if m := env.Config.Claude.Slot(slot); m != "" {
		return m
	}
	return catalog.Suggest(env.Models, slot)
}

// DefaultModel resolves claude.default: a slot name via the slot, otherwise
// it is a model name itself.
func (claude) DefaultModel(env Env) string {
	d := env.Config.Claude.DefaultOrOpus()
	if slices.Contains(config.Slots, d) {
		return SlotModel(env, d)
	}
	return d
}

// From this context size on, a model gets the suffix [1m] so Claude Code
// assumes a 1M context.
const claude1MThreshold = 900_000

// withContext appends [1m] if the gateway reports enough context for the model.
func withContext(name string, models []catalog.Model) string {
	i := slices.IndexFunc(models, func(m catalog.Model) bool { return m.Name == name })
	if i >= 0 && models[i].ContextTokens >= claude1MThreshold {
		return name + "[1m]"
	}
	return name
}

func (c claude) Command(ctx context.Context, m *catalog.Model, args []string, env Env) (*exec.Cmd, error) {
	if m != nil {
		args = append([]string{"--model", withContext(m.Name, env.Models)}, args...)
	}
	// Session settings only: ~/.claude stays untouched (ADR-0002). Own
	// --settings win, Claude Code takes only one.
	if !hasFlag(args, "--settings") {
		if s := sessionSettings(env); s != "" {
			args = append([]string{"--settings", settingsArg(s)}, args...)
		}
	}
	cmd := exec.CommandContext(ctx, "claude", args...)

	vars := maps.Clone(env.Config.Claude.EnvOrDefault())
	for _, slot := range config.Slots {
		if name := SlotModel(env, slot); name != "" {
			vars["ANTHROPIC_DEFAULT_"+strings.ToUpper(slot)+"_MODEL"] = withContext(name, env.Models)
		}
	}
	if d := env.Config.Claude.DefaultOrOpus(); slices.Contains(config.Slots, d) {
		vars["ANTHROPIC_MODEL"] = d
	} else {
		vars["ANTHROPIC_MODEL"] = withContext(d, env.Models)
	}
	if !env.Config.Claude.AutoCompactEnabled() {
		vars["DISABLE_AUTO_COMPACTION"] = "1"
	}
	vars["ANTHROPIC_BASE_URL"] = env.BaseURL
	vars["ANTHROPIC_AUTH_TOKEN"] = env.APIKey

	cmd.Env = env.childEnv()
	for _, k := range slices.Sorted(maps.Keys(vars)) {
		cmd.Env = append(cmd.Env, k+"="+vars[k])
	}
	return cmd, nil
}

// sessionSettings is the JSON for --settings: llmenu's status line
// (claude.statusline) and the gateway's models in /model (claude.picker).
// Empty if neither is on.
func sessionSettings(env Env) string {
	s := map[string]any{}
	if env.Config.Claude.StatusLine {
		s["statusLine"] = map[string]string{"type": "command", "command": "llmenu statusline"}
	}
	if env.Config.Claude.PickerEnabled() {
		if p := modelPicker(env); p != nil {
			s["modelPicker"] = p
		}
	}
	if len(s) == 0 {
		return ""
	}
	b, _ := json.Marshal(s) // maps of strings and structs: cannot fail
	return string(b)
}

// settingsArg returns the path of a file with the settings: inline, the
// model list could exceed Windows' command line limit (8191 characters
// through claude.cmd). The file is only rewritten when the settings change,
// as running sessions may reload it. If it cannot be written, the settings
// go inline.
func settingsArg(settings string) string {
	p, err := state.ClaudeSettingsPath()
	if err != nil {
		return settings
	}
	if b, err := os.ReadFile(p); err == nil && string(b) == settings {
		return p
	}
	if atomicfile.Write(p, []byte(settings), 0o600) != nil {
		return settings
	}
	return p
}

type pickerOption struct {
	Model       string `json:"model"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	BehavesAs   string `json:"behavesAs,omitempty"`
}

// modelPicker lists the gateway's models in /model, without aliases and
// duplicates and sorted like the menu. Picking one with Enter makes Claude
// Code save it as the default in ~/.claude/settings.json, where it fails
// without the gateway; /model fixes that (ADR-0002).
func modelPicker(env Env) map[string]any {
	models := catalog.Dedupe(env.Models)
	if len(models) == 0 {
		return nil
	}
	metric := env.Config.Bench.MetricOrDefault()
	catalog.Sort(models, metric)
	opts := make([]pickerOption, 0, len(models))
	for _, m := range models {
		o := pickerOption{
			Model:       withContext(m.Name, env.Models),
			Label:       m.Name,
			Description: pickerDescription(m, metric),
		}
		if id := catalog.ShortTarget(m.Target); reClaudeID.MatchString(id) {
			o.BehavesAs = id
		}
		opts = append(opts, o)
	}
	return map[string]any{"replaceBuiltInOptions": true, "options": opts}
}

// reClaudeID matches the Anthropic model IDs a shortened target can be used
// as for behavesAs, e.g. claude-opus-5-5 or claude-sonnet-4. Older names such
// as claude-3-5-sonnet need their date, which ShortTarget drops.
var reClaudeID = regexp.MustCompile(`^claude-(opus|sonnet|haiku|fable)-\d+(-\d+)?$`)

// metricName names the benchmark Model.Score uses for metric.
func metricName(metric string) string {
	switch metric {
	case "tb4":
		return "Terminal-Bench"
	case "code":
		return "coding"
	}
	return "intelligence"
}

// pickerDescription is a model's line in /model: context, prices and the
// benchmark the menu sorts by, e.g. "1M · $5.00 in / $25.00 out · intelligence 70.1".
func pickerDescription(m catalog.Model, metric string) string {
	var parts []string
	if m.ContextTokens > 0 {
		parts = append(parts, table.Tokens(m.ContextTokens))
	}
	switch {
	case m.NoPrice:
		parts = append(parts, "price unknown")
	case m.InputPrice == 0 && m.OutputPrice == 0:
		parts = append(parts, "free")
	default:
		price := usd(m, m.InputPrice) + " in / " + usd(m, m.OutputPrice) + " out"
		if len(m.Tiers) > 0 {
			price += ", more above " + table.Tokens(m.Tiers[0].AboveTokens)
		}
		parts = append(parts, price)
	}
	if v := m.Score(metric); v != nil {
		parts = append(parts, fmt.Sprintf("%s %.1f", metricName(metric), *v))
	}
	return strings.Join(parts, " · ")
}

func usd(m catalog.Model, v float64) string {
	if p := table.Price(m, v); p != "free" {
		return "$" + p
	}
	return "free"
}
