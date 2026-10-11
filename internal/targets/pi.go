package targets

import (
	"cmp"
	"context"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"

	"github.com/schowave/llmenu/internal/catalog"
)

func init() { register(pi{}) }

// pi maintains its own provider in ~/.pi/agent/models.json. pi is launched
// without llmenu and does its own model selection. pi fetches the key from
// llmenu via a command.
type pi struct {
	path string // tests only
}

func (pi) Name() string        { return "pi" }
func (pi) Description() string { return "Models are written into ~/.pi/agent/models.json by sync" }
func (pi) Binary() string      { return "pi" }

func (p pi) ConfigPath() (string, error) {
	if p.path != "" {
		return p.path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pi", "agent", "models.json"), nil
}

// Sync writes all of the gateway's chat models, aliases included, into its own
// provider. Anything already there that llmenu does not maintain (such as
// custom display names) is preserved.
func (p pi) Sync(_ context.Context, models []catalog.Model, opt SyncOptions) (Diff, error) {
	file, err := p.ConfigPath()
	if err != nil {
		return Diff{}, err
	}
	base, err := url.JoinPath(opt.BaseURL, "v1")
	if err != nil {
		return Diff{}, err
	}
	return syncJSON(file, []byte("{}\n"), []string{"providers", opt.Provider}, func(old any) any {
		prov := asMap(old)
		setDefault(prov, "name", opt.Provider)
		setDefault(prov, "api", "openai-completions")
		prov["baseUrl"] = base
		prov["apiKey"] = "!" + KeyCommand

		existing := map[string]map[string]any{}
		if list, ok := prov["models"].([]any); ok {
			for _, e := range list {
				m := asMap(e)
				if id, ok := m["id"].(string); ok {
					existing[id] = m
				}
			}
		}
		var list []any
		for _, m := range byName(models) {
			e := maps.Clone(existing[m.Name])
			if e == nil {
				e = map[string]any{}
			}
			e["id"] = m.Name
			setDefault(e, "name", m.Name)
			e["reasoning"] = m.Reasoning && !m.NoThinking
			e["input"] = []string{"text"}
			if m.Vision {
				e["input"] = []string{"text", "image"}
			}
			setPositive(e, "contextWindow", m.ContextTokens)
			setPositive(e, "maxTokens", m.MaxOutputTokens)
			e["cost"] = map[string]float64{
				"input": m.InputPrice, "output": m.OutputPrice,
				"cacheRead": m.CacheReadPrice, "cacheWrite": m.CacheWritePrice,
			}
			list = append(list, e)
		}
		prov["models"] = list
		return prov
	}, opt.DryRun)
}

// byName returns the models sorted by name so diffs stay stable.
func byName(models []catalog.Model) []catalog.Model {
	out := slices.Clone(models)
	slices.SortFunc(out, func(a, b catalog.Model) int { return cmp.Compare(a.Name, b.Name) })
	return out
}

func setDefault(m map[string]any, key string, v any) {
	if _, ok := m[key]; !ok {
		m[key] = v
	}
}

// setPositive sets a token count; unknown ones (0) are left out.
func setPositive(m map[string]any, key string, v int) {
	if v > 0 {
		m[key] = v
	} else {
		delete(m, key)
	}
}
