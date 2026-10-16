# ADR-0003: Configuration without vendor-specific defaults, set up via a repeatable setup

- **Status:** Proposed
- **Date:** 2026-10-11
- **Deciders:** David Schowalter
- **Builds on:** [ADR-0001](0001-cross-platform-cli-in-go.md), [ADR-0002](0002-command-structure-and-targets.md)

## Context and Problem Statement

So far, the settings are hard-wired in the code or in `.zshrc`:

- the gateway URL `https://api.mygateway.example.com/`,
- the models for the Claude Code slots
  (`aws/claude-opus-latest[1m]`, …, Haiku on `azure/gpt-5.6-luna`),
- flags such as `DISABLE_AUTO_COMPACTION=1`,
- keys via `creds` under names such as `gateway-api-key`.

However, `llmenu` is supposed to work with any LiteLLM gateway and be
published. At other companies the models have different names
(`anthropic/claude-opus-4-6`, `bedrock-opus`, …) or come from other
vendors. Built-in defaults for one particular gateway would be wrong there.

In addition, it should be possible to change settings later, such as the
default model, without losing the others.

Question: **Where and in what form are the settings stored, where do
sensible defaults come from, and how is `llmenu` set up and changed later?**

## Decision Drivers

1. **Generic:** no names of gateways, vendors or models hard-coded in the
   program.
2. **Repeatable:** the setup can be run again at any time. Existing values
   are the defaults; Enter keeps them.
3. **Individual values can be changed without a dialog,** also in scripts
   and during rollouts.
4. **Secrets only in the OS keychain** (ADR-0001).
5. **Scripts never hang** on a prompt.
6. **Little friction on first start:** anyone who runs `llmenu claude`
   without having set it up is guided through setup instead of being
   turned away.

## Considered Options

**A. Source of the defaults**

1. Build in defaults for one particular gateway.
2. No defaults, everything has to be entered.
3. **Derive defaults at runtime from the gateway's model list.**

**B. Setup flow**

1. Only a configuration file that is edited by hand.
2. **`llmenu setup` as a repeatable dialog, plus `llmenu config` for
   individual values.**
3. Setup starts automatically as soon as any setting is missing.

## Decision

We chose **A3 and B2**. In addition, there is a prompt that offers the setup
only when a *required* setting is missing and a terminal is attached.

### Storage

