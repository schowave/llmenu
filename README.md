<h1 align="center">llmenu</h1>

<p align="center">
  <strong>Pick an LLM from your LiteLLM gateway — by price, context and benchmarks — and use it in Claude Code, Codex CLI, GitHub Copilot CLI, pi and opencode.</strong>
</p>

<p align="center">
  <a href="https://github.com/schowave/llmenu/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/schowave/llmenu/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/schowave/llmenu/actions/workflows/codeql.yml"><img alt="CodeQL" src="https://github.com/schowave/llmenu/actions/workflows/codeql.yml/badge.svg"></a>
  <a href="https://scorecard.dev/viewer/?uri=github.com/schowave/llmenu"><img alt="OpenSSF Scorecard" src="https://api.scorecard.dev/projects/github.com/schowave/llmenu/badge"></a>
  <a href="https://github.com/schowave/llmenu/releases/latest"><img alt="Latest release" src="https://img.shields.io/github/v/release/schowave/llmenu"></a>
  <a href="go.mod"><img alt="Go version" src="https://img.shields.io/github/go-mod/go-version/schowave/llmenu"></a>
  <a href="https://goreportcard.com/report/github.com/schowave/llmenu"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/schowave/llmenu"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/github/license/schowave/llmenu"></a>
</p>

<p align="center"><a href="https://schowave.github.io/llmenu/">Website</a> · <a href="#installation">Install</a> · <a href="#documentation">Docs</a> · <a href="https://github.com/schowave/llmenu/discussions">Discussions</a></p>

---

**llmenu** (pronounced *L-L-menu*) is for teams that reach their models through a
[LiteLLM](https://github.com/BerriAI/litellm) gateway: dozens of models from
several providers, aliases like `claude-opus-latest`, different prices and
context sizes — and a coding tool that wants all of that configured in its own
way. llmenu reads the model list from the gateway, lets you compare the
models, and does the wiring for you.

<p align="center">
  <img alt="llmenu opens a menu of the gateway's models with context size, prices, Intelligence Index and Terminal-Bench scores, the project and its budget; Tab sorts by Terminal-Bench, by price and back, typing sonnet searches, Enter starts Claude Code with the chosen model" src="docs/demo.gif" width="900">
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
  (Intelligence Index and output speed with a free key, plus Terminal-Bench
  and Long Context Reasoning with a Pro key); aliases are folded
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

Details per tool, `/model` in Claude Code and `llmenu sync`: [docs/tools.md](docs/tools.md).

## Documentation

- [Using llmenu with your tools](docs/tools.md): Claude Code, Codex CLI, Copilot CLI, opencode, pi
- [Projects](docs/projects.md): one gateway key per project or cost center
- [Budget and spend](docs/budget.md): in the menu and in Claude Code's status line
- [Models and benchmarks](docs/models.md): `llmenu list`, sorting, how models are matched
- [Configuration](docs/configuration.md): settings, keys, environment variables
- [Troubleshooting](docs/troubleshooting.md)
- [Updating and uninstalling](docs/updating.md)
- [Design decisions](docs/ADR)

## Contributing

Contributions are welcome! [CONTRIBUTING.md](CONTRIBUTING.md) covers the
development setup, the local sandbox, tests and releases. Please follow the
[Code of Conduct](CODE_OF_CONDUCT.md). To report a security issue, see
[SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE) © David Schowalter
