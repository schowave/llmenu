package targets

import (
	"context"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/config"
)

func init() { register(claude{}) }

// claude launches Claude Code against the gateway. Claude Code has no gateway
// configuration that could be kept separate from the subscription, hence
// launch only.
type claude struct{}

func (claude) Name() string        { return "claude" }
func (claude) Description() string { return "launch Claude Code against the gateway" }
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

	cmd.Env = os.Environ()
	for _, k := range slices.Sorted(maps.Keys(vars)) {
		cmd.Env = append(cmd.Env, k+"="+vars[k])
	}
	return cmd, nil
}
