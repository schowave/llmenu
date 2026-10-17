// Package config reads and writes llmenu's settings (ADR-0003). The code
// deliberately contains no gateway or model names; suggestions for them come
// from the gateway at runtime.
package config

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/schowave/llmenu/internal/atomicfile"
	"github.com/schowave/llmenu/internal/guard"
)

// Config is the content of config.toml. Empty fields mean "not set"; the
// accessor methods supply the defaults.
type Config struct {
	// Tools are the coding tools in use (claude, codex, copilot, opencode, pi). nil:
	// never chosen, the installed tools apply; an empty list is a deliberate
	// "none" and is written as `tools = []`.
	Tools *[]string `toml:"tools,omitempty"`
	// Launch is the tool `llmenu` without a command starts after the menu;
	// "none": only print the chosen name; empty: never chosen.
	Launch  string  `toml:"launch,omitempty"`
	Gateway Gateway `toml:"gateway,omitempty"`
	Claude  Claude  `toml:"claude,omitempty"`
	Bench   Bench   `toml:"bench,omitempty"`
	Codex   Codex   `toml:"codex,omitempty"`
	Copilot Copilot `toml:"copilot,omitempty"`
}

type Codex struct {
	Default string `toml:"default,omitempty"` // model the menu starts on; required without a terminal
}

type Copilot struct {
	Default string `toml:"default,omitempty"` // model the menu starts on; required without a terminal
	// ProviderType is the protocol Copilot CLI speaks to the gateway
	// (CopilotProviderTypes); empty: openai.
	ProviderType string `toml:"provider_type,omitempty"`
	Offline      *bool  `toml:"offline,omitempty"` // COPILOT_OFFLINE: no contact with GitHub
}

// CopilotProviderTypes are the COPILOT_PROVIDER_TYPE values that work with a
// LiteLLM gateway; azure needs Azure's routes and deployment names.
var CopilotProviderTypes = []string{"openai", "anthropic"}

// ProviderTypeOrDefault returns the provider type, "openai" if unset, as in
// Copilot CLI.
func (c Copilot) ProviderTypeOrDefault() string {
	if c.ProviderType == "" {
		return "openai"
	}
	return c.ProviderType
}

// OfflineEnabled is false if unset.
func (c Copilot) OfflineEnabled() bool { return c.Offline != nil && *c.Offline }

type Gateway struct {
	URL  string `toml:"url,omitempty"`
	Name string `toml:"name,omitempty"` // provider name in other tools' configurations
	// Project is the active project, whose key replaces the main key; empty:
	// the main key. Projects are all projects with a key in the keyring,
	// listed here because the keyring cannot be enumerated.
	Project  string   `toml:"project,omitempty"`
	Projects []string `toml:"projects,omitempty"`
	// Dirs binds folders to a project, which then replaces Project in the
	// folder and below: absolute path -> project, "none" for the main key.
	Dirs map[string]string `toml:"dirs,omitempty"`
}

// ProjectFor returns the binding that applies in dir, the one of the
// nearest bound folder at or above dir. dir must be absolute and clean.
func (g Gateway) ProjectFor(dir string) (project, boundDir string, ok bool) {
	for d := dir; ; {
		if p, found := g.Dirs[d]; found {
			return p, d, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", "", false
		}
		d = parent
	}
}

// Bind sets the project for a folder, replacing an earlier binding.
func (g *Gateway) Bind(dir, project string) {
	if g.Dirs == nil {
		g.Dirs = map[string]string{}
	}
	g.Dirs[dir] = project
}

// Unbind removes the binding of exactly this folder.
func (g *Gateway) Unbind(dir string) bool {
	if _, ok := g.Dirs[dir]; !ok {
		return false
	}
	delete(g.Dirs, dir)
	if len(g.Dirs) == 0 {
		g.Dirs = nil
	}
	return true
}

// HasProject reports whether the project was added.
func (g Gateway) HasProject(name string) bool { return slices.Contains(g.Projects, name) }

// AddProject records a project, keeping the list sorted.
func (g *Gateway) AddProject(name string) {
	if !g.HasProject(name) {
		g.Projects = append(g.Projects, name)
		slices.Sort(g.Projects)
	}
}

// RemoveProject forgets a project and its folders; where it was active, the
// main key applies.
func (g *Gateway) RemoveProject(name string) {
	for d, p := range g.Dirs {
		if p == name {
			g.Unbind(d)
		}
	}
	g.Projects = slices.DeleteFunc(g.Projects, func(p string) bool { return p == name })
	if len(g.Projects) == 0 {
		g.Projects = nil
	}
	if g.Project == name {
		g.Project = ""
	}
}

// Slots are Claude Code's model slots.
var Slots = []string{"opus", "sonnet", "haiku"}

type Claude struct {
	Default     string            `toml:"default,omitempty"` // slot name or model name
	Opus        string            `toml:"opus,omitempty"`
	Sonnet      string            `toml:"sonnet,omitempty"`
	Haiku       string            `toml:"haiku,omitempty"`
	AutoCompact *bool             `toml:"auto_compact,omitempty"`
	StatusLine  *bool             `toml:"statusline,omitempty"` // adds project and spend to the session's status line; nil: on
	Picker      *bool             `toml:"picker,omitempty"`     // all gateway models in /model; nil: on
	Env         map[string]string `toml:"env,omitempty"`        // nil: DefaultClaudeEnv
}

// ToolList returns the chosen tools.
func (c *Config) ToolList() []string {
	if c.Tools == nil {
		return nil
	}
	return *c.Tools
}

// ToolsConfigured reports whether tools were ever chosen, even none.
func (c *Config) ToolsConfigured() bool { return c.Tools != nil }

// SetTools stores the tools; an empty list means "none", nil means "unset".
func (c *Config) SetTools(tools []string) {
	if tools == nil {
		c.Tools = nil
		return
	}
	list := append([]string{}, tools...)
	c.Tools = &list
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

// StatusLineEnabled is true if unset.
func (c Claude) StatusLineEnabled() bool { return c.StatusLine == nil || *c.StatusLine }

// PickerEnabled is true if unset.
func (c Claude) PickerEnabled() bool { return c.Picker == nil || *c.Picker }

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
	// A gateway on this machine or by IP has no useful host name.
	if host := u.Hostname(); host == "localhost" || net.ParseIP(host) != nil {
		return "local"
	}
	labels := strings.Split(u.Hostname(), ".")
	for len(labels) > 2 && slices.Contains([]string{"api", "llm", "llms", "ai", "gateway", "proxy", "www"}, labels[0]) {
		labels = labels[1:]
	}
	return labels[0]
}

// Path returns the location of config.toml.
func Path() (string, error) {
	p, err := path()
	if err != nil {
		return "", err
	}
	return guard.Path(p)
}

func path() (string, error) {
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
	if err := atomicfile.Write(p, b, 0o600); err != nil {
		return false, err
	}
	return true, nil
}
