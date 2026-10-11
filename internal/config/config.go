// Package config reads and writes llmenu's settings (ADR-0003). The code
// deliberately contains no gateway or model names; suggestions for them come
// from the gateway at runtime.
package config

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config is the content of config.toml. Empty fields mean "not set"; the
// accessor methods supply the defaults.
type Config struct {
	Gateway Gateway `toml:"gateway,omitempty"`
	Claude  Claude  `toml:"claude,omitempty"`
	Sync    Sync    `toml:"sync,omitempty"`
	Bench   Bench   `toml:"bench,omitempty"`
	Codex   Codex   `toml:"codex,omitempty"`
}

type Codex struct {
	Default string `toml:"default,omitempty"` // model the menu starts on; required without a terminal
}

type Gateway struct {
	URL  string `toml:"url,omitempty"`
	Name string `toml:"name,omitempty"` // provider name in other tools' configurations
}

// Slots are Claude Code's model slots.
var Slots = []string{"opus", "sonnet", "haiku"}

type Claude struct {
	Default     string            `toml:"default,omitempty"` // slot name or model name
	Opus        string            `toml:"opus,omitempty"`
	Sonnet      string            `toml:"sonnet,omitempty"`
	Haiku       string            `toml:"haiku,omitempty"`
	AutoCompact *bool             `toml:"auto_compact,omitempty"`
	Env         map[string]string `toml:"env,omitempty"` // nil: DefaultClaudeEnv
}

type Sync struct {
	Targets []string `toml:"targets,omitempty"`
}

// Metrics are the benchmarks the menu and `list` can sort by.
var Metrics = []string{"intel", "tb4", "code"}

type Bench struct {
	Metric string            `toml:"metric,omitempty"` // intel (default), tb4 or code
	Map    map[string]string `toml:"map,omitempty"`    // gateway name -> Artificial Analysis slug
}

// MetricOrDefault returns the benchmark used for sorting.
func (b Bench) MetricOrDefault() string {
	if b.Metric == "" {
		return "intel"
	}
	return b.Metric
}

// DefaultClaudeEnv applies to every gateway: a gateway usually does not
// understand Anthropic's telemetry and beta headers.
var DefaultClaudeEnv = map[string]string{
	"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
	"CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS":   "1",
}

// Slot returns the model configured for a slot.
func (c Claude) Slot(slot string) string {
	switch slot {
	case "opus":
		return c.Opus
	case "sonnet":
		return c.Sonnet
	case "haiku":
		return c.Haiku
	}
	return ""
}

// SetSlot puts a model into a slot.
func (c *Claude) SetSlot(slot, model string) {
	switch slot {
	case "opus":
		c.Opus = model
	case "sonnet":
		c.Sonnet = model
	case "haiku":
		c.Haiku = model
	}
}

// DefaultOrOpus returns the default, "opus" if unset, as in Claude Code.
func (c Claude) DefaultOrOpus() string {
	if c.Default == "" {
		return "opus"
	}
	return c.Default
}

// AutoCompactEnabled is true if unset, as in Claude Code itself.
func (c Claude) AutoCompactEnabled() bool { return c.AutoCompact == nil || *c.AutoCompact }

// EnvOrDefault returns the additional variables for Claude Code.
func (c Claude) EnvOrDefault() map[string]string {
	if c.Env == nil {
		return DefaultClaudeEnv
	}
	return c.Env
}

// NameOrDerived returns gateway.name, derived from the host if unset.
func (g Gateway) NameOrDerived() string {
	if g.Name != "" {
		return g.Name
	}
	return NameFromURL(g.URL)
}

// NameFromURL derives a provider name from the host:
// https://api.example-gpt.corp.de/ yields "example-gpt".
func NameFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "gateway"
	}
	labels := strings.Split(u.Hostname(), ".")
	for len(labels) > 2 && slices.Contains([]string{"api", "llm", "llms", "ai", "gateway", "proxy", "www"}, labels[0]) {
		labels = labels[1:]
	}
	return labels[0]
}

// Path returns the location of config.toml.
func Path() (string, error) {
	if p := os.Getenv("LLMENU_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "llmenu", "config.toml"), nil
}

// Load reads config.toml. A missing file is not an error.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := toml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return &c, nil
}

const header = "# Written by llmenu. Change with `llmenu setup` or `llmenu config set`;\n" +
	"# comments in this file are lost on the next write.\n\n"

// Marshal returns the file content.
func (c *Config) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(header)
	enc := toml.NewEncoder(&buf)
	enc.SetIndentTables(false)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Save writes config.toml atomically, but only if something changed.
// changed reports whether it was written.
func (c *Config) Save() (changed bool, err error) {
	p, err := Path()
	if err != nil {
		return false, err
	}
	b, err := c.Marshal()
	if err != nil {
		return false, err
	}
	if old, err := os.ReadFile(p); err == nil && bytes.Equal(old, b) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return false, err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return false, err
	}
	return true, os.Rename(tmp, p)
}
