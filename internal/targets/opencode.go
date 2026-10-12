package targets

import (
	"context"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/schowave/llmenu/internal/catalog"
)

func init() { register(opencode{}) }

// opencode maintains its own provider in the global opencode configuration.
// opencode can only read the key from an environment variable ({env:…}),
// so there is also `llmenu opencode`, which sets it at launch.
type opencode struct {
	path string // tests only
}

func (opencode) Name() string { return "opencode" }
func (opencode) Description() string {
	return "Start opencode with the gateway key"
}
func (opencode) Binary() string { return "opencode" }

// ConfigPath prefers an existing opencode.jsonc, otherwise opencode.json.
// TODO: check the path on Windows.
func (o opencode) ConfigPath() (string, error) {
	if o.path != "" {
		return o.path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "opencode")
	if p := filepath.Join(dir, "opencode.jsonc"); fileExists(p) {
		return p, nil
	}
	return filepath.Join(dir, "opencode.json"), nil
}

const opencodeInitial = "{\n  \"$schema\": \"https://opencode.ai/config.json\"\n}\n"

// Sync writes all of the gateway's chat models into its own provider.
// Fields llmenu does not maintain are preserved.
func (o opencode) Sync(_ context.Context, models []catalog.Model, opt SyncOptions) (Diff, error) {
	file, err := o.ConfigPath()
	if err != nil {
		return Diff{}, err
	}
	base, err := url.JoinPath(opt.BaseURL, "v1")
	if err != nil {
		return Diff{}, err
	}
	return syncJSON(file, []byte(opencodeInitial), []string{"provider", opt.Provider}, func(old any) any {
		prov := asMap(old)
		setDefault(prov, "npm", "@ai-sdk/openai-compatible")
		setDefault(prov, "name", opt.Provider)
		options := asMap(prov["options"])
		options["baseURL"] = base
		options["apiKey"] = "{env:" + KeyEnv + "}"
		prov["options"] = options

		existing := asMap(prov["models"])
		out := map[string]any{}
		for _, m := range models {
			e := maps.Clone(asMap(existing[m.Name]))
			setDefault(e, "name", m.Name)
			limit := map[string]any{}
			setPositive(limit, "context", m.ContextTokens)
			setPositive(limit, "output", m.MaxOutputTokens)
			e["limit"] = limit
			e["cost"] = map[string]float64{
				"input": m.InputPrice, "output": m.OutputPrice,
				"cache_read": m.CacheReadPrice, "cache_write": m.CacheWritePrice,
			}
			e["attachment"] = m.Vision
			e["reasoning"] = m.Reasoning && !m.NoThinking
			out[m.Name] = e
		}
		prov["models"] = out
		return prov
	}, opt.DryRun)
}

func (opencode) OwnSelection() {}

func (opencode) HasModelArg(args []string) bool { return hasFlag(args, "-m", "--model") }

func (opencode) Supports(catalog.Model) (bool, string) { return true, "" }

func (opencode) Command(ctx context.Context, _ *catalog.Model, args []string, env Env) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "opencode", args...)
	cmd.Env = env.childEnv(KeyEnv + "=" + env.APIKey)
	return cmd, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
