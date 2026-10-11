// Package bench fetches benchmarks from Artificial Analysis and maps them to
// the gateway's models. The values are cached so that the menu and `list`
// work without a fetch.
package bench

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/fetch"
)

const (
	URL    = "https://artificialanalysis.ai/api/v2/data/llms/models"
	Source = "Artificial Analysis (https://artificialanalysis.ai)"
	// StaleAfter: from this age on, the menu points to `llmenu bench`.
	StaleAfter = 14 * 24 * time.Hour
)

// Stopwords are longer than in the menu: for matching, variants like -high,
// -max or -reasoning count as the same model; the best value wins.
var Stopwords = append(slices.Clone(catalog.PickerStopwords),
	"reasoning", "adaptive", "preview", "low", "medium", "high", "xhigh", "max")

// Entry is a model from the Artificial Analysis API.
type Entry struct {
	Slug        string `json:"slug"`
	Evaluations struct {
		Intelligence  *float64 `json:"artificial_analysis_intelligence_index"`
		Coding        *float64 `json:"artificial_analysis_coding_index"`
		TerminalBench *float64 `json:"terminalbench_v4_0"` // fraction 0..1
	} `json:"evaluations"`
}

// Fetch retrieves all models from Artificial Analysis. url is overridable for tests.
func Fetch(ctx context.Context, hc *http.Client, url, key string) ([]Entry, error) {
	body, err := fetch.Get(ctx, hc, url, map[string]string{"x-api-key": key})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data *[]Entry `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Data == nil {
		return nil, fmt.Errorf("unexpected response from Artificial Analysis (no data array)")
	}
	return *resp.Data, nil
}

// Match assigns values to each gateway model, as claude-benchmark did before:
//   - via the real target model, so -latest aliases get values too;
//   - variants (-high, -max, …) count as one model, the best value wins;
//   - if the gateway disables thinking, only non-reasoning measurements count;
//   - overrides (gateway name -> AA slug) take precedence.
func Match(entries []Entry, models []catalog.Model, overrides map[string]string) map[string]catalog.Bench {
	byKey, byKeyNR, bySlug := map[string][]Entry{}, map[string][]Entry{}, map[string][]Entry{}
	for _, e := range entries {
		ev := e.Evaluations
		if ev.Intelligence == nil && ev.Coding == nil && ev.TerminalBench == nil {
			continue
		}
		k := catalog.Key(e.Slug, Stopwords)
		byKey[k] = append(byKey[k], e)
		if strings.Contains(e.Slug, "non-reasoning") {
			byKeyNR[k] = append(byKeyNR[k], e)
		}
		bySlug[e.Slug] = append(bySlug[e.Slug], e)
	}

	out := map[string]catalog.Bench{}
	for _, m := range models {
		var hits []Entry
		switch slug, ok := overrides[m.Name]; {
		case ok:
			hits = bySlug[slug]
		case m.NoThinking:
			hits = byKeyNR[catalog.Key(m.Target, Stopwords)]
		default:
			hits = byKey[catalog.Key(m.Target, Stopwords)]
		}
		if len(hits) > 0 {
			out[m.Name] = best(hits)
		}
	}
	return out
}

// best merges variants: highest value each, shortest slug.
func best(entries []Entry) catalog.Bench {
	var b catalog.Bench
	slugs := make([]string, 0, len(entries))
	for _, e := range entries {
		slugs = append(slugs, e.Slug)
		b.Intelligence = maxPtr(b.Intelligence, e.Evaluations.Intelligence)
		b.Coding = maxPtr(b.Coding, e.Evaluations.Coding)
		b.TerminalBench = maxPtr(b.TerminalBench, e.Evaluations.TerminalBench)
	}
	slices.SortFunc(slugs, func(a, b string) int { return cmp.Or(cmp.Compare(len(a), len(b)), cmp.Compare(a, b)) })
	b.Slug = slugs[0]
	if b.TerminalBench != nil {
		pct := math.Round(*b.TerminalBench*1000) / 10
		b.TerminalBench = &pct
	}
	return b
}

func maxPtr(a, b *float64) *float64 {
	if a == nil || (b != nil && *b > *a) {
		return b
	}
	return a
}

// Store is the cache of the matched values.
type Store struct {
	FetchedAt time.Time                `json:"fetched_at"`
	Models    map[string]catalog.Bench `json:"models"` // gateway name -> values
}

// Path returns the cache location.
func Path() (string, error) {
	if dir := os.Getenv("LLMENU_CACHE_DIR"); dir != "" {
		return filepath.Join(dir, "bench.json"), nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "llmenu", "bench.json"), nil
}

// Load reads the cache; nil without a cache.
func Load() (*Store, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return &s, nil
}

// Save writes the cache atomically.
func (s *Store) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Apply attaches the values to the models.
func (s *Store) Apply(models []catalog.Model) {
	if s == nil {
		return
	}
	for i := range models {
		if b, ok := s.Models[models[i].Name]; ok {
			models[i].Bench = &b
		}
	}
}

// Note describes source and date for the menu and `list`.
func (s *Store) Note(now time.Time) string {
	if s == nil {
		return "Benchmarks: none, see llmenu bench"
	}
	note := fmt.Sprintf("Benchmarks: %s, as of %s", Source, s.FetchedAt.Local().Format("2006-01-02"))
	if age := now.Sub(s.FetchedAt); age > StaleAfter {
		note += fmt.Sprintf(", %d days old: run llmenu bench", int(age.Hours()/24))
	}
	return note
}
