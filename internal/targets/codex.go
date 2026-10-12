package targets

import (
	"context"
	"errors"
	"net/url"
	"os/exec"
	"regexp"
	"slices"
	"strconv"

	"github.com/schowave/llmenu/internal/catalog"
)

func init() { register(codex{}) }

// codex launches Codex CLI with the gateway as provider, entirely via -c
// overrides; ~/.codex/config.toml stays untouched. Codex speaks the Responses
// API; LiteLLM translates it even for models that do not offer it natively
// (spike, ADR-0002).
type codex struct{}

func (codex) Name() string        { return "codex" }
func (codex) Description() string { return "Pick a model and start Codex CLI" }
func (codex) Binary() string      { return "codex" }

func (codex) HasModelArg(args []string) bool { return hasFlag(args, "-m", "--model") }

func (codex) Supports(m catalog.Model) (bool, string) {
	if slices.Contains(m.Endpoints, "/v1/responses") {
		return true, ""
	}
	return true, "gateway reports no Responses API; runs via LiteLLM's translation, Codex does not know the model"
}

// DefaultModel is codex.default from the configuration; none if unset.
func (codex) DefaultModel(env Env) string { return env.Config.Codex.Default }

var reProviderID = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func (codex) Command(ctx context.Context, m *catalog.Model, args []string, env Env) (*exec.Cmd, error) {
	base, err := url.JoinPath(env.BaseURL, "v1")
	if err != nil {
		return nil, err
	}
	id := reProviderID.ReplaceAllString(env.ProviderName(), "_")
	pre := []string{
		"-c", "model_provider=" + id,
		"-c", "model_providers." + id + "={name=" + strconv.Quote(id) +
			", base_url=" + strconv.Quote(base) +
			", env_key=" + strconv.Quote(KeyEnv) +
			`, wire_api="responses"}`,
	}
	switch {
	case m != nil:
		// As a config override instead of -m, so it also applies to `codex exec` etc.
		pre = append(pre, "-c", "model="+strconv.Quote(m.Name))
		if m.ContextTokens > 0 {
			pre = append(pre, "-c", "model_context_window="+strconv.Itoa(m.ContextTokens))
		}
		if m.MaxOutputTokens > 0 {
			pre = append(pre, "-c", "model_max_output_tokens="+strconv.Itoa(m.MaxOutputTokens))
		}
	case !hasFlag(args, "-m", "--model"):
		return nil, errors.New("no model: pass -m <model>, run in a terminal or llmenu config set codex.default <model>")
	}

	cmd := exec.CommandContext(ctx, "codex", append(pre, args...)...)
	cmd.Env = env.childEnv(KeyEnv + "=" + env.APIKey)
	return cmd, nil
}
