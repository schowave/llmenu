package config

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Key is a key for `llmenu config get/set`.
type Key struct {
	Name string
	Help string
	get  func(*Config) (value string, set bool) // set: present in the file
	def  func(*Config) string                   // default when not set
	put  func(*Config, string) error            // "" deletes the entry
}

// NoLaunch as launch: llmenu without a command only prints the model name.
const NoLaunch = "none"

// ValidLaunch checks a tool for launch; set by the cli, which knows the
// tools.
var ValidLaunch func(name string) error

const (
	envPrefix      = "claude.env."
	benchMapPrefix = "bench.map."
)

// Keys are all fixed keys; claude.env.<VARIABLE> comes on top.
var Keys = []Key{
	str("gateway.url", "URL of the LiteLLM gateway (required)",
		func(c *Config) *string { return &c.Gateway.URL }, nil),
	str("gateway.name", "provider name in other tools' configurations",
		func(c *Config) *string { return &c.Gateway.Name },
		func(c *Config) string { return NameFromURL(c.Gateway.URL) }),
	{
		Name: "gateway.project",
		Help: "active project, whose gateway key is used (empty: the main key); see llmenu project",
		get:  func(c *Config) (string, bool) { return c.Gateway.Project, c.Gateway.Project != "" },
		put: func(c *Config, v string) error {
			if v != "" && !c.Gateway.HasProject(v) {
				return fmt.Errorf("unknown project %q: llmenu project add %s", v, v)
			}
			c.Gateway.Project = v
			return nil
		},
	},
	str("claude.default", "default model: slot name (opus|sonnet|haiku) or model name",
		func(c *Config) *string { return &c.Claude.Default },
		func(*Config) string { return "opus" }),
	str("claude.opus", "model for the opus slot (empty: gateway suggestion)",
		func(c *Config) *string { return &c.Claude.Opus }, nil),
	str("claude.sonnet", "model for the sonnet slot (empty: gateway suggestion)",
		func(c *Config) *string { return &c.Claude.Sonnet }, nil),
	str("claude.haiku", "model for the haiku slot (empty: gateway suggestion)",
		func(c *Config) *string { return &c.Claude.Haiku }, nil),
	optBool("claude.auto_compact", "automatic compaction in Claude Code (true|false)",
		func(c *Config) **bool { return &c.Claude.AutoCompact }, "true"),
	optBool("claude.statusline", "add project and spend to Claude Code's status line in sessions llmenu starts; your own status line stays (true|false)",
		func(c *Config) **bool { return &c.Claude.StatusLine }, "true"),
	optBool("claude.picker", "list all gateway models in Claude Code's /model in sessions llmenu starts (true|false)",
		func(c *Config) **bool { return &c.Claude.Picker }, "true"),
	str("codex.default", "default model for Codex (required without a terminal if no -m is given)",
		func(c *Config) *string { return &c.Codex.Default }, nil),
	{
		Name: "bench.metric",
		Help: "benchmark for sorting: intel, tb4 or code",
		get:  func(c *Config) (string, bool) { return c.Bench.Metric, c.Bench.Metric != "" },
		def:  func(*Config) string { return "intel" },
		put: func(c *Config, v string) error {
			if v != "" && !slices.Contains(Metrics, v) {
				return fmt.Errorf("expected %v, not %q", Metrics, v)
			}
			c.Bench.Metric = v
			return nil
		},
	},
	{
		Name: "launch",
		Help: "tool llmenu without a command starts, or none to only print the model name; see llmenu default",
		get:  func(c *Config) (string, bool) { return c.Launch, c.Launch != "" },
		put: func(c *Config, v string) error {
			if v != "" && v != NoLaunch && ValidLaunch != nil {
				if err := ValidLaunch(v); err != nil {
					return err
				}
			}
			c.Launch = v
			return nil
		},
	},
	{
		Name: "tools",
		Help: "coding tools in use, comma-separated (claude, codex, opencode, pi), or none",
		get: func(c *Config) (string, bool) {
			return strings.Join(c.ToolList(), ","), c.ToolsConfigured()
		},
		// "" removes the entry (unset); "none" stores a deliberate empty list.
		put: func(c *Config, v string) error {
			if strings.TrimSpace(v) == "" {
				c.SetTools(nil)
				return nil
			}
			list := []string{}
			for _, t := range strings.Split(v, ",") {
				if t = strings.TrimSpace(t); t != "" && t != "none" {
					list = append(list, t)
				}
			}
			c.SetTools(list)
			return nil
		},
	},
}

