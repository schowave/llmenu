package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/config"
	"github.com/schowave/llmenu/internal/gateway"
	"github.com/schowave/llmenu/internal/secrets"
	"github.com/schowave/llmenu/internal/targets"
)

var sections = []string{"gateway", "tools", "claude", "codex", "benchmarks"}

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup [" + strings.Join(sections, "|") + "]",
		Short: "Set up gateway, key, your coding tools and their defaults",
		Long: `Walks through all sections, or only the given one:
  gateway     URL and key of the LiteLLM gateway (checked right away)
  tools       which coding tools you use (Claude Code, Codex CLI, opencode, pi)
  claude      models for Claude Code's opus/sonnet/haiku slots, default, compaction
  codex       default model for Codex CLI
  benchmarks  free Artificial Analysis key for llmenu bench

The claude and codex sections are only asked for the tools you use.

Run it again any time: Enter keeps the current value. To change a single
value without the dialog, use llmenu config set.

LLMENU_ACCESSIBLE=1 asks line by line instead of drawing forms, e.g. for
screen readers; the answers can then also come from stdin.`,
		Example: `  llmenu setup
  llmenu setup claude`,
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		ValidArgs: sections,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !canPrompt() {
				return errors.New("setup needs a terminal (or LLMENU_ACCESSIBLE=1); without a dialog: llmenu config set")
			}
			err := runSetup(cmd.Context(), args)
			if errors.Is(err, huh.ErrUserAborted) {
				return exitCodeError{130}
			}
			return err
		},
	}
}

func runSetup(ctx context.Context, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	only := ""
	if len(args) == 1 {
		only = args[0]
	}

	var models []catalog.Model
	gatewayModels := func() ([]catalog.Model, error) {
		if models == nil {
			env, err := loadEnv(ctx)
			if err != nil {
				return nil, err
			}
			models = env.Models
		}
		return models, nil
	}
	// A section runs when asked for by name, or in a full run when relevant.
	want := func(section string, relevant bool) bool {
		return only == section || (only == "" && relevant)
	}

	if want("gateway", true) {
		if err := setupGateway(ctx, cfg); err != nil {
			return err
		}
	}
	if want("tools", true) {
		if err := setupTools(cfg); err != nil {
			return err
		}
	}
	if want("claude", toolEnabled(cfg, "claude")) {
		m, err := gatewayModels()
		if err != nil {
			return err
		}
		if err := setupClaude(cfg, m); err != nil {
			return err
		}
	}
	if want("codex", toolEnabled(cfg, "codex")) {
		m, err := gatewayModels()
		if err != nil {
			return err
		}
		if err := setupCodex(cfg, m); err != nil {
			return err
		}
	}
	if want("benchmarks", true) {
		if err := setupBenchmarks(); err != nil {
			return err
		}
	}
	return nil
}

func save(cfg *config.Config) error {
	changed, err := cfg.Save()
	if err != nil {
		return err
	}
	if changed {
		p, _ := config.Path()
		fmt.Fprintln(os.Stderr, "Saved:", p)
	}
	return nil
}

// setupGateway prompts for URL and key and checks both right away with /model/info.
func setupGateway(ctx context.Context, cfg *config.Config) error {
	_, keySource, err := secrets.Get(secrets.Gateway)
	if err != nil {
		return err
	}
	const urlHelp = "Base URL of the LiteLLM proxy, e.g. https://llm.example.com/"
	u := cfg.Gateway.URL
	for {
		var key string
		keyHelp := "Stored in the operating system's keyring."
		if keySource != "" {
			keyHelp = "Leave empty to keep the existing key (" + keySource + ")."
		}
		err := runPrompt(asForm{huh.NewForm(huh.NewGroup(
			// The example is a placeholder, not a value: nothing to delete first.
			huh.NewInput().Title(titled("Gateway URL", urlHelp)).Description(urlHelp).
				Placeholder("https://llm.example.com/").
				Value(&u).Validate(enterKeeps(&u, validateURL)),
			huh.NewInput().Title(titled("Gateway key", keyHelp)).Description(keyHelp).
				EchoMode(secretEcho()).Value(&key).
				Validate(func(s string) error {
					if s == "" && keySource == "" {
						return errors.New("key missing")
					}
					return nil
				}),
		))})
		if err != nil {
			return err
		}

		test := key
		if test == "" {
			test, _, _ = secrets.Get(secrets.Gateway)
		}
		models, err := gateway.Client{BaseURL: u, APIKey: test}.Models(ctx)
		if err == nil {
			fmt.Fprintf(os.Stderr, "Gateway reachable, %d chat models.\n", len(models))
			if key != "" {
				if err := secrets.Set(secrets.Gateway, key); err != nil {
					return err
				}
			}
			break
		}
		fmt.Fprintln(os.Stderr, "Gateway check failed:", err)
		retry := true
		if err := runPrompt(huh.NewConfirm().Title("Enter again?").Affirmative("Yes").Negative("Cancel").
			Value(&retry)); err != nil || !retry {
			return errors.New("gateway not set up")
		}
	}
	cfg.Gateway.URL = u

	name := cfg.Gateway.NameOrDerived()
	const nameHelp = "llmenu registers the gateway in pi, opencode, etc. under this name."
	err = runPrompt(huh.NewInput().Title(titled("Provider name", nameHelp)).Description(nameHelp).
		Value(&name).Validate(enterKeeps(&name, func(s string) error {
		if strings.TrimSpace(s) == "" {
			return errors.New("name missing")
		}
		return nil
	})))
	if err != nil {
		return err
	}
	applyGatewayName(cfg, name)
	return save(cfg)
}

