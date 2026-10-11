# ADR-0002: Command structure with targets that sync models or launch tools

- **Status:** Proposed
- **Date:** 2026-10-11
- **Deciders:** David Schowalter
- **Builds on:** [ADR-0001](0001-cross-platform-cli-in-go.md)

## Context and Problem Statement

Today each tool has its own functions: `claude-api` and `claude-pick` for
Claude Code, `pi-pick` for pi. Plus an alias for strix with a hard-wired
model. Each function decides on its own how the model gets into the tool.

Yet there are only two fundamentally different kinds of tools:

- **Tools with their own model configuration** such as pi and opencode. They
  keep a provider and its model list in a configuration file and have their
  own model selection. For them, the work is keeping that list up to date
  (for pi, that is one half of `pi-pick` today). Launching them is trivial.
- **Tools without a suitable configuration** such as Claude Code. Gateway,
  token and model can only be passed in via environment variables or flags
  at startup. They cannot be set globally, because that would also affect
  Claude Code when used without the gateway, i.e. with the subscription.

More tools are to be added in the medium term. First Claude Code, pi,
opencode and, if possible, Codex CLI.

Question: **How do we structure the commands of `llmenu` and the code behind
them so that new tools can be added with little effort and daily use stays
consistent?**

## Decision Drivers

1. **A new tool is an extension,** not a change across the codebase.
2. **A tool's own model selection is used where it exists.** `llmenu` should
   not get between the user and the tool when that is not necessary.
3. **Pass-through without surprises:** arguments for the tool arrive
   unchanged, without collisions with `llmenu` flags (`claude -c`, `-p`,
   `-r` …).
4. **Other tools' configuration files stay intact:** only our own entries
   are changed, comments and formatting are preserved, runs are repeatable,
   and changes can be previewed.
5. **No plaintext keys in other tools' configuration files.**
6. **Usable in scripts too:** without a terminal, nothing may wait for input.

## Considered Options

1. **One hard-coded subcommand per tool,** as in `.zshrc` today
   (`llmenu claude`, `llmenu pi`, …, each with its own logic).
2. **Always launch:** `llmenu` launches every tool itself and passes model
   and gateway via environment variables or flags.
3. **Always sync:** `llmenu` only writes configuration files.
4. **Targets with two capabilities:** each tool is a *target* that supports
   *sync*, *launch* or both. The commands follow from the capabilities.
5. **Plugins as external programs** (`llmenu-<target>` on the `PATH`, as with
   git or kubectl).

## Decision

We chose **Option 4: targets with the capabilities sync and launch,**
compiled in for now.

- Option 3 is ruled out because Claude Code does not work without being
  launched via `llmenu` (see context).
- Option 2 violates driver 2: pi and opencode have their own good model
  selection that only needs an up-to-date list.
- Option 1 does not satisfy driver 1. Each tool brings its own logic, and
  the differences grow, as they do today between `claude-pick` and
  `pi-pick`.
- Option 5 is too much effort for four targets. It remains an open path in
  case third-party targets are to be added (see "More Information").

### Commands

| Command | Purpose | Replaces |
|---|---|---|
| `llmenu` | Open the menu, print the selected model name to stdout. This way any tool without its own target can be integrated: `strix --model "$(llmenu)"` | `_gateway-pick` |
| `llmenu run <target> [args…]` | Launch the tool with gateway and model (only targets with launch) | `claude-api`, `claude-pick` |
| `llmenu claude [args…]`, `llmenu codex [args…]` | Shorthands for `run`, one per launch target | |
| `llmenu sync [target…] [--dry-run]` | Write the model list into the targets' configuration; without arguments, the tools in use (setting `tools`, ADR-0003) that keep a model list. `--dry-run` shows the diff | Configuration part of `pi-pick` |
| `llmenu bench` | Refresh benchmarks. No automatic `sync`: pi and opencode do not store benchmark values | `claude-benchmark` |
| `llmenu list [--json]` | Table without menu | new |
| `llmenu secrets set\|get\|delete <name>` | Keys in the OS keychain | `creds` |
| `llmenu targets` | Targets with capabilities, installation status and configuration path | new |

### Rules for launch

