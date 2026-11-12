// Package bench fetches benchmarks from Artificial Analysis and maps them to
// the gateway's models. The values are cached so that the menu and `list`
// work without a fetch.
package bench

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/schowave/llmenu/internal/atomicfile"
	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/fetch"

	"github.com/schowave/llmenu/internal/guard"
)

const (
	// URL is the full model list (Pro keys); free keys use URL/free, with
	// fewer fields: no Terminal-Bench, no Long Context.
	URL    = "https://artificialanalysis.ai/api/v2/language/models"
	Source = "Artificial Analysis (https://artificialanalysis.ai)"
	// TierFree is the tier of a key that only reaches the free list.
	TierFree = "free"
	// StaleAfter: from this age on, the menu points to `llmenu bench`.
	StaleAfter = 14 * 24 * time.Hour
	// maxPages caps the pages read (the list has a few at 200 models each);
	// beyond it the result is marked as truncated.
	maxPages = 50
	// parallelPages: pages 2.. are fetched this many at a time.
	parallelPages = 4
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
		TerminalBench *float64 `json:"terminalbench_v4_0"` // fraction 0..1, Pro only
		LongContext   *float64 `json:"aa_lcr"`             // Long Context Reasoning, fraction 0..1, Pro only
	} `json:"evaluations"`
	Performance struct {
		// OutputSpeed is measured at the model's own provider, not through
		// the gateway; 0 means not measured.
		OutputSpeed *float64 `json:"median_output_tokens_per_second"`
	} `json:"performance"`
}

// Result is the outcome of Fetch.
type Result struct {
	Entries []Entry
	// Tier as the API reports it, e.g. "pro"; TierFree if the free list
	// was used.
	Tier string
	// Truncated: the list has more than maxPages pages; Entries holds the
	// first maxPages.
	Truncated bool
}

// Fetch retrieves all models from Artificial Analysis. base is URL, or
// another server for tests. A key without Pro access gets HTTP 403 for the
// full list; Fetch then reads the free list. The full list is tried every
// time, so a key that became Pro, or a passing 403, is noticed.
func Fetch(ctx context.Context, hc *http.Client, base, key string) (Result, error) {
	free, err := url.JoinPath(base, TierFree)
	if err != nil {
		return Result{}, err
	}
	res, err := fetchAll(ctx, hc, base, key)
	var se *fetch.StatusError
	if !errors.As(err, &se) || se.Code != http.StatusForbidden {
		res.Tier = cmp.Or(res.Tier, "pro")
		return res, err
	}
	res, ferr := fetchAll(ctx, hc, free, key)
	if ferr != nil {
		return Result{}, fmt.Errorf("%w (before that, the full list: %v)", ferr, err)
	}
	res.Tier = cmp.Or(res.Tier, TierFree)
	return res, nil
}

// page is one response of a model list.
type page struct {
	Tier       string   `json:"tier"`
	Data       *[]Entry `json:"data"`
	Pagination struct {
		TotalPages int  `json:"total_pages"`
		HasMore    bool `json:"has_more"`
	} `json:"pagination"`
}

// fetchAll reads page 1, then the remaining pages in parallel if the API
// names their number, otherwise one after another while it reports more.
func fetchAll(ctx context.Context, hc *http.Client, base, key string) (Result, error) {
	first, err := getPage(ctx, hc, base, key, 1)
	if err != nil {
		return Result{}, err
	}
	res := Result{Tier: first.Tier, Entries: slices.Clone(*first.Data)}
	if res.Entries == nil {
		res.Entries = []Entry{}
	}
	if !first.Pagination.HasMore {
		return res, nil
	}

	if n := first.Pagination.TotalPages; n > 1 {
		last := min(n, maxPages)
		pages := make([][]Entry, last+1)
		// The first error cancels the pages still running or waiting, e.g.
		// on a rate limit, and is the one reported.
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		var once sync.Once
		var firstErr error
		sem := make(chan struct{}, parallelPages)
		var wg sync.WaitGroup
		for i := 2; i <= last; i++ {
			wg.Go(func() {
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				defer func() { <-sem }()
				p, err := getPage(ctx, hc, base, key, i)
				if err != nil {
					once.Do(func() { firstErr = err; cancel() })
					return
				}
				pages[i] = *p.Data
			})
		}
		wg.Wait()
		if firstErr != nil {
			return Result{}, firstErr
		}
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		for _, p := range pages {
			res.Entries = append(res.Entries, p...)
		}
		res.Truncated = n > maxPages
		return res, nil
	}

	for i := 2; ; i++ {
		if i > maxPages {
			res.Truncated = true
			return res, nil
		}
		p, err := getPage(ctx, hc, base, key, i)
		if err != nil {
			return Result{}, err
		}
		res.Entries = append(res.Entries, *p.Data...)
		if !p.Pagination.HasMore {
			return res, nil
		}
	}
}

