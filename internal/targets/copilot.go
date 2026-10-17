package targets

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/tailscale/hujson"

	"github.com/schowave/llmenu/internal/catalog"
)

func init() { register(copilot{}) }

// copilotEnvPrefix covers Copilot CLI's BYOK variables. Inherited ones are
// dropped: a bearer token or key command from the shell would beat the
// gateway key. Likewise COPILOT_OFFLINE, so copilot.offline decides.
const (
	copilotEnvPrefix  = "COPILOT_PROVIDER_"
	copilotOfflineEnv = "COPILOT_OFFLINE"
)

// copilot launches GitHub Copilot CLI with the gateway as BYOK provider,
// entirely via COPILOT_PROVIDER_* variables; ~/.copilot stays untouched. A
// ~/.copilot/providers.json that declares providers takes precedence over
// the variables. With a BYOK provider Copilot needs no GitHub login.
type copilot struct{}

func (copilot) Name() string        { return "copilot" }
func (copilot) Description() string { return "Pick a model and start GitHub Copilot CLI" }
func (copilot) Binary() string      { return "copilot" }

func (copilot) HasModelArg(args []string) bool { return hasFlag(args, "--model") }

// Supports accepts every model: LiteLLM translates both provider types.
func (copilot) Supports(catalog.Model) (bool, string) { return true, "" }

// DefaultModel is copilot.default from the configuration; none if unset.
func (copilot) DefaultModel(env Env) string { return env.Config.Copilot.Default }

func (copilot) Command(ctx context.Context, m *catalog.Model, args []string, env Env) (*exec.Cmd, error) {
	typ := env.Config.Copilot.ProviderTypeOrDefault()
	// Copilot appends /chat/completions for openai, but /v1/messages for
	// anthropic.
	base := strings.TrimSuffix(env.BaseURL, "/")
	if typ == "openai" {
		var err error
		if base, err = url.JoinPath(env.BaseURL, "v1"); err != nil {
			return nil, err
		}
	}
	vars := []string{
		copilotEnvPrefix + "BASE_URL=" + base,
		copilotEnvPrefix + "TYPE=" + typ,
		copilotEnvPrefix + "API_KEY=" + env.APIKey,
	}
	switch {
	case m != nil:
		// As a variable instead of --model, so a --model passed through wins.
		vars = append(vars, "COPILOT_MODEL="+m.Name)
	case hasFlag(args, "--model"):
		// The passed model's limits and ID, if the gateway knows it.
		name := flagValue(args, "--model")
		if i := slices.IndexFunc(env.Models, func(g catalog.Model) bool { return g.Name == name }); i >= 0 {
			m = &env.Models[i]
		}
	default:
		return nil, errors.New("no model: pass --model <model>, run in a terminal or llmenu config set copilot.default <model>")
	}
	if m != nil {
		if m.ContextTokens > 0 {
			vars = append(vars, copilotEnvPrefix+"MAX_PROMPT_TOKENS="+strconv.Itoa(m.ContextTokens))
		}
		if m.MaxOutputTokens > 0 {
			vars = append(vars, copilotEnvPrefix+"MAX_OUTPUT_TOKENS="+strconv.Itoa(m.MaxOutputTokens))
		}
		// Copilot knows Claude models by their Anthropic ID (token counting,
		// agent settings); the gateway name still goes over the wire.
		if id := catalog.ShortTarget(m.Target); reClaudeID.MatchString(id) {
			vars = append(vars, copilotEnvPrefix+"MODEL_ID="+id)
		}
	}
	if env.Config.Copilot.OfflineEnabled() {
		vars = append(vars, copilotOfflineEnv+"=true")
	}

	cmd := exec.CommandContext(ctx, "copilot", args...)
	cmd.Env = slices.DeleteFunc(env.childEnv(), func(kv string) bool {
		return strings.HasPrefix(kv, copilotEnvPrefix) || strings.HasPrefix(kv, copilotOfflineEnv+"=")
	})
	cmd.Env = append(cmd.Env, vars...)
	return cmd, nil
}

// Check warns about a providers.json that declares providers or models:
// Copilot then ignores the gateway and model llmenu passes.
func (copilot) Check() []string {
	path := os.Getenv("COPILOT_PROVIDERS_CONFIG")
	if path == "" {
		home := os.Getenv("COPILOT_HOME")
		if home == "" {
			h, err := os.UserHomeDir()
			if err != nil {
				return nil
			}
			home = filepath.Join(h, ".copilot")
		}
		path = filepath.Join(home, "providers.json")
	}
	raw, err := os.ReadFile(path) //nolint:gosec // G304: Copilot's own config file, read only
	if err != nil {
		return nil
	}
	var f struct {
		Providers, Models map[string]any
	}
	if std, err := hujson.Standardize(raw); err != nil || json.Unmarshal(std, &f) != nil {
		return nil
	}
	if len(f.Providers)+len(f.Models) == 0 {
		return nil
	}
	return []string{path + " declares providers, so Copilot CLI ignores the gateway and model llmenu passes"}
}
