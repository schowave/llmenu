# llmenu

Picks models from a LiteLLM gateway, with context, prices
and benchmarks, and wires them into coding tools: Claude Code and Codex CLI
are launched with the selected model, and for pi and opencode the model
list is written into their configuration.

Decisions and rationale: [docs/ADR](docs/ADR).

> **Status:** Everything described here is implemented: setup and
> configuration (`setup`, `config`, `secrets`), the menu (`llmenu`),
> the launchers `llmenu claude`, `llmenu codex` and `llmenu opencode`,
> `sync` for pi and opencode, `bench`, `list` and `targets`.

## Installation

```sh
go install github.com/schowave/llmenu/cmd/llmenu@latest
```

Requires Go 1.27.2 or later. Releases for Homebrew and Scoop are planned
([ADR-0001](docs/ADR/0001-cross-platform-cli-in-go.md)).

## Usage

```sh
llmenu setup                   # gateway, Claude slots, benchmarks, sync targets; repeatable
llmenu setup claude            # just one section
llmenu config list             # all values with their source
llmenu config set claude.default sonnet

llmenu claude                  # menu, cursor on the default
llmenu claude --model sonnet   # without menu
strix --model "$(llmenu)"      # menu, name on stdout
llmenu list                    # table; --all with aliases, --json, --sort price
llmenu bench                   # fetch benchmarks from Artificial Analysis (key: setup benchmarks)
llmenu config set bench.metric tb4                                  # sort by Terminal-Bench
llmenu config set bench.map.mistral/mistral-large-2512 mistral-large-3  # fixed mapping

llmenu codex                   # menu, then Codex CLI via the gateway
llmenu codex exec "fix the failing test"   # arguments go to codex unchanged
llmenu config set codex.default <model>    # default model (needed without a terminal unless -m is given)

llmenu sync --dry-run          # diff for the enabled sync targets (sync.targets)
llmenu sync pi opencode        # write the models; the old file is kept as *.bak
pi                             # fetches the key itself: !llmenu secrets get gateway
llmenu opencode                # start opencode with the key (opencode reads it from LLMENU_API_KEY)
```

On the first run without setup, `llmenu` offers to run the setup. The
configuration lives at `llmenu config path`, keys in the operating system's
keychain. `LLMENU_BASE_URL`, `LLMENU_API_KEY`, `LLMENU_AA_API_KEY`,
`LLMENU_CONFIG` and `LLMENU_CACHE_DIR` override these, e.g. for CI.

`llmenu codex` leaves `~/.codex/config.toml` untouched: provider and model
are passed via `-c` overrides, and the key only goes into Codex's
environment ([ADR-0002](docs/ADR/0002-command-structure-and-targets.md)).

## Development

Tools and tasks are managed with [mise](https://mise.jdx.dev) (`mise.toml`):

```sh
mise install                   # Go, golangci-lint, goreleaser, actionlint, govulncheck
mise run ci                    # everything CI checks: fmt, tidy, vet, tests (-race), lint, govulncheck, workflows
mise run test                  # single tasks: fmt, vet, test, lint, vuln, workflows, build, install
mise run release-check         # validate .goreleaser.yaml and build a snapshot into dist/
mise tasks                     # list all tasks
```

On GitHub, CI runs the same checks on Linux, macOS and Windows for every push
and pull request, plus CodeQL, dependency review and Dependabot updates.

For tests with a separate keychain entry, set
`LLMENU_KEYRING_SERVICE=llmenu-test`; for a separate configuration,
`LLMENU_CONFIG=<file>`. Do not redirect `HOME` on macOS: `security` then
finds no keychain and opens a dialog. Always choose "Cancel" there.

## Layout

| Path | Contents |
|---|---|
| `cmd/llmenu` | Entry point |
| `internal/cli` | Commands (cobra) |
| `internal/targets` | Targets: `Target`, `Syncer`, `Launcher` and one file per tool |
| `internal/gateway` | Client for the LiteLLM proxy (`/model/info`) |
| `internal/menu`, `internal/table` | Menu (go-fuzzyfinder) and table format for the menu and `list` |
| `internal/config`, `internal/secrets` | `config.toml` and keys in the keychain ([ADR-0003](docs/ADR/0003-configuration-and-setup.md)) |
| `internal/state` | Most recently selected model per target |
| `internal/bench` | Fetching and matching benchmarks, cache `bench.json` |
| `internal/fetch` | HTTP GET with readable errors |
| `internal/catalog` | Models with benchmarks; normalization, aliases, duplicates |
| `.goreleaser.yaml` | Release for macOS, Linux, Windows; tap and bucket still commented out |

Adding a new tool: create a file under `internal/targets/`, implement
`Syncer` and/or `Launcher`, register it in `init()`, add a test. Commands
and help are generated from that.
