// Command fakegateway serves a fake LiteLLM gateway and a fake Artificial
// Analysis API for the local development sandbox (`mise run sandbox`). It
// answers only what llmenu reads; chat requests get a short explanation.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

const aaResponse = `{"data": [
  {"slug": "claude-opus-5-5", "evaluations": {"artificial_analysis_intelligence_index": 57.6, "terminalbench_v4_0": 0.596}},
  {"slug": "claude-opus-5-5-xhigh", "evaluations": {"artificial_analysis_intelligence_index": 56, "artificial_analysis_coding_index": 80}},
  {"slug": "claude-sonnet-4-5", "evaluations": {"artificial_analysis_intelligence_index": 42}},
  {"slug": "gpt-6-sol", "evaluations": {"artificial_analysis_intelligence_index": 47.6, "terminalbench_v4_0": 0.439}},
  {"slug": "qwen3-next", "evaluations": {"artificial_analysis_intelligence_index": 30}}
]}`

func main() {
	addr := flag.String("addr", "127.0.0.1:4111", "listen address")
	models := flag.String("models", "internal/gateway/testdata/model_info.json", "response for /model/info")
	key := flag.String("key", "sk-dev", "expected gateway key")
	aaKey := flag.String("aa-key", "aa-dev", "expected Artificial Analysis key")
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
		"/key/info":  `{"info":{"spend":312.5,"max_budget":null,"user_id":"dev"}}`,
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
	mux.HandleFunc("/aa", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != *aaKey {
			http.Error(w, `{"error":"invalid key, the sandbox expects `+*aaKey+`"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(aa)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"sandbox gateway: only /model/info, /key/info and /user/info are implemented, no chat"}}`, http.StatusNotImplemented)
	})

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	fmt.Fprintf(os.Stderr, "fake gateway on http://%s/ (key %s), Artificial Analysis on http://%s/aa (key %s)\n", *addr, *key, *addr, *aaKey)
	log.Fatal(srv.ListenAndServe())
}
