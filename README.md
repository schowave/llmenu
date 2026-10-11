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

## Quick start

```sh
llmenu setup        # gateway URL and key, Claude Code slots, benchmarks, sync targets
llmenu claude       # pick a model, start Claude Code
```

`llmenu setup` can be re-run at any time; Enter keeps the current value. If
you run a command before setting up, llmenu offers to do it right there.

## Usage

### Pick and launch

```sh
llmenu claude                      # menu, cursor on your default model
llmenu claude --model sonnet -c    # no menu; all arguments go to claude unchanged
llmenu codex                       # the same for Codex CLI
llmenu codex exec "fix the failing test"
llmenu opencode                    # start opencode with the gateway key
llmenu                             # just pick: prints the model name, e.g. strix --model "$(llmenu)"
```

### Models and benchmarks

```sh
llmenu list                        # table, best benchmark first
llmenu list --sort price --all     # by price, including aliases and duplicates
llmenu list --json                 # for scripts
llmenu bench                       # refresh benchmarks (free Artificial Analysis key)
```

### Sync into pi and opencode

```sh
llmenu sync --dry-run              # show the diff
llmenu sync                        # write the models into the enabled tools
```

pi fetches the key itself (`!llmenu secrets get gateway`); opencode reads it
from the environment that `llmenu opencode` sets.

### Configuration

```sh
llmenu config list                         # all settings and where they come from
llmenu config set claude.default sonnet    # default model for Claude Code
llmenu config set bench.metric tb4         # sort by Terminal-Bench
llmenu secrets set gateway                 # store a key (read hidden, never as an argument)
llmenu targets                             # supported tools, install status, config paths
```

| Environment variable | Purpose |
|---|---|
| `LLMENU_BASE_URL`, `LLMENU_API_KEY` | gateway URL and key; override config and keyring, e.g. in CI |
| `LLMENU_AA_API_KEY` | Artificial Analysis key |
| `LLMENU_CONFIG`, `LLMENU_CACHE_DIR` | alternative config file and cache directory |
| `LLMENU_ACCESSIBLE=1` | line-based setup prompts with numbered options, e.g. for screen readers |

## Supported tools

| Tool | How llmenu integrates it |
|---|---|
| [Claude Code](https://github.com/anthropics/claude-code) | launch with gateway, key and model slots in the environment |
| [Codex CLI](https://github.com/openai/codex) | launch with the gateway as provider via `-c` overrides |
| [pi](https://www.npmjs.com/package/@earendil-works/pi-coding-agent) | sync models into `~/.pi/agent/models.json` |
| [opencode](https://opencode.ai) | sync models into the opencode config, launch with the key |

Any other tool can use the picker directly: `tool --model "$(llmenu)"`. A new
integration is a single file — see [CONTRIBUTING.md](CONTRIBUTING.md).

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
