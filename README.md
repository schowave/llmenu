<h1 align="center">llmenu</h1>

<p align="center">
  <strong>Pick an LLM from your LiteLLM gateway — by price, context and benchmarks — and use it in Claude Code, Codex CLI, pi and opencode.</strong>
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

Many teams reach their models through a [LiteLLM](https://github.com/BerriAI/litellm)
gateway: dozens of models from several providers, aliases like
`claude-opus-latest`, different prices and context sizes — and every coding
tool wants that configured differently. **llmenu** reads the model list from
the gateway and does the wiring for you:

```console
$ llmenu claude
MODEL                          CONTEXT      $ IN     $ OUT  INTEL    TB4   CODE
aws/claude-opus-5-5              1000k      4.40     22.00   57.6   59.6   80.0
azure/gpt-6-sol                   922k      2.00     10.00   47.6   43.9      -
gcp/claude-sonnet-4-5             200k      3.00     15.00   42.0      -      -
…
model> █
```

Pick a model, press Enter, and Claude Code starts against your gateway with
that model — no environment variables to export, no config files to edit.

## Features

- **Fuzzy menu** of all chat models on the gateway with context size, input
  and output prices and benchmark scores from
  [Artificial Analysis](https://artificialanalysis.ai) (Intelligence Index,
  Terminal-Bench, Coding Index); aliases and provider duplicates are folded
  together.
- **Launches** Claude Code and Codex CLI against the gateway with the chosen
  model. Your `~/.claude` and `~/.codex` configs stay untouched.
- **Syncs** the gateway's models, limits and prices into pi and opencode, so
  their own model pickers show everything. Comments and other providers in
  those files are preserved, and a backup is kept.
- **Works with any LiteLLM gateway.** Suggestions for Claude Code's
  opus/sonnet/haiku slots are derived from the gateway's model list; nothing
  is hard-coded.
- **Keys stay in the OS keyring** (macOS Keychain, Windows Credential Manager,
  Secret Service), never in config files or process arguments.
- **Scriptable:** `llmenu list --json`, `strix --model "$(llmenu)"`; without a
  terminal nothing ever waits for input.
- **Single binary** for macOS, Linux and Windows on amd64 and arm64.

## Contents

- [Installation](#installation)
- [Quick start](#quick-start)
- [Using llmenu with your tools](#using-llmenu-with-your-tools)
- [Models and benchmarks](#models-and-benchmarks)
- [Configuration](#configuration)
- [Command reference](#command-reference)
- [How it works](#how-it-works)

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

### Shell completion

Tab completes commands, settings, key names and model names. Homebrew sets it
up automatically; with Scoop or `go install`, set it up once for your shell:

zsh

```sh
llmenu completion zsh > "${fpath[1]}/_llmenu"
```

bash

```sh
llmenu completion bash > ~/.local/share/bash-completion/completions/llmenu
```

fish

```sh
llmenu completion fish > ~/.config/fish/completions/llmenu.fish
```

PowerShell

```powershell
Add-Content $PROFILE 'llmenu completion powershell | Out-String | Invoke-Expression'
```

### Updating

To the [latest release](https://github.com/schowave/llmenu/releases/latest);
`llmenu --version` shows the installed one. Settings and keys are kept.

Homebrew (`brew update` fetches the new cask from the tap first)

```sh
brew update && brew upgrade --cask llmenu
```

Scoop (`scoop update` refreshes the bucket first)

```powershell
scoop update; scoop update llmenu
```

Go

```sh
go install github.com/schowave/llmenu/cmd/llmenu@latest
```

### Uninstalling

A normal uninstall keeps your settings, cache and keys, so a reinstall picks
up where you left off:

```sh
brew uninstall --cask llmenu
```

```powershell
scoop uninstall llmenu
```

**To remove everything**, delete the keys first, while llmenu is still
installed, then uninstall with its data:

```sh
llmenu secrets delete gateway
llmenu secrets delete artificialanalysis
```

Homebrew (`--zap` also removes settings and caches)

```sh
brew uninstall --zap --cask llmenu
```

Scoop

```powershell
scoop uninstall llmenu
Remove-Item -Recurse -ErrorAction SilentlyContinue "$env:APPDATA\llmenu", "$env:LOCALAPPDATA\llmenu"
```

Go

```sh
rm -rf "$(go env GOPATH)/bin/llmenu" ~/.config/llmenu ~/.cache/llmenu ~/Library/"Application Support"/llmenu ~/Library/Caches/llmenu
```

llmenu also wrote a provider entry (named like your gateway, see
`llmenu config get gateway.name`) into the configs of pi and opencode if you
used `llmenu sync`; remove it there by hand. `llmenu targets` shows the files.

## Quick start

1. **Set up** — llmenu asks for your gateway URL and key (and checks them
   right away), which coding tools you use, and their default models:

   ```sh
   llmenu setup
   ```

   Run it again any time; Enter keeps the current value. If you skip this,
   the first command offers to do it.

2. **Use it** with your tool, e.g. pick a model and start Claude Code:

   ```sh
   llmenu claude
   ```

3. **Optional:** fetch benchmark scores so the menu can sort by quality
   (free key, the command explains how to get one):

   ```sh
   llmenu bench
   ```

## Using llmenu with your tools

llmenu integrates each tool the way that tool works best: some are
**started** by llmenu with the chosen model, others get the gateway's models
**written into their configuration** and then work on their own.

| Tool | What llmenu does | You run |
|---|---|---|
| [Claude Code](https://github.com/anthropics/claude-code) | starts it with the chosen model | `llmenu claude` |
| [Codex CLI](https://github.com/openai/codex) | starts it with the chosen model | `llmenu codex` |
| [opencode](https://opencode.ai) | writes the models into its config; starts it with the key | `llmenu sync`, then `llmenu opencode` |
| [pi](https://www.npmjs.com/package/@earendil-works/pi-coding-agent) | writes the models into its config | `llmenu sync`, then `pi` |
| anything else | prints the chosen model name | `tool --model "$(llmenu)"` |

### Claude Code

```sh
llmenu claude                      # menu with the cursor on your default, then Claude Code starts
llmenu claude --model sonnet -c    # no menu; every argument goes to claude unchanged
llmenu claude -p "explain this" > answer.md   # no terminal (scripts, redirects): default model, no menu
```

The gateway, key and the models for Claude Code's opus/sonnet/haiku slots are
passed in the environment; your `~/.claude` stays untouched. Defaults:
`llmenu setup claude`.

### Codex CLI

```sh
llmenu codex
llmenu codex exec "fix the failing test"
```

The gateway is passed as provider via `-c` overrides; `~/.codex/config.toml`
stays untouched. Default model: `llmenu setup codex`.

### opencode and pi

```sh
llmenu sync --dry-run    # show what would change
llmenu sync              # write the gateway's models, limits and prices
llmenu opencode          # start opencode with the gateway key
pi                       # pi gets the key itself: !llmenu secrets get gateway
```

Only llmenu's own provider entry is changed; comments and your other
providers stay, and the previous file is kept as `*.bak`. Then pick models in
the tool as usual. Run `llmenu sync` again when the gateway's models change.

## Models and benchmarks

```sh
llmenu list                        # table, best benchmark first
llmenu list --sort price --all     # by price, including aliases and duplicates
llmenu list --json                 # for scripts
llmenu bench                       # refresh benchmark scores
```

In the menu, type to search, **Tab** switches the sort column (INTEL, TB4,
CODE, price) and Enter picks. The best model is on top, models that do not
suit the tool (e.g. non-Claude models in `llmenu claude`) are dimmed and
marked `!`, and the preview on the right explains the columns.

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
| `LLMENU_BASE_URL`, `LLMENU_API_KEY` | gateway URL and key |
| `LLMENU_AA_API_KEY` | Artificial Analysis key |
| `LLMENU_CONFIG`, `LLMENU_CACHE_DIR` | alternative config file and cache directory |
| `LLMENU_ACCESSIBLE=1` | line-based setup prompts, e.g. for screen readers |

## Command reference

| Command | Purpose |
|---|---|
| `llmenu setup [section]` | set up gateway, tools and defaults (`gateway`, `tools`, `claude`, `codex`, `benchmarks`) |
| `llmenu claude`, `llmenu codex`, `llmenu opencode` | pick a model and start the tool |
| `llmenu` | pick a model and print its name |
| `llmenu sync [tool…]` | write the gateway's models into opencode and pi |
| `llmenu list`, `llmenu bench` | show models; fetch benchmark scores |
| `llmenu config`, `llmenu secrets` | change settings; store keys |
| `llmenu targets` | supported tools and their status |
| `llmenu help [command]` | help; `llmenu help environment` for environment variables |

## How it works

llmenu calls the gateway's `/model/info` endpoint, normalizes model names
across providers (`bedrock/eu.anthropic.claude-sonnet-4-5-20250929-v1:0` and
`vertex_ai/claude-sonnet-4-5@20250929` are the same model), folds aliases and
duplicates, and matches benchmark results to the models. The design decisions
are recorded as Architecture Decision Records in [docs/ADR](docs/ADR).

## Contributing

Contributions are welcome! [CONTRIBUTING.md](CONTRIBUTING.md) covers the
development setup, the local sandbox, tests and releases. Please follow the
[Code of Conduct](CODE_OF_CONDUCT.md). To report a security issue, see
[SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE) © David Schowalter
