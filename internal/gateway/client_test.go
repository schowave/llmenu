package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/schowave/llmenu/internal/catalog"
)

func serve(t *testing.T, status int, body string) Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/model/info" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			http.Error(w, "wrong key", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return Client{BaseURL: srv.URL + "/", APIKey: "sk-test"}
}

func TestModels(t *testing.T) {
	fixture, err := os.ReadFile("testdata/model_info.json")
	if err != nil {
		t.Fatal(err)
	}
	models, err := serve(t, http.StatusOK, string(fixture)).Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	byName := map[string]catalog.Model{}
	for _, m := range models {
		names = append(names, m.Name)
		byName[m.Name] = m
	}
	if slices.Contains(names, "azure/text-embedding") {
		t.Error("embedding model not filtered out")
	}
	if len(models) != 8 {
		t.Errorf("%d chat models, want 8: %v", len(models), names)
	}

	sonnet := byName["aws/claude-sonnet-4-5"]
	if sonnet.InputPrice != 3.3 || sonnet.OutputPrice != 16.5 || sonnet.CacheReadPrice != 0.33 || sonnet.CacheWritePrice != 4.125 {
		t.Errorf("prices per million converted incorrectly: %+v", sonnet)
	}
	if !sonnet.Vision || !sonnet.Reasoning || sonnet.ContextTokens != 200_000 || sonnet.MaxOutputTokens != 64_000 {
		t.Errorf("capabilities or limits wrong: %+v", sonnet)
	}
	if got := byName["mistral/mistral-large-4"].MaxOutputTokens; got != defaultMaxOutputTokens {
		t.Errorf("missing max_output_tokens = %d, want default %d", got, defaultMaxOutputTokens)
	}
	if got := byName["local/Qwen3-Next"].ContextTokens; got != 229_376 {
		t.Errorf("context as float = %d, want 229376", got)
	}
	if !byName["local/Qwen3-Next-non-thinking"].NoThinking || byName["local/Qwen3-Next"].NoThinking {
		t.Error("enable_thinking: false not detected")
	}
	if !slices.Contains(byName["azure/gpt-6-sol"].Endpoints, "/v1/responses") {
		t.Error("supported_endpoints missing")
	}
}

func TestModelsErrors(t *testing.T) {
	tests := []struct {
		name   string
		client Client
		want   string
	}{
		{"HTTP error with response", serve(t, http.StatusInternalServerError, `{"error":"broken"}`), `HTTP 500`},
		{"wrong key", func() Client { c := serve(t, 200, "{}"); c.APIKey = "x"; return c }(), "HTTP 401"},
		{"no JSON", serve(t, http.StatusOK, "<html>"), "unexpected response"},
		{"no data", serve(t, http.StatusOK, `{"foo":1}`), "no data array"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.client.Models(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
