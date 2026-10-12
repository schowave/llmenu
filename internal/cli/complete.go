package cli

import (
	"cmp"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/config"
	"github.com/schowave/llmenu/internal/secrets"
	"github.com/schowave/llmenu/internal/state"
	"github.com/schowave/llmenu/internal/targets"
)

// Shell completion. Model names come from the cache that every command
// talking to the gateway refreshes, so Tab never waits for the network.

func cacheModelNames(models []catalog.Model) {
	names := make([]string, 0, len(models))
	for _, m := range models {
		names = append(names, m.Name)
	}
	slices.Sort(names)
	_ = state.SaveModels(names) // completion is a convenience; never fail a command for it
}

// matching keeps the candidates that start with what has been typed and
// have not been given already.
func matching(candidates []string, toComplete string, given []string) []string {
	var out []string
	for _, c := range candidates {
		if strings.HasPrefix(c, toComplete) && !slices.Contains(given, c) {
			out = append(out, c)
		}
	}
	return out
}

func targetNames(keep func(targets.Target) bool) []string {
	var out []string
	for _, t := range targets.All() {
		if keep(t) {
			out = append(out, t.Name())
		}
	}
	return out
}

func isSyncer(t targets.Target) bool   { _, ok := t.(targets.Syncer); return ok }
func isLauncher(t targets.Target) bool { _, ok := t.(targets.Launcher); return ok }

// completeArgs completes positional arguments from a fixed list.
func completeArgs(list func() []string) cobra.CompletionFunc {
	return func(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return matching(list(), toComplete, args), cobra.ShellCompDirectiveNoFileComp
	}
}

// completeFirst completes only the first positional argument.
func completeFirst(list func() []string) cobra.CompletionFunc {
	return func(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return matching(list(), toComplete, nil), cobra.ShellCompDirectiveNoFileComp
	}
}

func secretNames() []string {
	var out []string
	for n := range secrets.Names {
		out = append(out, n)
	}
	for _, p := range projectNames() {
		out = append(out, secrets.ProjectKey(p))
	}
	slices.Sort(out)
	return out
}

// projectNames come from the config, so Tab never touches the keyring.
func projectNames() []string {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	return cfg.Gateway.Projects
}

// configKeys are all fixed keys plus the prefixes for maps.
func configKeys() []string {
	var out []string
	for _, k := range config.Keys {
		out = append(out, k.Name)
	}
	return append(out, "claude.env.", "bench.map.")
}

// completeConfigKey completes the key; keys ending in "." need more input.
func completeConfigKey(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if strings.HasPrefix(toComplete, "bench.map.") {
		var out []string
		for _, m := range state.LoadModels() {
			out = append(out, "bench.map."+m)
		}
		return matching(out, toComplete, nil), cobra.ShellCompDirectiveNoFileComp
	}
	keys := matching(configKeys(), toComplete, nil)
	directive := cobra.ShellCompDirectiveNoFileComp
	if len(keys) == 1 && strings.HasSuffix(keys[0], ".") {
		directive |= cobra.ShellCompDirectiveNoSpace
	}
	return keys, directive
}

// completeConfigSet completes the key, then a value that fits it.
func completeConfigSet(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return completeConfigKey(cmd, args, toComplete)
	}
	if len(args) > 1 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var values []string
	switch args[0] {
	case "claude.default":
		values = append(slices.Clone(config.Slots), state.LoadModels()...)
	case "claude.opus", "claude.sonnet", "claude.haiku", "codex.default":
		values = state.LoadModels()
	case "bench.metric":
		values = config.Metrics
	case "gateway.project":
		values = projectNames()
	case "claude.auto_compact", "claude.statusline":
		values = []string{"true", "false"}
	case "tools":
		values = append(targetNames(func(targets.Target) bool { return true }), "none")
	}
	return matching(values, toComplete, nil), cobra.ShellCompDirectiveNoFileComp
}

// modelFlags are the flags with which each launch target takes a model.
var modelFlags = map[string][]string{
	"claude":   {"--model"},
	"codex":    {"-m", "--model"},
	"opencode": {"-m", "--model"},
}

// completeLaunch completes model names after the tool's model flag and
// leaves everything else to the shell (files), since arguments go to the tool.
func completeLaunch(name string) cobra.CompletionFunc {
	return func(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		flags := modelFlags[name]
		for _, f := range flags {
			if v, ok := strings.CutPrefix(toComplete, f+"="); ok {
				var out []string
				for _, m := range matching(launchModels(name), v, nil) {
					out = append(out, f+"="+m)
				}
				return out, cobra.ShellCompDirectiveNoFileComp
			}
		}
		if len(args) > 0 && slices.Contains(flags, args[len(args)-1]) {
			return matching(launchModels(name), toComplete, nil), cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveDefault
	}
}

// launchModels are the cached model names in the form the tool expects;
// opencode wants provider/model.
func launchModels(name string) []string {
	models := state.LoadModels()
	if name != "opencode" {
		return models
	}
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	// No key needed for the provider name, so the keyring is not touched on Tab.
	provider := targets.Env{BaseURL: cmp.Or(os.Getenv("LLMENU_BASE_URL"), cfg.Gateway.URL), Config: cfg}.ProviderName()
	out := make([]string, 0, len(models))
	for _, m := range models {
		out = append(out, provider+"/"+m)
	}
	return out
}

// completeRun completes the target, then behaves like the target's command.
func completeRun(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return matching(targetNames(isLauncher), toComplete, nil), cobra.ShellCompDirectiveNoFileComp
	}
	return completeLaunch(args[0])(cmd, args[1:], toComplete)
}
