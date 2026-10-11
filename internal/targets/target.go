// Package targets integrates tools: via sync (write the model list into their
// configuration) and/or launch (start them with gateway and model).
// See docs/ADR/0002-command-structure-and-targets.md.
package targets

import (
	"context"
	"errors"
	"os/exec"
	"sort"

	"github.com/schowave/llmenu/internal/catalog"
	"github.com/schowave/llmenu/internal/config"
)

// ErrNotImplemented marks parts of the scaffold that are still missing.
var ErrNotImplemented = errors.New("not implemented yet")

// Target is a tool llmenu knows. It additionally implements Syncer, Launcher
// or both.
type Target interface {
	Name() string        // name on the command line, e.g. "claude"
	Description() string // one line for help and `llmenu targets`
	Binary() string      // program name in PATH
}

// SyncOptions controls a sync run.
type SyncOptions struct {
	BaseURL  string // gateway URL as in the configuration
	Provider string // own provider entry in the other tool's configuration, from gateway.name (ADR-0003)
	DryRun   bool
}

// Diff describes what a sync changed or would change.
type Diff struct {
	Path    string
	Changed bool
	Patch   string // readable diff for --dry-run
}

// Syncer writes the model list into a tool's configuration.
type Syncer interface {
	Target
	ConfigPath() (string, error)
	Sync(ctx context.Context, models []catalog.Model, opt SyncOptions) (Diff, error)
}

// Env is everything a Launcher needs to start. The key only goes to the
// child process.
type Env struct {
	BaseURL string
	APIKey  string
	Config  *config.Config
	Models  []catalog.Model // all of the gateway's chat models, aliases included
}

// Launcher starts a tool with gateway and model.
type Launcher interface {
	Target
	// HasModelArg reports whether the passed-through arguments already set a
	// model. Then no menu opens.
	HasModelArg(args []string) bool
	// Supports reports whether the model works with the tool, and otherwise
	// returns a hint for the menu.
	Supports(m catalog.Model) (ok bool, hint string)
	Command(ctx context.Context, m *catalog.Model, args []string, env Env) (*exec.Cmd, error)
}

// Defaulter is optional: the target's default model. The menu cursor starts
// on it, and without a terminal it is used without asking.
// "" means: no known default.
type Defaulter interface {
	DefaultModel(env Env) string
}

// OwnSelection is optional: the tool lets the user choose a model itself,
// llmenu shows no menu at launch.
type OwnSelection interface {
	OwnSelection()
}

// KeyCommand prints the gateway key. Tools that support a command as key
// source get it instead of the key itself (ADR-0002).
const KeyCommand = "llmenu secrets get gateway"

// KeyEnv carries the key for tools that only support environment variables.
const KeyEnv = "LLMENU_API_KEY"

// Installed returns the tool's path if it is in PATH.
func Installed(t Target) (string, bool) {
	p, err := exec.LookPath(t.Binary())
	return p, err == nil
}

var registry = map[string]Target{}

func register(t Target) { registry[t.Name()] = t }

// All returns all targets, sorted by name.
func All() []Target {
	all := make([]Target, 0, len(registry))
	for _, t := range registry {
		all = append(all, t)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name() < all[j].Name() })
	return all
}

// Get returns a target by name.
func Get(name string) (Target, bool) {
	t, ok := registry[name]
	return t, ok
}

// hasFlag checks whether args contains one of the flags, also in the form
// --flag=value. Only args up to "--" count; positional arguments follow it.
func hasFlag(args []string, flags ...string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		for _, f := range flags {
			if a == f || (len(a) > len(f) && a[:len(f)+1] == f+"=") {
				return true
			}
		}
	}
	return false
}
