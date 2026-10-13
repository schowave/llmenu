package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

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

func TestDeploymentModel(t *testing.T) {
	tests := []struct {
		name       string
		json       string
		wantTarget string
		wantCtx    int
	}{
		{"target from litellm_params", `{"model_name":"a","litellm_params":{"model":"bedrock/a"}}`, "bedrock/a", 0},
		{"target falls back to name", `{"model_name":"a"}`, "a", 0},
		{"one token is a valid limit", `{"model_name":"a","model_info":{"max_input_tokens":1}}`, "a", 1},
		{"price in token field discarded", `{"model_name":"a","model_info":{"max_input_tokens":3e-6}}`, "a", 0},
		{"negative discarded", `{"model_name":"a","model_info":{"max_input_tokens":-5}}`, "a", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d deployment
			if err := json.Unmarshal([]byte(tt.json), &d); err != nil {
				t.Fatal(err)
			}
			m := d.model()
			if m.Target != tt.wantTarget || m.ContextTokens != tt.wantCtx {
				t.Errorf("Target, ContextTokens = %q, %d, want %q, %d", m.Target, m.ContextTokens, tt.wantTarget, tt.wantCtx)
			}
		})
	}
}

// serveBudgets answers /key/info, /user/info and /team/info with the given
// bodies; a missing body is a 404, as for routes the gateway hides.
func serveBudgets(t *testing.T, bodies map[string]string) Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			http.Error(w, "wrong key", http.StatusUnauthorized)
			return
		}
		path := r.URL.Path
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		body, ok := bodies[path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return Client{BaseURL: srv.URL, APIKey: "sk-test"}
}

func TestSpend(t *testing.T) {
	reset := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		bodies map[string]string
		want   Budget
	}{
		{"key budget", map[string]string{
			"/key/info": `{"key":"x","info":{"spend":12.345,"max_budget":50,"budget_duration":"30d","budget_reset_at":"2026-11-01T00:00:00+00:00"}}`,
		}, Budget{Scope: "key", Spend: 12.345, Max: 50, Period: "30d", ResetAt: reset}},
		{"reset without zone", map[string]string{
			"/key/info": `{"info":{"spend":1,"max_budget":2,"budget_reset_at":"2026-11-01T00:00:00.123456"}}`,
		}, Budget{Scope: "key", Spend: 1, Max: 2, ResetAt: reset.Add(123456 * time.Microsecond)}},
		{"no budget anywhere", map[string]string{
			"/key/info": `{"info":{"spend":0.5,"max_budget":null,"budget_duration":null,"budget_reset_at":null}}`,
		}, Budget{Scope: "key", Spend: 0.5}},
		{"zero budget means none", map[string]string{"/key/info": `{"info":{"spend":0,"max_budget":0}}`}, Budget{Scope: "key"}},
		// The common case: the key has none, its user a monthly one.
		{"user budget", map[string]string{
			"/key/info":  `{"info":{"spend":719.89,"total_spend":12.45,"max_budget":null,"user_id":"u1","team_id":null}}`,
			"/user/info": `{"user_id":"u1","user_info":{"spend":17.99,"max_budget":50,"budget_duration":"30d","budget_reset_at":"2026-11-01T00:00:00Z"}}`,
		}, Budget{Scope: "user", Spend: 17.99, KeySpend: 12.45, Max: 50, Period: "30d", ResetAt: reset}},
		{"key spend in the period", map[string]string{
			"/key/info": `{"info":{"spend":719.89,"total_spend":12.45}}`,
		}, Budget{Scope: "key", Spend: 12.45}},
		{"key budget counts spend", map[string]string{
			"/key/info": `{"info":{"spend":20,"total_spend":12.45,"max_budget":50}}`,
		}, Budget{Scope: "key", Spend: 20, Max: 50}},
		// A team key spends from the team's budget, not its user's.
		{"team budget from user info", map[string]string{
			"/key/info":  `{"info":{"spend":0,"total_spend":0,"user_id":"u1","team_id":"t1"}}`,
			"/user/info": `{"user_info":{"spend":17.99,"max_budget":50},"teams":[{"team_id":"t0","spend":1,"max_budget":2},{"team_id":"t1","spend":0,"max_budget":100,"budget_duration":"30d","budget_reset_at":"2026-11-01T00:00:00Z"}]}`,
		}, Budget{Scope: "team", Max: 100, Period: "30d", ResetAt: reset}},
		{"team budget from team info", map[string]string{
			"/key/info":             `{"info":{"spend":1,"user_id":"u1","team_id":"t1"}}`,
			"/user/info":            `{"user_info":{"spend":49,"max_budget":50},"teams":[]}`,
			"/team/info?team_id=t1": `{"team_info":{"spend":45,"max_budget":50}}`,
		}, Budget{Scope: "team", Spend: 45, KeySpend: 1, Max: 50}},
		{"team key without readable team budget", map[string]string{
			"/key/info":  `{"info":{"spend":2,"user_id":"u1","team_id":"t1"}}`,
			"/user/info": `{"user_info":{"spend":49,"max_budget":50}}`,
		}, Budget{Scope: "key", Spend: 2}},
		{"key budget tighter than user's", map[string]string{
			"/key/info":  `{"info":{"spend":4,"max_budget":5,"user_id":"u1"}}`,
			"/user/info": `{"user_info":{"spend":10,"max_budget":50}}`,
		}, Budget{Scope: "key", Spend: 4, Max: 5}},
		{"user without budget", map[string]string{
			"/key/info":  `{"info":{"spend":3,"user_id":"u1"}}`,
			"/user/info": `{"user_info":{"spend":30,"max_budget":null}}`,
		}, Budget{Scope: "key", Spend: 3}},
		{"user info refused", map[string]string{
			"/key/info": `{"info":{"spend":3,"max_budget":5,"user_id":"u1","team_id":"t1"}}`,
		}, Budget{Scope: "key", Spend: 3, Max: 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := serveBudgets(t, tt.bodies).Spend(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got.Scope != tt.want.Scope || got.Spend != tt.want.Spend || got.KeySpend != tt.want.KeySpend || got.Max != tt.want.Max || got.Period != tt.want.Period || !got.ResetAt.Equal(tt.want.ResetAt) {
				t.Errorf("Spend = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSpendErrors(t *testing.T) {
	tests := []struct {
		name   string
		client Client
		want   string
	}{
		{"not found", serveBudgets(t, nil), "HTTP 404"},
		{"wrong key", func() Client { c := serveBudgets(t, nil); c.APIKey = "x"; return c }(), "HTTP 401"},
		{"no JSON", serveBudgets(t, map[string]string{"/key/info": "<html>"}), "unexpected response"},
		{"no info", serveBudgets(t, map[string]string{"/key/info": `{"key":"x"}`}), "no info"},
		{"no spend", serveBudgets(t, map[string]string{"/key/info": `{"info":{}}`}), "no spend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.client.Spend(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
