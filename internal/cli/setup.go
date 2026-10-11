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

var sections = []string{"gateway", "claude", "benchmarks", "sync"}

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup [" + strings.Join(sections, "|") + "]",
		Short: "Prompt for settings; repeatable, Enter keeps the current value",
		Long: `Without a section, all sections are walked through. Defaults are the current
values, otherwise gateway suggestions. Single values without a dialog: llmenu config set.`,
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		ValidArgs: sections,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isTerminal(os.Stdin) {
				return errors.New("setup needs a terminal; without a dialog: llmenu config set")
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
	if only == "" || only == "gateway" {
		if err := setupGateway(ctx, cfg); err != nil {
			return err
		}
	}
	if only == "" || only == "claude" {
		env, err := loadEnv(ctx)
		if err != nil {
			return err
		}
		models = env.Models
		if err := setupClaude(cfg, models); err != nil {
			return err
		}
	}
	if only == "" || only == "benchmarks" {
		if err := setupBenchmarks(); err != nil {
			return err
		}
	}
	if only == "" || only == "sync" {
		if err := setupSync(cfg); err != nil {
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
	u := cfg.Gateway.URL
	for {
		var key string
		keyHelp := "Stored in the operating system's keyring."
		if keySource != "" {
			keyHelp = "Leave empty to keep the existing key (" + keySource + ")."
		}
		err := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Gateway URL").
				Description("Base URL of the LiteLLM proxy, e.g. https://llm.example.com/").
				Value(&u).Validate(validateURL),
			huh.NewInput().Title("Gateway key").Description(keyHelp).
				EchoMode(huh.EchoModePassword).Value(&key).
				Validate(func(s string) error {
					if s == "" && keySource == "" {
						return errors.New("key missing")
					}
					return nil
				}),
		)).Run()
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
		if err := huh.NewConfirm().Title("Enter again?").Affirmative("Yes").Negative("Cancel").
			Value(&retry).Run(); err != nil || !retry {
			return errors.New("gateway not set up")
		}
	}
	cfg.Gateway.URL = u

	name := cfg.Gateway.NameOrDerived()
	err = huh.NewInput().Title("Provider name").
		Description("llmenu registers the gateway in pi, opencode, etc. under this name.").
		Value(&name).Validate(func(s string) error {
		if strings.TrimSpace(s) == "" {
			return errors.New("name missing")
		}
		return nil
	}).Run()
	if err != nil {
		return err
	}
	// Only store what differs from the derived name; that way the name follows a new URL.
	cfg.Gateway.Name = ""
	if name != config.NameFromURL(u) {
		cfg.Gateway.Name = name
	}
	return save(cfg)
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
	fields = append(fields,
		huh.NewSelect[string]().Title("Default model").
			Description("The menu cursor starts here; without a terminal it is used directly.").
			Options(defOpts...).Value(&def),
		huh.NewConfirm().Title("Automatic compaction?").
			Description("Off: context grows up to the limit, which costs considerably more with large contexts.").
			Affirmative("On").Negative("Off").Value(&autoCompact),
	)
	if err := huh.NewForm(huh.NewGroup(fields...)).Run(); err != nil {
		return err
	}

	for _, slot := range config.Slots {
		// An accepted suggestion stays unset and so follows the gateway.
		if c.Slot(slot) == "" && *values[slot] == catalog.Suggest(models, slot) {
			continue
		}
		c.SetSlot(slot, *values[slot])
		if i := slices.IndexFunc(models, func(m catalog.Model) bool { return m.Name == *values[slot] }); i >= 0 && !targets.IsClaude(models[i]) {
			fmt.Fprintf(os.Stderr, "Warning: %s is not a Claude model; slot %s runs e.g. subagents on it.\n", *values[slot], slot)
		}
	}
	// Do not pin defaults, so they can change with llmenu.
	c.Default = ""
	if def != "opus" {
		c.Default = def
	}
	c.AutoCompact = nil
	if !autoCompact {
		c.AutoCompact = &autoCompact
	}
	return save(cfg)
}

// slotOptions offers all models including aliases for a slot, matching ones
// first. A configured model the gateway no longer knows stays selectable.
func slotOptions(models []catalog.Model, slot, current string) []huh.Option[string] {
	var opts []huh.Option[string]
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
	help := "Optional, for `llmenu bench`. Free key: https://artificialanalysis.ai. Leave empty to skip."
	if source != "" {
		help = "Leave empty to keep the existing key (" + source + ")."
	}
	var key string
	if err := huh.NewInput().Title("Artificial Analysis key").Description(help).
		EchoMode(huh.EchoModePassword).Value(&key).Run(); err != nil {
		return err
	}
	if key == "" {
		return nil
	}
	return secrets.Set(secrets.ArtificialAnalysis, key)
}

func setupSync(cfg *config.Config) error {
	var opts []huh.Option[string]
	selected := cfg.Sync.Targets
	for _, t := range targets.All() {
		if _, ok := t.(targets.Syncer); !ok {
			continue
		}
		label := t.Name()
		_, installed := targets.Installed(t)
		if !installed {
			label += "  (not installed)"
		}
		opts = append(opts, huh.NewOption(label, t.Name()))
		if cfg.Sync.Targets == nil && installed {
			selected = append(selected, t.Name())
		}
	}
	if err := huh.NewMultiSelect[string]().Title("Sync targets").
		Description("`llmenu sync` writes the gateway's models into these tools. Space selects.").
		Options(opts...).Value(&selected).Run(); err != nil {
		return err
	}
	cfg.Sync.Targets = selected
	return save(cfg)
}