- Everything after the target name is passed to the tool **unchanged**.
  `run` and the shorthands have no flags of their own, so there can be no
  collisions. Settings for `llmenu` itself come from the configuration file
  or from environment variables (`LLMENU_*`).
- If the arguments already contain the tool's model flag (`--model` for
  Claude Code, `-m`/`--model` for Codex), no menu opens.
- Otherwise the menu opens with the cursor on the target's default model
  (see [ADR-0003](0003-configuration-and-setup.md)). A single Enter is
  enough. If the target has no default, the cursor is on the most recently
  selected model.
- Without a terminal on stdin or stdout, the default model is always used
  without a menu, never the one last selected interactively. That way the
  behavior of scripts does not depend on the last selection.
- Keys are only passed to the child process as environment variables.

### Rules for sync

- In the other tool's configuration, a target changes **only its own
  provider entry** (name configurable, default `mygateway`) and leaves
  everything else unchanged.
- The file is only written if something changes, and then atomically
  (temporary file, then rename) with a `*.bak` backup of the previous
  version.
- JSONC files (comments, trailing commas) are edited so that comments are
  preserved, e.g. with `tailscale/hujson`. If that is not possible, `sync`
  aborts instead of deleting comments.
- Models that the gateway no longer offers are removed from our own
  provider.
- **Keys** are entered as a reference, never in plaintext: as a command
  (`llmenu secrets get gateway`) or as the name of an environment variable,
  depending on what the tool supports. If it supports neither, the target
  additionally gets launch, and the key is passed in at startup.

### Targets in the first step

| Target | Capability | Implementation | Status |
|---|---|---|---|
| **Claude Code** | Launch | Environment variables as in `claude-api` today (base URL, token, Opus/Sonnet/Haiku mapping, flags), model via `--model`, suffix `[1m]` from 900k context | implemented |
| **pi** | Sync | Provider `mygateway` in `~/.pi/agent/models.json` with context, max output, vision, reasoning and prices. Key via `!llmenu secrets get gateway`. pi already supports `!command`; today it contains `!~/credentials/creds …` | implemented |
| **opencode** | Sync + Launch | Provider (name from `gateway.name`) in the global opencode configuration (`~/.config/opencode/opencode.json[c]`) via `@ai-sdk/openai-compatible`, models with `limit`, `cost`, `attachment`, `reasoning`. For the key, opencode only knows `{env:…}`/`{file:…}`, not a command; so it contains `{env:LLMENU_API_KEY}`, and `llmenu opencode` sets the variable at startup (without its own menu, opencode picks the model itself) | implemented; path on Windows still to be verified |
| **Codex CLI** | Launch | Provider entirely via `-c` overrides at startup: `model_provider=<id>` and `model_providers.<id>={name, base_url=<gateway>/v1, env_key="LLMENU_API_KEY", wire_api="responses"}`. The model is passed as `-c model="<name>"` plus `model_context_window` and `model_max_output_tokens` instead of `-m`, so it also applies to subcommands such as `codex exec`. `~/.codex/config.toml` stays untouched. Optional `codex.default` in the configuration as the default model (required without a terminal unless `-m` is given) | implemented; spike succeeded, see below |

**Codex CLI spike result:** Codex speaks the Responses API
(`/v1/responses`) and no longer supports Chat Completions. Before building
the target, a short spike checked the three conditions; all of them hold:

1. The gateway serves `/v1/responses`.
2. `codex exec` works through it, including tool calls (shell commands), for
   GPT models and, via LiteLLM's translation, also for Claude models. For the
   latter, Codex warns that it has no model metadata.
3. The `-c` overrides are sufficient; nothing has to be written to
   `config.toml`.

The key goes only into the child process's environment as
`LLMENU_API_KEY`. In the menu, models for which the gateway does not report
`/v1/responses` are marked (via `Supports`), not hidden, because they
usually still work through LiteLLM's translation.

### Code structure

```go
// internal/targets
type Target interface {
    Name() string              // "claude", "pi", "opencode", "codex"
    Installed() (path string, ok bool)
}

type Syncer interface {        // optional
    Sync(ctx context.Context, models []catalog.Model, opt SyncOptions) (Diff, error)
}

type Launcher interface {      // optional
    HasModelArg(args []string) bool
    Supports(m catalog.Model) (ok bool, hint string)  // e.g. Codex: mark models without Responses API
    Command(ctx context.Context, m catalog.Model, args []string, env Env) (*exec.Cmd, error)
}
```

