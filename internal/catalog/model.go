// Package catalog describes the gateway's models, enriched with benchmarks.
package catalog

// Model is a chat model of the gateway, as returned by /model/info.
// Prices in dollars per million tokens.
type Model struct {
	Name            string   `json:"name"`
	Target          string   `json:"target"`                // real target model (litellm_params.model), differs from Name for aliases
	NoThinking      bool     `json:"no_thinking,omitempty"` // gateway disables thinking (enable_thinking: false)
	ContextTokens   int      `json:"context_tokens"`
	MaxOutputTokens int      `json:"max_output_tokens"`
	Vision          bool     `json:"vision"`
	Reasoning       bool     `json:"reasoning"`
	Endpoints       []string `json:"endpoints,omitempty"` // supported_endpoints, e.g. /v1/responses

	InputPrice      float64 `json:"input_price"`
	OutputPrice     float64 `json:"output_price"`
	CacheReadPrice  float64 `json:"cache_read_price"`
	CacheWritePrice float64 `json:"cache_write_price"`
	NoPrice         bool    `json:"no_price,omitempty"` // the gateway names no input or output price; 0 then means unknown, not free
	Tiers           []Tier  `json:"tiers,omitempty"`    // higher prices for long prompts, by ascending threshold

	Bench *Bench `json:"bench,omitempty"` // nil until `llmenu bench` has run or if there is no match
}

// Tier is a price level for long prompts: once a prompt has more than
// AboveTokens tokens, the whole request costs these prices (dollars per
// million tokens).
type Tier struct {
	AboveTokens int     `json:"above_tokens"`
	InputPrice  float64 `json:"input_price"`
	OutputPrice float64 `json:"output_price"`
}

// Bench holds the matched values from Artificial Analysis. Missing values are nil.
type Bench struct {
	Slug          string   `json:"slug"` // AA slug used for the match
	Intelligence  *float64 `json:"intelligence,omitempty"`
	Coding        *float64 `json:"coding,omitempty"`
	TerminalBench *float64 `json:"terminal_bench,omitempty"` // Terminal-Bench v4 in percent
	LongContext   *float64 `json:"long_context,omitempty"`   // AA Long Context Reasoning in percent
	OutputSpeed   *float64 `json:"output_speed,omitempty"`   // median output tokens per second, measured at the model's provider
}