| What | Where |
|---|---|
| Settings | `config.toml` under `os.UserConfigDir()/llmenu/` (macOS: `~/Library/Application Support/llmenu/`, Linux: `~/.config/llmenu/`, Windows: `%AppData%\llmenu\`) |
| Gateway key, Artificial Analysis key | Keychain, service `llmenu`, entries `gateway` and `artificialanalysis` |
| Most recently selected model | Cache, as before (`state.json`) |

Environment variables take precedence over the file and the keychain:
`LLMENU_BASE_URL`, `LLMENU_API_KEY`, `LLMENU_AA_API_KEY`. This is intended
for CI and for trying things out. For tests and the development sandbox
([ADR-0004](0004-testing-strategy.md)) there are also `LLMENU_CONFIG`,
`LLMENU_CACHE_DIR`, `LLMENU_AA_URL` and `LLMENU_KEYRING_FILE`; the latter
stores keys in a plain-text file instead of the OS keyring and is not meant
for real keys.

`tools` lists the coding tools in use (claude, codex, opencode, pi). The
setup only asks the claude and codex sections for tools in use, `llmenu sync`
without arguments writes into the tools in use that keep a model list, and
the help hides the launch commands of tools not in use. `tools` distinguishes
"never configured" (then only Claude Code is preselected, whatever is
installed) from a deliberate empty
selection, which is written as `tools = []` and kept on the next setup.

`llmenu` writes the file. Comments added by hand are lost the next time it
is written. A comment at the top of the file points this out.

### Schema

```toml
tools = ["claude", "pi"]            # coding tools in use

[gateway]
url  = "https://llm.example.com/"   # required
name = "example"                    # provider name in other tools' configurations (sync)

[claude]
default = "opus"                    # slot name (opus|sonnet|haiku) or a model name
opus    = "…"                       # suggestion from the gateway, see below
sonnet  = "…"
haiku   = "…"
auto_compact = true                 # false sets DISABLE_AUTO_COMPACTION=1
statusline = true                   # false: no project and spend in the status line (ADR-0005)
picker = true                       # false keeps Claude's own /model list (ADR-0002)

[claude.env]                        # any additional variables for Claude Code
CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC = "1"

[codex]
default = "…"                       # optional; default model for llmenu codex (ADR-0002)

```

The only built-in defaults are those that apply to every gateway:
`auto_compact = true` (as in Claude Code itself) as well as
`CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` and
`CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS` in `[claude.env]`, because gateways
usually cannot handle Anthropic's telemetry and beta headers.

### Defaults from the gateway

For each slot, the setup suggests a model from the list returned by
`/model/info`:

1. Candidates are models whose target model contains the slot name as a
   token, i.e. `opus`, `sonnet` or `haiku`. Matching uses the same
   normalization as the menu.
2. If there is an alias with `latest` in its name, it is preferred, because
   it follows new versions without the setup having to run again.
3. Otherwise the highest version is chosen (numbers in the name compared in
   order), with ties broken by the same rules as when removing duplicates.
4. If there is no candidate, the setup suggests nothing and the user has to
   choose.

The setup offers all models *including* aliases, with the `-latest` aliases
first. A non-Claude model in a slot is allowed but produces a warning.

`gateway.name` is derived from the host: prefixes such as `api.` or `llm.`
are removed and the first remaining part is taken. So
`api.mygateway.example.com` becomes `mygateway`.

### Default model

- `claude.default` refers to a slot (default: `opus`) or names a model
  directly. An alias is resolved afresh on every start, so the default
  follows new versions.
- In the menu, the cursor is on the default (ADR-0002, changed). The most
  recently selected model only counts for targets without a default and for
  `llmenu` without a target.
- Without a terminal, the default always applies.

### Commands

| Command | Purpose |
|---|---|
| `llmenu setup` | ask what to set up (everything or one section); on the very first run everything without asking. Defaults are the current values, otherwise the derived suggestions |
| `llmenu setup <section>` | only `gateway`, `tools`, `claude`, `codex` or `benchmarks` |
| `llmenu config list` | all values and where they come from (file, environment, default); keys only shown as "set" |
| `llmenu config get <key>` | single value, e.g. `claude.default` |
| `llmenu config set <key> <value>` | change a single value without a dialog |
| `llmenu config path` | path of the file |
| `llmenu secrets set\|get\|delete <name>` | keys in the keychain; `set` prompts for the value without echoing it |

### Required settings and prompt

Only the gateway URL and the gateway key are required. If one of them is
missing:

- **with a terminal:** "llmenu is not set up yet. Set it up now? [Y/n]". On
  the very first run (no config file yet) the whole setup runs — gateway,
  tools, Claude Code defaults, benchmark key — otherwise only the missing
  `gateway` section. Then the original command continues.
- **without a terminal:** abort with a hint to run `llmenu setup`.
- **accessible mode** (`LLMENU_ACCESSIBLE=1`): all prompts are plain
  line-based questions on stderr with numbered options, read from stdin,
  also without a terminal. Meant for screen readers; the tests use it to
  drive the setup. Only this explicit opt-in reads stdin without a terminal,
  so scripts still never hang.

Benchmarks: entering the Artificial Analysis key in the setup fetches them
right away. With a key, benchmarks that are missing or older than 14 days are
refreshed automatically before the menu or `llmenu list` opens — at most one
attempt a day, with a 3-second timeout; a failure is silent and the old
values stay. Without benchmarks, the menu's footer suggests `llmenu bench`.

If the gateway offers no model for a Claude Code slot, the setup offers
"not set" first, so Enter does not silently store some other model.

Missing *optional* settings never trigger a prompt. If the Claude Code slots
are missing, they are suggested from the gateway at startup, as in the
setup.

The setup checks the gateway URL and key immediately with `/model/info`. A
typo is therefore caught during setup and not only on the next start.

## Consequences

### Positive

- `llmenu` works with any LiteLLM gateway without code changes. Other
  companies get sensible suggestions from their own gateway.
- Settings can be changed individually at any time (`config set`) or
  revisited section by section (`setup claude`).
- The first start guides through setup; scripts are not affected by it.

### Negative

- The suggestions are a heuristic. For unusual model names they stay empty
  or do not fit and have to be corrected by hand.
- Hand-written comments in `config.toml` do not survive `llmenu` writing the
  file.
- Additional dependencies: `charmbracelet/huh` (forms),
  `pelletier/go-toml/v2`, `zalando/go-keyring`.

### Neutral / Follow-ups

- The previous values from `.zshrc` (Haiku on GPT Luna, no automatic
  compaction) are carried over once during migration via `llmenu setup` or
  `llmenu config set`.
- Possible later: `llmenu setup --from <file|url>` takes a template provided
  by the company (gateway URL, slots) as defaults. That would allow a rollout
  to many people without a dialog.

## Confirmation

- Unit tests for the suggestions: with the gateway test data, the Opus slot
  picks the `-latest` alias. Without an alias, the highest version is
  chosen; without candidates, nothing.
- A second run of `llmenu setup` in which only Enter is pressed does not
  change `config.toml`.
- `llmenu claude` without a configuration asks about setup in the terminal.
  With `</dev/null` it aborts with a hint.
- The program code (outside of tests and test data) contains no gateway host
  and no model name of a specific vendor.

## More Information

- huh: <https://github.com/charmbracelet/huh>
- go-toml: <https://github.com/pelletier/go-toml>
- go-keyring: <https://github.com/zalando/go-keyring>
- Model for the repeatable setup: `aws configure`