// optBool is a boolean key that can be unset: nil uses def.
func optBool(name, help string, field func(*Config) **bool, def string) Key {
	return Key{
		Name: name,
		Help: help,
		get: func(c *Config) (string, bool) {
			if p := *field(c); p != nil {
				return strconv.FormatBool(*p), true
			}
			return "", false
		},
		def: func(*Config) string { return def },
		put: func(c *Config, v string) error {
			if v == "" {
				*field(c) = nil
				return nil
			}
			b, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("expected true or false, not %q", v)
			}
			*field(c) = &b
			return nil
		},
	}
}

func str(name, help string, field func(*Config) *string, def func(*Config) string) Key {
	return Key{
		Name: name,
		Help: help,
		get:  func(c *Config) (string, bool) { v := *field(c); return v, v != "" },
		def:  def,
		put:  func(c *Config, v string) error { *field(c) = v; return nil },
	}
}

// Entry is a value with its origin for `llmenu config list`.
type Entry struct {
	Key, Value, Source string // Source: "file" or "default"; empty when unset
}

// Get returns the effective value of a key.
func (c *Config) Get(name string) (Entry, error) {
	if v, ok := strings.CutPrefix(name, envPrefix); ok {
		val, set := c.Claude.Env[v]
		if c.Claude.Env == nil {
			val, set = DefaultClaudeEnv[v]
			return entry(name, val, set, "default"), nil
		}
		return entry(name, val, set, "file"), nil
	}
	if model, ok := strings.CutPrefix(name, benchMapPrefix); ok {
		v, set := c.Bench.Map[model]
		return entry(name, v, set, "file"), nil
	}
	k, err := find(name)
	if err != nil {
		return Entry{}, err
	}
	if v, set := k.get(c); set {
		return Entry{name, v, "file"}, nil
	}
	if k.def != nil {
		if d := k.def(c); d != "" {
			return Entry{name, d, "default"}, nil
		}
	}
	if slot, ok := strings.CutPrefix(name, "claude."); ok && slices.Contains(Slots, slot) {
		return Entry{Key: name, Source: "gateway suggestion"}, nil
	}
	return Entry{Key: name}, nil
}

func entry(key, val string, set bool, source string) Entry {
	if !set {
		return Entry{Key: key}
	}
	return Entry{key, val, source}
}

// Set sets a key; an empty value deletes the entry.
func (c *Config) Set(name, value string) error {
	if v, ok := strings.CutPrefix(name, envPrefix); ok {
		if v == "" {
			return fmt.Errorf("variable name missing: %s<NAME>", envPrefix)
		}
		if c.Claude.Env == nil {
			c.Claude.Env = maps.Clone(DefaultClaudeEnv)
		}
		if value == "" {
			delete(c.Claude.Env, v)
		} else {
			c.Claude.Env[v] = value
		}
		return nil
	}
	if model, ok := strings.CutPrefix(name, benchMapPrefix); ok {
		if model == "" {
			return fmt.Errorf("model name missing: %s<GATEWAY-MODEL>", benchMapPrefix)
		}
		if value == "" {
			delete(c.Bench.Map, model)
			return nil
		}
		if c.Bench.Map == nil {
			c.Bench.Map = map[string]string{}
		}
		c.Bench.Map[model] = value
		return nil
	}
	k, err := find(name)
	if err != nil {
		return err
	}
	return k.put(c, value)
}

// List returns all keys with their effective values.
func (c *Config) List() []Entry {
	var out []Entry
	for _, k := range Keys {
		e, _ := c.Get(k.Name)
		out = append(out, e)
	}
	env := c.Claude.EnvOrDefault()
	for _, v := range slices.Sorted(maps.Keys(env)) {
		e, _ := c.Get(envPrefix + v)
		out = append(out, e)
	}
	for _, m := range slices.Sorted(maps.Keys(c.Bench.Map)) {
		e, _ := c.Get(benchMapPrefix + m)
		out = append(out, e)
	}
	return out
}

func find(name string) (Key, error) {
	for _, k := range Keys {
		if k.Name == name {
			return k, nil
		}
	}
	return Key{}, fmt.Errorf("unknown key %q, see llmenu config list", name)
}
