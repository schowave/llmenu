// Command fakegateway serves a fake LiteLLM gateway and a fake Artificial
// Analysis API for the local development sandbox (`mise run sandbox`). It
// answers only what llmenu reads; chat requests get a short explanation.
package main

import (
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

const aaResponse = `{"data": [
  {"slug": "claude-opus-5-5", "evaluations": {"artificial_analysis_intelligence_index": 57.6, "terminalbench_v4_0": 0.596}},
  {"slug": "claude-opus-5-5-xhigh", "evaluations": {"artificial_analysis_intelligence_index": 56}},
  {"slug": "claude-sonnet-4-5", "evaluations": {"artificial_analysis_intelligence_index": 42}},
  {"slug": "gpt-6-sol", "evaluations": {"artificial_analysis_intelligence_index": 47.6, "terminalbench_v4_0": 0.439}},
  {"slug": "qwen3-next", "evaluations": {"artificial_analysis_intelligence_index": 30}}
]}`

func main() {
	addr := flag.String("addr", "127.0.0.1:4111", "listen address")
	models := flag.String("models", "internal/gateway/testdata/model_info.json", "response for /model/info")
	key := flag.String("key", "sk-dev", "expected gateway key")
	aaKey := flag.String("aa-key", "aa-dev", "expected Artificial Analysis Pro key")
	aaFreeKey := flag.String("aa-free-key", "aa-free", "expected Artificial Analysis free key: HTTP 403 for /aa, /aa/free in pages")
	bench := flag.String("bench", "", "response for the Artificial Analysis API (default: a small built-in set)")
	flag.Parse()

	info, err := os.ReadFile(*models)
	if err != nil {
		log.Fatal(err)
	}

	aa := []byte(aaResponse)
	if *bench != "" {
		if aa, err = os.ReadFile(*bench); err != nil {
			log.Fatal(err)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/model/info", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+*key {
			http.Error(w, `{"error":{"message":"invalid key, the sandbox gateway expects `+*key+`"}}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(info)
	})
	// As on a typical gateway: the key has no budget, its user a monthly one.
	reset := time.Now().UTC().AddDate(0, 0, 18).Format("2006-01-02T15:04:05")
	budgets := map[string]string{
		"/key/info":  `{"info":{"spend":312.5,"total_spend":8.21,"max_budget":null,"user_id":"dev"}}`,
		"/user/info": `{"user_id":"dev","user_info":{"spend":12.34,"max_budget":50,"budget_duration":"30d","budget_reset_at":"` + reset + `"}}`,
	}
	for path, body := range budgets {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+*key {
				http.Error(w, `{"error":{"message":"invalid key"}}`, http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		})
	}
	// A deliberately malformed -bench file is still served to the Pro key,
	// for llmenu's error path; only the free list is then unavailable.
	free, freeErr := freePages(aa, 2)
	if freeErr != nil {
		log.Printf("no free list: %v", freeErr)
	}
	mux.HandleFunc("/aa", func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("x-api-key") {
		case *aaKey:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(aa)
		case *aaFreeKey:
			http.Error(w, `{"error":"Language models list requires a Pro subscription"}`, http.StatusForbidden)
		default:
			http.Error(w, `{"error":"invalid key, the sandbox expects `+*aaKey+` or `+*aaFreeKey+`"}`, http.StatusUnauthorized)
		}
	})
	mux.HandleFunc("/aa/free", func(w http.ResponseWriter, r *http.Request) {
		if k := r.Header.Get("x-api-key"); k != *aaKey && k != *aaFreeKey {
			http.Error(w, `{"error":"invalid key"}`, http.StatusUnauthorized)
			return
		}
		if freeErr != nil {
			http.Error(w, `{"error":"sandbox: no free list, the -bench file is not valid"}`, http.StatusInternalServerError)
			return
		}
		n, err := strconv.Atoi(cmp.Or(r.URL.Query().Get("page"), "1"))
		if err != nil || n < 1 || n > len(free) {
			http.Error(w, `{"error":"no such page"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(free[n-1])
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"sandbox gateway: only /model/info, /key/info and /user/info are implemented, no chat"}}`, http.StatusNotImplemented)
	})

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	fmt.Fprintf(os.Stderr, "fake gateway on http://%s/ (key %s), Artificial Analysis on http://%s/aa (Pro key %s, free key %s)\n",
		*addr, *key, *addr, *aaKey, *aaFreeKey)
	log.Fatal(srv.ListenAndServe())
}

// freePages turns the full list into the free one as Artificial Analysis
// serves it: only the Intelligence Index, a speed, and pages of size models.
func freePages(full []byte, size int) ([][]byte, error) {
	var in struct {
		Data []struct {
			Slug        string `json:"slug"`
			Evaluations struct {
				Intelligence *float64 `json:"artificial_analysis_intelligence_index"`
			} `json:"evaluations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(full, &in); err != nil {
		return nil, fmt.Errorf("benchmarks: %w", err)
	}
	type entry struct {
		Slug        string         `json:"slug"`
		Evaluations map[string]any `json:"evaluations"`
		Performance map[string]any `json:"performance"`
	}
	total := max((len(in.Data)+size-1)/size, 1)
	pages := make([][]byte, total)
	for p := range total {
		data := []entry{}
		for i, e := range in.Data[min(p*size, len(in.Data)):min((p+1)*size, len(in.Data))] {
			data = append(data, entry{
				Slug:        e.Slug,
				Evaluations: map[string]any{"artificial_analysis_intelligence_index": e.Evaluations.Intelligence},
				Performance: map[string]any{"median_output_tokens_per_second": 40 + 20*(p*size+i)},
			})
		}
		b, err := json.Marshal(map[string]any{
			"tier":       "free",
			"pagination": map[string]any{"page": p + 1, "page_size": size, "total_pages": total, "has_more": p+1 < total},
			"data":       data,
		})
		if err != nil {
			return nil, err
		}
		pages[p] = b
	}
	return pages, nil
}