- Each target is a file under `internal/targets/` and registers itself in a
  registry. The commands `run`, `sync` and `targets` as well as the
  shorthands are generated from the registry.
- `Supports` lets the menu mark or hide models per target, e.g. non-Claude
  models for Claude Code. This is a finding from the review of the zsh
  functions.
- Tests: for each sync target a golden-file test (input configuration plus
  models yields the expected configuration), including comments and other
  providers. For each launch target a test of the generated arguments and
  environment variables, without starting the tool.

## Consequences

### Positive

- A new tool is one file plus tests. Commands, help and completion follow
  automatically.
- pi and opencode are started without `llmenu` and keep their own user
  interface. `llmenu` only runs when something changes on the gateway.
- `llmenu` without a subcommand also integrates tools without their own
  target (strix, scripts).
- No more plaintext keys in other tools' configuration files. The global
  `GATEWAY_API_KEY` in `.zshrc` and the `omp` alias become unnecessary.

### Negative

- `llmenu` writes to other tools' configuration files. If a tool changes its
  format, the target breaks. The golden-file tests only catch this for known
  formats.
- For sync targets, the tool's model selection lacks the benchmark values.
  Anyone who wants to see them uses `llmenu list`.
- Because `run` has no flags of its own, `llmenu` can only be controlled per
  invocation via environment variables.

### Neutral / Follow-ups

- Codex CLI spike: done, see above. Codex has no metadata for models it
  does not know (e.g. Claude models via LiteLLM) and warns about it; context
  window and max output are therefore passed via `-c`.
- Verify the opencode configuration path on Windows.
- Further candidates for targets, deliberately not implemented yet: aider
  (sync of `.aider.model.metadata.json` plus launch), Crush, omp (probably
  like pi), Continue, Zed, strix.

## Confirmation

- `llmenu claude`, `llmenu claude -c` and
  `llmenu claude --model sonnet -p "hi"` behave like today's `claude-pick`,
  `claude-api -c` and `claude-api --model sonnet -p "hi"`.
- After `llmenu sync`, pi and opencode show all chat models of the gateway in
  their own selection. A second run changes nothing. Comments and other
  providers in the files are preserved.
- `grep` finds no plaintext API key in the configurations of pi and
  opencode.
- `llmenu codex exec "…"` runs a task with tool calls through the gateway
  without changing `~/.codex/config.toml`.

## Pros and Cons of the Options

### Option 1: One hard-coded subcommand per tool

- Good: simplest for the first two tools.
- Bad: each tool brings its own logic for menu, pass-through and keys, and
  that logic drifts apart.

### Option 2: Always launch

- Good: consistent, no other tools' files are touched.
- Bad: replaces the own model selection of pi and opencode. pi only knows
  models from its configuration, so it cannot do without sync anyway.

### Option 3: Always sync

- Good: `llmenu` runs rarely, the tools stay untouched.
- Bad: not possible for Claude Code without affecting use with the
  subscription.

### Option 4: Targets with sync and launch (chosen)

- Good: each tool is integrated in the way that suits it best. New targets
  are self-contained.
- Bad: two mechanisms to understand. Write access to other tools' files.

### Option 5: Plugins as external programs

- Good: targets could be developed and published independently of `llmenu`.
- Bad: a stable protocol between `llmenu` and the plugins would be needed,
  plus versioning and distribution per plugin. For four in-house targets
  that is disproportionate.

## More Information

- Tools integrated in `~/.zshrc` today: `claude-api`, `claude-pick`,
  `pi-pick`, the aliases `strix-api` and `omp`
- Codex CLI configuration: <https://github.com/openai/codex/blob/main/docs/config.md>
- opencode providers: <https://opencode.ai/docs/providers/>
- LiteLLM Responses API: <https://docs.litellm.ai/docs/response_api>
- hujson (JSONC with comments preserved): <https://github.com/tailscale/hujson>
- Revisit: if third parties are to contribute targets (then option 5) or if
  a tool gets its own gateway configuration that makes launch unnecessary.