func getPage(ctx context.Context, hc *http.Client, base, key string, n int) (page, error) {
	u, err := url.Parse(base)
	if err != nil {
		return page{}, err
	}
	if n > 1 {
		q := u.Query()
		q.Set("page", strconv.Itoa(n))
		u.RawQuery = q.Encode()
	}
	body, err := fetch.Get(ctx, hc, u.String(), map[string]string{"x-api-key": key})
	if err != nil {
		return page{}, err
	}
	var p page
	if err := json.Unmarshal(body, &p); err != nil || p.Data == nil {
		return page{}, fmt.Errorf("unexpected response from Artificial Analysis (no data array)")
	}
	return p, nil
}

// Match assigns values to each gateway model, as claude-benchmark did before:
//   - via the real target model, so -latest aliases get values too;
//   - variants (-high, -max, …) count as one model, the best value wins;
//   - if the gateway disables thinking, only non-reasoning measurements count;
//   - overrides (gateway name -> AA slug) take precedence.
func Match(entries []Entry, models []catalog.Model, overrides map[string]string) map[string]catalog.Bench {
	byKey, byKeyNR, bySlug := map[string][]Entry{}, map[string][]Entry{}, map[string][]Entry{}
	for _, e := range entries {
		if !e.hasValues() {
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

// hasValues: at least one benchmark or a measured speed.
func (e Entry) hasValues() bool {
	ev := e.Evaluations
	speed := e.Performance.OutputSpeed
	return ev.Intelligence != nil || ev.TerminalBench != nil || ev.LongContext != nil ||
		(speed != nil && *speed > 0)
}

// best merges variants: highest value each, shortest slug.
func best(entries []Entry) catalog.Bench {
	var b catalog.Bench
	slugs := make([]string, 0, len(entries))
	for _, e := range entries {
		slugs = append(slugs, e.Slug)
		b.Intelligence = maxPtr(b.Intelligence, e.Evaluations.Intelligence)
		b.TerminalBench = maxPtr(b.TerminalBench, e.Evaluations.TerminalBench)
		b.LongContext = maxPtr(b.LongContext, e.Evaluations.LongContext)
		if v := e.Performance.OutputSpeed; v != nil && *v > 0 {
			b.OutputSpeed = maxPtr(b.OutputSpeed, v)
		}
	}
	slices.SortFunc(slugs, func(a, b string) int { return cmp.Or(cmp.Compare(len(a), len(b)), cmp.Compare(a, b)) })
	b.Slug = slugs[0]
	b.TerminalBench = percent(b.TerminalBench)
	b.LongContext = percent(b.LongContext)
	if b.OutputSpeed != nil {
		tps := math.Round(*b.OutputSpeed)
		b.OutputSpeed = &tps
	}
	return b
}

// percent turns a fraction into percent with one decimal.
func percent(v *float64) *float64 {
	if v == nil {
		return nil
	}
	pct := math.Round(*v*1000) / 10
	return &pct
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
	Tier      string                   `json:"tier,omitempty"` // the key's API tier, e.g. "free"
	Models    map[string]catalog.Bench `json:"models"`         // gateway name -> values
	// Truncated: the fetch stopped at the page limit; not saved.
	Truncated bool `json:"-"`
}

// Path returns the cache location.
func Path() (string, error) {
	p, err := path()
	if err != nil {
		return "", err
	}
	return guard.Path(p)
}

func path() (string, error) {
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
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(p, b, 0o600)
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
