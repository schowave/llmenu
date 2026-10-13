package cli

import (
	"slices"

	"github.com/charmbracelet/huh"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/config"
	"github.com/schowave/llmenu/internal/targets"
)

// installed is replaceable in tests.
var installed = func(t targets.Target) bool { _, ok := targets.Installed(t); return ok }

// defaultTools apply as long as no tools have been chosen in setup.
var defaultTools = []string{"claude"}

// enabledTools are the tools chosen in setup, or defaultTools (ADR-0003).
func enabledTools(cfg *config.Config) []string {
	if cfg.ToolsConfigured() {
		return cfg.ToolList()
	}
	return slices.Clone(defaultTools)
}

func toolEnabled(cfg *config.Config, name string) bool {
	return slices.Contains(enabledTools(cfg), name)
}

// toolLabels describe what llmenu does with each tool in the setup list.
var toolLabels = map[string]string{
	"claude":   "Claude Code  (llmenu claude starts it with a model)",
	"codex":    "Codex CLI    (llmenu codex starts it with a model)",
	"opencode": "opencode     (llmenu sync writes the models, llmenu opencode starts it)",
	"pi":       "pi           (llmenu sync writes the models into its config)",
}

func setupTools(cfg *config.Config) error {
	var opts []huh.Option[string]
	for _, t := range targets.All() {
		label := toolLabels[t.Name()]
		if label == "" {
			label = t.Name()
		}
		if !installed(t) {
			label += "  - not installed"
		}
		opts = append(opts, huh.NewOption(label, t.Name()))
	}
	selected := enabledTools(cfg)
	if err := runPrompt(huh.NewMultiSelect[string]().Title("Coding tools you use").
		Description("Setup only asks about these; llmenu sync writes into those that keep a model list.").
		Options(opts...).Value(&selected)); err != nil {
		return err
	}
	if selected == nil {
		selected = []string{} // deliberately none, not "unset"
	}
	cfg.SetTools(selected)
	// A default tool no longer in use is asked for again.
	if cfg.Launch != "" && cfg.Launch != config.NoLaunch && !slices.Contains(selected, cfg.Launch) {
		cfg.Launch = ""
	}
	return save(cfg)
}

// setupCodex asks for Codex CLI's default model. Without one, llmenu codex
// needs a terminal (menu) or -m.
func setupCodex(cfg *config.Config, models []catalog.Model) error {
	codex, _ := targets.Get("codex")
	l := codex.(targets.Launcher)
	def := cfg.Codex.Default
	opts := []huh.Option[string]{huh.NewOption("(no default: pick in the menu or pass -m)", "")}
	if def != "" && !slices.ContainsFunc(models, func(m catalog.Model) bool { return m.Name == def }) {
		opts = append(opts, huh.NewOption(def+"  (no longer in gateway)", def))
	}
	shown := catalog.Dedupe(models)
	catalog.SortByPrice(shown)
	for _, m := range shown {
		label := m.Name
		if _, hint := l.Supports(m); hint != "" {
			label += "  ! no Responses API"
		}
		opts = append(opts, huh.NewOption(label, m.Name))
	}
	if err := runPrompt(huh.NewSelect[string]().Title("Default model for Codex CLI").
		Description("Used without a terminal and as the cursor position in the menu.").
		Options(opts...).Height(10).Value(&def)); err != nil {
		return err
	}
	cfg.Codex.Default = def
	return save(cfg)
}