// applyGatewayName stores the provider name only if it differs from the one
// derived from the URL; that way the name follows a new URL.
func applyGatewayName(cfg *config.Config, name string) {
	cfg.Gateway.Name = ""
	if name != config.NameFromURL(cfg.Gateway.URL) {
		cfg.Gateway.Name = name
	}
}

func validateURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("http(s) URL expected")
	}
	return nil
}

// setupClaude prompts for the slots, the default and compaction.
func setupClaude(cfg *config.Config, models []catalog.Model) error {
	c := &cfg.Claude
	values := map[string]*string{}
	var fields []huh.Field
	for _, slot := range config.Slots {
		v := c.Slot(slot)
		if v == "" {
			v = catalog.Suggest(models, slot)
		}
		values[slot] = &v
		fields = append(fields, huh.NewSelect[string]().
			Title("Model for "+slot).
			Description("-latest aliases follow new versions without rerunning setup. / filters.").
			Options(slotOptions(models, slot, v)...).
			Filtering(false).Height(10).
			Value(&v))
	}

	def := c.DefaultOrOpus()
	defOpts := huh.NewOptions(config.Slots...)
	if !slices.Contains(config.Slots, def) {
		defOpts = append(defOpts, huh.NewOption(def, def))
	}
	autoCompact := c.AutoCompactEnabled()
	const compactHelp = "Off: context grows up to the limit, which costs considerably more with large contexts."
	fields = append(fields,
		huh.NewSelect[string]().Title("Default model").
			Description("The menu cursor starts here; without a terminal it is used directly.").
			Options(defOpts...).Value(&def),
		huh.NewConfirm().Title(titled("Automatic compaction?", compactHelp)).Description(compactHelp).
			Affirmative("On").Negative("Off").Value(&autoCompact),
	)
	if err := runPrompt(asForm{huh.NewForm(huh.NewGroup(fields...))}); err != nil {
		return err
	}

	answers := map[string]string{}
	for _, slot := range config.Slots {
		answers[slot] = *values[slot]
	}
	for _, w := range applyClaude(c, models, answers, def, autoCompact) {
		fmt.Fprintln(os.Stderr, "Warning:", w)
	}
	return save(cfg)
}

// applyClaude stores the answers of the claude section. Accepted suggestions
// and defaults stay unset, so they follow the gateway and llmenu. It returns
// warnings for non-Claude models in a slot.
func applyClaude(c *config.Claude, models []catalog.Model, slots map[string]string, def string, autoCompact bool) []string {
	var warnings []string
	for _, slot := range config.Slots {
		v := slots[slot]
		if c.Slot(slot) == "" && v == catalog.Suggest(models, slot) {
			continue
		}
		c.SetSlot(slot, v)
		if i := slices.IndexFunc(models, func(m catalog.Model) bool { return m.Name == v }); i >= 0 && !targets.IsClaude(models[i]) {
			warnings = append(warnings, fmt.Sprintf("%s is not a Claude model; slot %s runs e.g. subagents on it.", v, slot))
		}
	}
	c.Default = ""
	if def != "opus" {
		c.Default = def
	}
	c.AutoCompact = nil
	if !autoCompact {
		c.AutoCompact = &autoCompact
	}
	return warnings
}

// slotOptions offers all models including aliases for a slot, matching ones
// first. A configured model the gateway no longer knows stays selectable.
func slotOptions(models []catalog.Model, slot, current string) []huh.Option[string] {
	var opts []huh.Option[string]
	// Without a suggestion Enter must not silently pick some model; leaving
	// the slot unset is offered first.
	if current == "" && catalog.Suggest(models, slot) == "" {
		opts = append(opts, huh.NewOption("(not set: Claude Code uses its built-in name, which the gateway may not know)", ""))
	}
	if current != "" && !slices.ContainsFunc(models, func(m catalog.Model) bool { return m.Name == current }) {
		opts = append(opts, huh.NewOption(current+"  (no longer in gateway)", current))
	}
	for _, m := range catalog.ForSlot(models, slot) {
		label := m.Name
		if m.IsAlias() {
			label += " -> " + catalog.ShortTarget(m.Target)
		}
		if m.IsLatestAlias() {
			label += "  (follows new versions)"
		}
		if !targets.IsClaude(m) {
			label += "  ! not a Claude model"
		}
		opts = append(opts, huh.NewOption(label, m.Name))
	}
	return opts
}

func setupBenchmarks() error {
	_, source, err := secrets.Get(secrets.ArtificialAnalysis)
	if err != nil {
		return err
	}
	help := "Optional, for `llmenu bench`. Free key: " + aaKeyURL + ". Leave empty to skip."
	if source != "" {
		help = "Leave empty to keep the existing key (" + source + ")."
	}
	var key string
	if err := runPrompt(huh.NewInput().Title(titled("Artificial Analysis key", help)).Description(help).
		EchoMode(secretEcho()).Value(&key)); err != nil {
		return err
	}
	if key == "" {
		return nil
	}
	return secrets.Set(secrets.ArtificialAnalysis, key)
}
