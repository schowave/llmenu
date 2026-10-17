<h1 align="center">llmenu</h1>

<p align="center">
  <strong>Pick an LLM from your LiteLLM gateway — by price, context and benchmarks — and use it in Claude Code, Codex CLI, GitHub Copilot CLI, pi and opencode.</strong>
</p>

<p align="center">
  <a href="https://github.com/schowave/llmenu/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/schowave/llmenu/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/schowave/llmenu/actions/workflows/codeql.yml"><img alt="CodeQL" src="https://github.com/schowave/llmenu/actions/workflows/codeql.yml/badge.svg"></a>
  <a href="https://github.com/schowave/llmenu/releases/latest"><img alt="Latest release" src="https://img.shields.io/github/v/release/schowave/llmenu"></a>
  <a href="go.mod"><img alt="Go version" src="https://img.shields.io/github/go-mod/go-version/schowave/llmenu"></a>
  <a href="https://goreportcard.com/report/github.com/schowave/llmenu"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/schowave/llmenu"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/github/license/schowave/llmenu"></a>
</p>

---

**llmenu** (pronounced *L-L-menu*) is for teams that reach their models through a
[LiteLLM](https://github.com/BerriAI/litellm) gateway: dozens of models from
several providers, aliases like `claude-opus-latest`, different prices and
context sizes — and a coding tool that wants all of that configured in its own
way. llmenu reads the model list from the gateway, lets you compare the
models, and does the wiring for you.

<p align="center">
  <img alt="llmenu default claude makes Claude Code the default tool, llmenu project use acme --here binds the project acme to the folder; then llmenu alone opens a menu of the gateway's models with context size, prices and benchmark scores, the project and its budget; Tab changes the sort column, typing searches, Enter starts Claude Code with the chosen model" src="docs/demo.gif" width="900">
  <br>
  <sub>Recorded against a fake gateway with sample prices and scores (<code>mise run demo</code>).</sub>
</p>

Pick a model, press Enter, and Claude Code starts against your gateway with
that model.

## Why llmenu

- **Choose, don't guess.** Which of the gateway's models is the best for
  coding — and what does it cost? The menu shows context size, input and
  output prices and benchmark scores side by side, sortable by quality or
  price.
- **Zero configuration.** Without llmenu you export a gateway URL, a key and
  one model variable per slot, in a different format for each tool. With it:
  `llmenu claude`. Your `~/.claude`, `~/.codex` and `~/.copilot` stay untouched.
- **See what you spend.** The menu and Claude Code's status line show the
  budget of the key you are using — yours or the project's — and how much of
  it is gone.
- **One gateway, five tools.** Claude Code, Codex CLI and Copilot CLI are
  started with the chosen model; pi and opencode get the gateway's models,
  limits and prices written into their configs. When the gateway changes,
  `llmenu sync` brings every tool up to date.

> **Requires** a LiteLLM gateway (URL and API key). llmenu does not talk to
> model providers directly. The budget display also needs the gateway to let
> keys read `/key/info`; without it, everything else works as before.

## Details

- **Benchmarks** from [Artificial Analysis](https://artificialanalysis.ai)
  (Intelligence Index, Terminal-Bench, Coding Index); aliases are folded
  together, the same model at several providers (e.g. aws/ and gcp/) stays
  selectable. Nothing is hard-coded: the suggestions for
  Claude Code's opus/sonnet/haiku slots come from the gateway's model list.
- **Safe config edits:** in pi and opencode only llmenu's own provider entry
  changes; comments and other providers stay, and a backup is kept.
- **Keys stay in the OS keyring** (macOS Keychain, Windows Credential Manager,
  Secret Service), never in config files or process arguments, and are only
  sent to the gateway they were stored for. See [SECURITY.md](SECURITY.md).
- **Projects:** one gateway key per project or cost center, switched with
  `llmenu project`.
- **Scriptable:** `llmenu list --json`, `strix --model "$(llmenu)"`; without a
  terminal nothing ever waits for input.
- **Single binary** for macOS, Linux and Windows on amd64 and arm64.

## Installation

**Homebrew** (macOS, Linux)

```sh
brew install --cask schowave/tap/llmenu
```

**Scoop** (Windows)

```powershell
scoop bucket add schowave https://github.com/schowave/scoop-bucket
scoop install llmenu
```

**Go** (1.27.2 or later)

```sh
go install github.com/schowave/llmenu/cmd/llmenu@latest
```

Or download an archive from the [releases page](https://github.com/schowave/llmenu/releases).
Archives come with signed build provenance; check one with
`gh attestation verify <archive> --repo schowave/llmenu`.

<details>
<summary>Shell completion</summary>

Tab completes commands, settings, key names and model names. Homebrew sets it
up automatically; with Scoop or `go install`, set it up once for your shell:

```sh
llmenu completion zsh > "${fpath[1]}/_llmenu"                                       # zsh
llmenu completion bash > ~/.local/share/bash-completion/completions/llmenu          # bash
llmenu completion fish > ~/.config/fish/completions/llmenu.fish                     # fish
```

```powershell
Add-Content $PROFILE 'llmenu completion powershell | Out-String | Invoke-Expression'   # PowerShell
```

</details>

## Quick start

```sh
llmenu setup     # gateway URL and key (checked right away), your tools, default models
llmenu claude    # pick a model and start Claude Code
llmenu           # the same with your default tool, see below
llmenu bench     # optional: benchmark scores, so the menu can sort by quality
```

`llmenu` without a command opens the menu and then starts your default tool,
so you need not type `llmenu claude` every time. The setup asks for the
default (or `llmenu` asks once); change it with `llmenu default`, e.g.
`llmenu default codex`, or `llmenu default --none` to only print the model
name. Captured as in `"$(llmenu)"`, it always prints only the name.

`setup` can be run again any time: it asks what to change (everything, or
e.g. only Claude Code's models), and Enter keeps the current value. If you
skip it, the first command offers it. The benchmark key is free; the setup
asks for it, and afterwards llmenu refreshes the scores every two weeks.

## Using llmenu with your tools

llmenu integrates each tool the way that tool works best: some are
**started** by llmenu with the chosen model, others get the gateway's models
**written into their configuration** and then work on their own.

| Tool | What llmenu does | You run |
|---|---|---|
| [Claude Code](https://github.com/anthropics/claude-code) | starts it with the chosen model | `llmenu claude` |
| [Codex CLI](https://github.com/openai/codex) | starts it with the chosen model | `llmenu codex` |
| [GitHub Copilot CLI](https://github.com/github/copilot-cli) | starts it with the chosen model | `llmenu copilot` |
| [opencode](https://opencode.ai) | writes the models into its config; starts it with the key | `llmenu sync`, then `llmenu opencode` |
| [pi](https://www.npmjs.com/package/@earendil-works/pi-coding-agent) | writes the models into its config | `llmenu sync`, then `pi` |
| anything else | prints the chosen model name | `tool --model "$(llmenu)"` |

```sh
llmenu claude                      # menu with the cursor on your default, then Claude Code starts
llmenu claude --model sonnet -c    # no menu; every argument goes to claude unchanged
llmenu claude -p "explain this" > answer.md   # no terminal: default model, no menu
llmenu codex exec "fix the failing test"
llmenu copilot -p "fix the failing test"
```

Gateway, key and models are passed in the environment (Claude Code, Copilot
CLI) or via `-c` overrides (Codex CLI); `~/.claude`, `~/.codex` and
`~/.copilot` stay untouched. Defaults: `llmenu setup claude`,
`llmenu setup codex`, `llmenu setup copilot`.

Copilot CLI uses the gateway as its own provider (BYOK) and needs no GitHub
login then. It speaks Chat Completions to `<gateway>/v1` by default;
`llmenu config set copilot.provider_type anthropic` switches to the Messages
API. `copilot.offline true` also turns off telemetry, web tools and
auto-update; a `COPILOT_OFFLINE` from your shell does not count. A
`~/.copilot/providers.json` that declares providers wins over what llmenu
passes; llmenu warns at startup.

In a Claude Code session started by llmenu, `/model` lists all of the
gateway's models with context, prices and benchmark instead of Claude's own,
so you can switch without restarting. Pick with **`s`** (this session only):
**Enter** makes Claude Code save the gateway model as its default in
`~/.claude/settings.json`, and a plain `claude` on your subscription then fails
until you pick another model with `/model` (see
[Troubleshooting](#troubleshooting)). To keep Claude's own list:
`llmenu config set claude.picker false`.

```sh
llmenu sync --dry-run    # opencode and pi: show what would change
llmenu sync              # write the gateway's models, limits and prices
llmenu opencode          # start opencode with the gateway key
pi                       # pi asks for the key itself: !llmenu secrets get gateway
```

Only llmenu's own provider entry is changed; comments and your other
providers stay, and the previous file is kept as `*.bak`. Run `llmenu sync`
again when the gateway's models change.

## Projects: one key per project

If your gateway hands out a separate key per project, e.g. to bill each
customer or cost center on its own, store them side by side and switch with
one command. URL and models stay the same; only the key changes.

```sh
llmenu project add acme        # asks for the key hidden, checks it with the gateway
llmenu project                 # pick: main key, acme, …, or add a project
llmenu project use acme        # or switch directly
llmenu project use --none      # back to the main key
llmenu project use acme --here # only in this folder and below
llmenu project unpin           # the folder follows the global choice again
LLMENU_PROJECT=acme llmenu claude   # just for this command or shell
```

A folder can have its own project: outside your home directory,
`llmenu project` asks after the choice whether it applies everywhere or only
in the current folder. Choosing again in the same folder replaces its
project. The binding lives in llmenu's config, not in the repository, and
`llmenu project list` shows all bound folders.

The switch holds for every later command and for pi; there is nothing to sync
again. Running sessions keep their key. The active project is shown in the
menu and when a tool starts (`llmenu: project acme · aws/claude-opus-5-5`).
`llmenu project current` prints it for your own prompt without touching the
keyring or the network.

<details>
<summary>The project in your shell prompt</summary>

In your own prompt or status line script:

```sh
project=$(llmenu project current) && [ -n "$project" ] && printf 'project %s · ' "$project"
```

starship (`~/.config/starship.toml`):

```toml
[custom.llmenu]
command = "llmenu project current"
when = "llmenu project current | grep -q ."
format = "[$output]($style) "
```

</details>

## Budget and spend

LiteLLM gateways often give each person and each project a monthly budget.
llmenu shows the budget that applies to the key in use, and that key's share:

- **in the menu**, next to the project,
- **in `llmenu project list`**, for the main key and every project,
- **in Claude Code's status line**, in sessions started with `llmenu claude`.
  Your own status line keeps running; llmenu adds one line below it, e.g. in
  a session with project acme:

  ```text
  project acme · team budget $40.00 / $100.00 · this key $12.45 · resets in 19d
  ```

  To turn it off: `llmenu config set claude.statusline false` (or
  `llmenu setup claude`).

  Without your own status line, llmenu's line also names the model.

A project's key spends from its team's budget, your personal key from your
user budget; a key's own budget is shown instead if it has less left. Without
any budget, llmenu shows what the key has spent (`$3.20 spent`).

The numbers come from LiteLLM's `/key/info` and `/user/info`, which every key
may ask about itself, so no admin rights are needed. They are LiteLLM's
estimate from its prices, not the provider's invoice. llmenu asks the gateway
at most once a minute and caches the result without the key.

## Models and benchmarks

```sh
llmenu list                        # table, best benchmark first
llmenu list --sort price --all     # by price, including aliases and duplicates
llmenu list --json                 # for scripts
llmenu bench                       # refresh benchmark scores
```

In the menu, type to search, **Tab** switches the sort column (INTEL, TB4,
CODE, price) and Enter picks. The preview shows details and notes when a model
does not suit the tool (e.g. a non-Claude model in `llmenu claude`).

## Configuration

```sh
llmenu config list                         # all settings and where they come from
llmenu config set claude.default sonnet    # change a single setting
llmenu secrets set gateway                 # store a key (read hidden, never as an argument)
llmenu targets                             # supported tools, install status, config paths
```

Settings live in `config.toml` (`llmenu config path`), keys in the OS keyring.
Environment variables override both, e.g. in CI — see `llmenu help environment`:

| Variable | Purpose |
|---|---|
| `LLMENU_BASE_URL`, `LLMENU_API_KEY` | gateway URL and key (the key wins over a project's) |
| `LLMENU_PROJECT` | project whose key to use, `none` for the main key |
| `LLMENU_AA_API_KEY` | Artificial Analysis key |
| `LLMENU_CONFIG`, `LLMENU_CACHE_DIR` | alternative config file and cache directory |
| `LLMENU_ACCESSIBLE=1` | line-based setup prompts, e.g. for screen readers |

`llmenu help` lists all commands, `llmenu help <command>` explains one.

## How it works

llmenu calls the gateway's `/model/info`, normalizes model names across
providers (`bedrock/eu.anthropic.claude-sonnet-4-5-20250929-v1:0` and
`vertex_ai/claude-sonnet-4-5@20250929` are the same model), folds aliases and
duplicates and matches benchmark results. Budgets come from `/key/info` and
`/user/info`. The design decisions are recorded in [docs/ADR](docs/ADR).

## Troubleshooting

**"Gateway check failed: HTTP 401"** during setup: the gateway does not
accept the key. Check that the URL is the gateway itself (not its admin UI)
and that the key is not expired or blocked.

**"the gateway key was stored for …, but the gateway URL now points to …"**:
llmenu only sends a key to the gateway it was stored for. If the move is
intended, confirm it with `llmenu config set gateway.url <new URL>`.

**No budget in the menu or status line**: the gateway does not let keys read
`/key/info` (or `/user/info` for user and team budgets). Check with
`curl -H "Authorization: Bearer $KEY" <gateway>/key/info`; llmenu shows
whatever the gateway does answer.

**"There's an issue with the selected model (aws/…)" in a plain `claude`**:
the model was picked with Enter in a session started by llmenu, so Claude Code
saved it as the default for every session. Run `/model` and pick one of
Claude's own models; in llmenu sessions pick with `s` instead.

**No llmenu line in Claude Code's status line**: it is only added to sessions
started with `llmenu claude`, and not with `claude.statusline` off
(`llmenu config get claude.statusline`). It runs `llmenu statusline`, so
`llmenu` must be on the `PATH`.

**Copilot CLI answers with an error about tool calling or streaming**: Copilot
needs a model that supports both; pick another one.

## Updating and uninstalling

Updating keeps settings and keys; `llmenu --version` shows the installed
version.

```sh
brew update && brew upgrade --cask llmenu        # Homebrew
scoop update; scoop update llmenu                # Scoop
go install github.com/schowave/llmenu/cmd/llmenu@latest   # Go
```

A normal uninstall (`brew uninstall --cask llmenu`, `scoop uninstall llmenu`,
or deleting `$(go env GOPATH)/bin/llmenu`) keeps settings, cache and keys, so
a reinstall picks up where you left off.

To start over or remove everything, run `llmenu reset` first, while llmenu is
still installed. It lists what it removes and asks before doing so: settings,
all keys in the keyring (projects included), caches, and the provider entry
`llmenu sync` wrote into pi and opencode. `llmenu reset --dry-run` only shows
the list. Then `llmenu setup` sets it up anew, or uninstall as above.

## Contributing

Contributions are welcome! [CONTRIBUTING.md](CONTRIBUTING.md) covers the
development setup, the local sandbox, tests and releases. Please follow the
[Code of Conduct](CODE_OF_CONDUCT.md). To report a security issue, see
[SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE) © David Schowalter
