// Package gateway talks to a LiteLLM proxy.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"

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
