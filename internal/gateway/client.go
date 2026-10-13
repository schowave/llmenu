// Package gateway talks to a LiteLLM proxy.
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/fetch"
)

// If max_output_tokens is missing, this value is used, as in the earlier zsh version.
const defaultMaxOutputTokens = 16384

// Client queries a LiteLLM proxy.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client // nil: default client with a 30 s timeout
}

// Models returns all chat models from /model/info in the gateway's order.
func (c Client) Models(ctx context.Context) ([]catalog.Model, error) {
	u, err := url.JoinPath(c.BaseURL, "model/info")
	if err != nil {
		return nil, fmt.Errorf("gateway URL %q: %w", c.BaseURL, err)
	}
	body, err := fetch.Get(ctx, c.HTTP, u, map[string]string{"Authorization": "Bearer " + c.APIKey})
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data *[]deployment `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unexpected response from %s: %w", u, err)
	}
	if resp.Data == nil {
		return nil, fmt.Errorf("unexpected response from %s: no data array", u)
	}

	var models []catalog.Model
	for _, d := range *resp.Data {
		if d.Info.Mode == "chat" {
			models = append(models, d.model())
		}
	}
	return models, nil
}

// deployment is an entry from /model/info. Numbers are *float64 because
// LiteLLM occasionally returns token counts as floats or null.
type deployment struct {
	ModelName string `json:"model_name"`
	Params    struct {
		Model     string `json:"model"`
		ExtraBody struct {
			ChatTemplateKwargs struct {
				EnableThinking *bool `json:"enable_thinking"`
			} `json:"chat_template_kwargs"`
		} `json:"extra_body"`
	} `json:"litellm_params"`
	Info struct {
		Mode               string   `json:"mode"`
		MaxInputTokens     *float64 `json:"max_input_tokens"`
		MaxOutputTokens    *float64 `json:"max_output_tokens"`
		SupportsVision     *bool    `json:"supports_vision"`
		SupportsReasoning  *bool    `json:"supports_reasoning"`
		SupportedEndpoints []string `json:"supported_endpoints"`
		InputCost          *float64 `json:"input_cost_per_token"`
		OutputCost         *float64 `json:"output_cost_per_token"`
		CacheReadCost      *float64 `json:"cache_read_input_token_cost"`
		CacheWriteCost     *float64 `json:"cache_creation_input_token_cost"`
	} `json:"model_info"`
}

func (d deployment) model() catalog.Model {
	target := d.Params.Model
	if target == "" {
		target = d.ModelName
	}
	enable := d.Params.ExtraBody.ChatTemplateKwargs.EnableThinking
	maxOut := tokens(d.Info.MaxOutputTokens)
	if maxOut == 0 {
		maxOut = defaultMaxOutputTokens
	}
	return catalog.Model{
		Name:            d.ModelName,
		Target:          target,
		NoThinking:      enable != nil && !*enable,
		ContextTokens:   tokens(d.Info.MaxInputTokens),
		MaxOutputTokens: maxOut,
		Vision:          d.Info.SupportsVision != nil && *d.Info.SupportsVision,
		Reasoning:       d.Info.SupportsReasoning != nil && *d.Info.SupportsReasoning,
		Endpoints:       d.Info.SupportedEndpoints,
		InputPrice:      perMillion(d.Info.InputCost),
		OutputPrice:     perMillion(d.Info.OutputCost),
		CacheReadPrice:  perMillion(d.Info.CacheReadCost),
		CacheWritePrice: perMillion(d.Info.CacheWriteCost),
	}
}

// tokens discards nonsensical values; LiteLLM occasionally puts prices in the token field.
func tokens(v *float64) int {
	if v == nil || *v < 1 {
		return 0
	}
	return int(*v)
}

// perMillion converts dollars per token to dollars per million tokens,
// rounded to six decimal places against floating-point noise.
func perMillion(v *float64) float64 {
	if v == nil {
		return 0
	}
	return math.Round(*v*1e12) / 1e6
}

// Budget is what the gateway has booked against the limit that binds a key.
type Budget struct {
	Scope    string    `json:"scope"`               // whose budget: "key", "team" or "user"
	Spend    float64   `json:"spend"`               // US dollars, as LiteLLM computes them from its prices
	KeySpend float64   `json:"key_spend,omitempty"` // this key's share of a team's or user's Spend
	Max      float64   `json:"max,omitempty"`       // max_budget; 0: none
	Period   string    `json:"period,omitempty"`    // budget_duration, e.g. "30d"; empty: never reset
	ResetAt  time.Time `json:"reset_at,omitzero"`   // when Spend starts again from zero
}

// left is what remains of the budget.
func (b Budget) left() float64 { return b.Max - b.Spend }

// Spend returns the budget that binds the client's own key, with the key's
// share. A team key spends from its team's max_budget, a personal key from
// its user's; either may also have one of its own. Of those with a limit,
// the one with the least left wins; without any, the key's spend. Every key
// may ask about itself, its user and its user's teams, so no admin rights
// are needed.
func (c Client) Spend(ctx context.Context) (Budget, error) {
	var key struct {
		budgetInfo
		TeamID *string `json:"team_id"`
		UserID *string `json:"user_id"`
		// total_spend is what the key spent in the current period, the
		// figure LiteLLM's UI shows per key; spend can be far higher.
		TotalSpend *float64 `json:"total_spend"`
	}
	if err := c.getJSON(ctx, "key/info", nil, "info", &key); err != nil {
		return Budget{}, err
	}
	if key.Spend == nil {
		return Budget{}, errors.New("unexpected response from key/info: no spend")
	}
	best := key.budget("key")
	keySpend := best.Spend
	if key.TotalSpend != nil {
		keySpend = *key.TotalSpend
	}
	if best.Max == 0 {
		best.Spend = keySpend
	}

	var other Budget
	switch {
	case key.TeamID != nil && *key.TeamID != "":
		other = c.teamBudget(ctx, *key.TeamID)
	case key.UserID != nil && *key.UserID != "":
		other = c.scopeBudget(ctx, "user/info", nil, "user_info", "user")
	}
	if other.Max > 0 && (best.Max == 0 || other.left() < best.left()) {
		best = other
		best.KeySpend = keySpend
	}
	return best, nil
}

// teamBudget is a team's budget: from the teams /user/info lists, which every
// key may read, else from /team/info, which gateways often keep for admins.
func (c Client) teamBudget(ctx context.Context, id string) Budget {
	var teams []struct {
		budgetInfo
		TeamID string `json:"team_id"`
	}
	if c.getJSON(ctx, "user/info", nil, "teams", &teams) == nil {
		for _, t := range teams {
			if t.TeamID == id && t.Spend != nil {
				return t.budget("team")
			}
		}
	}
	return c.scopeBudget(ctx, "team/info", url.Values{"team_id": {id}}, "team_info", "team")
}

// scopeBudget is a team's or user's budget; zero if it has none or the
// gateway does not tell.
func (c Client) scopeBudget(ctx context.Context, path string, query url.Values, field, scope string) Budget {
	var info budgetInfo
	if c.getJSON(ctx, path, query, field, &info) != nil || info.Spend == nil {
		return Budget{}
	}
	return info.budget(scope)
}

// budgetInfo holds the budget fields LiteLLM uses for keys, teams and users.
type budgetInfo struct {
	Spend   *float64 `json:"spend"`
	Max     *float64 `json:"max_budget"`
	Period  *string  `json:"budget_duration"`
	ResetAt *string  `json:"budget_reset_at"`
}

func (i budgetInfo) budget(scope string) Budget {
	b := Budget{Scope: scope}
	if i.Spend != nil {
		b.Spend = *i.Spend
	}
	if i.Max != nil && *i.Max > 0 {
		b.Max = *i.Max
	}
	if i.Period != nil {
		b.Period = *i.Period
	}
	if i.ResetAt != nil {
		b.ResetAt = parseTime(*i.ResetAt)
	}
	return b
}

// getJSON reads the object in field of the response from path into v.
func (c Client) getJSON(ctx context.Context, path string, query url.Values, field string, v any) error {
	u, err := url.JoinPath(c.BaseURL, path)
	if err != nil {
		return fmt.Errorf("gateway URL %q: %w", c.BaseURL, err)
	}
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	body, err := fetch.Get(ctx, c.HTTP, u, map[string]string{"Authorization": "Bearer " + c.APIKey})
	if err != nil {
		return err
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("unexpected response from %s: %w", u, err)
	}
	raw, ok := resp[field]
	if !ok || string(raw) == "null" {
		return fmt.Errorf("unexpected response from %s: no %s", u, field)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("unexpected response from %s: %w", u, err)
	}
	return nil
}

// parseTime reads LiteLLM's timestamps, which come with or without a zone
// (then UTC); zero if none fits.
func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
