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
brew install --cask schowave/tap/llmenu        # macOS, Linux

scoop bucket add schowave https://github.com/schowave/scoop-bucket
scoop install llmenu                           # Windows

go install github.com/schowave/llmenu/cmd/llmenu@latest   # from source, Go 1.27.2+
```

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

`LLMENU_ACCESSIBLE=1` turns all setup forms into plain line-based questions
with numbered options (huh's accessible mode), e.g. for screen readers.

`llmenu codex` leaves `~/.codex/config.toml` untouched: provider and model
are passed via `-c` overrides, and the key only goes into Codex's
environment ([ADR-0002](docs/ADR/0002-command-structure-and-targets.md)).

## Development

Tools and tasks are managed with [mise](https://mise.jdx.dev) (`mise.toml`).
`mise install` once, then:

```sh
mise run ci                    # everything CI checks: fmt, tidy, vet, tests (-race), coverage, lint, govulncheck, workflows
mise run test                  # unit and script tests only
mise run cover                 # coverage report, fails below the threshold in mise.toml
mise run mutation:changed      # mutation testing of your changes only (seconds); see docs/mutation-testing.md
mise run mutation              # mutation testing of everything (a few minutes; also weekly on GitHub)
mise run compat                # pi, opencode and codex read what llmenu generates (needs the tools installed)
mise run release-check         # validate .goreleaser.yaml and build a snapshot into dist/
mise tasks                     # all tasks
```

### Sandbox

To try llmenu by hand after a change, without touching your real setup:

```sh
mise run sandbox               # fresh sandbox, starts with the first-run experience
mise run sandbox:resume        # continue with the state of the last session
mise run sandbox:live          # same isolation, but against your real gateway
mise run sandbox:clean         # delete .sandbox/
```

The sandbox opens a shell in which llmenu, pi and opencode only see
`.sandbox/`: its own HOME, config, cache and a plain-text key file instead of
the OS keyring. A fake gateway answers on `http://127.0.0.1:4111/` (key
`sk-dev`, benchmark key `aa-dev`), and `claude`/`codex` are stand-ins that
print how llmenu would have started them. Exit the shell to leave.

### Tests

- Unit tests next to the code; `internal/cli/testdata/script/*.txtar` are
  end-to-end scripts ([testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript))
  that run the real commands against a fake gateway with stand-in tools.
- Fuzz tests for JSON editing and name normalization:
  `go test -run=^$ -fuzz=FuzzEditJSON ./internal/targets`.
- `test/compat` (build tag `compat`) checks the generated configs against the
  real pi, opencode and Codex CLI; it runs weekly on GitHub.

Never redirect `HOME` on macOS while llmenu can reach the OS keyring:
`security` then finds no keychain and opens a dialog — choose "Cancel",
never "Reset to defaults". The sandbox avoids this with `LLMENU_KEYRING_FILE`.

### Releasing

1. One-time setup on GitHub:
   - create the empty public repos `schowave/homebrew-tap` and
     `schowave/scoop-bucket`;
   - create a fine-grained personal access token with *Contents: read and
     write* on exactly these two repos and store it in this repo as the
     Actions secret `TAP_GITHUB_TOKEN`.
2. For each release: GitHub → Releases → *Draft a new release*, create a new
   tag like `v0.1.0` on `main`, write the notes, *Publish release*.
3. `.github/workflows/release.yml` builds all platforms, attaches the
   archives and checksums to that release and updates the cask in the tap and
   the manifest in the bucket.

On GitHub, CI runs on Linux, macOS and Windows for every push and pull
request, plus CodeQL, dependency review and Dependabot updates.

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
| `test/compat` | Compatibility tests against the real tools (build tag `compat`) |
| `tools/`, `scripts/sandbox.sh` | Fake gateway and stand-in tools for the sandbox |
| `.goreleaser.yaml` | Release for macOS, Linux, Windows, Homebrew cask and Scoop manifest |

Adding a new tool: create a file under `internal/targets/`, implement
`Syncer` and/or `Launcher`, register it in `init()`, add a test. Commands
and help are generated from that.

## License

[MIT](LICENSE)
