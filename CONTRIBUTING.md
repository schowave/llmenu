# Contributing to llmenu

Thanks for your interest! Bug reports, ideas and pull requests are welcome.
Please follow the [Code of Conduct](CODE_OF_CONDUCT.md).

- **Bugs and ideas:** open an [issue](https://github.com/schowave/llmenu/issues/new/choose).
- **Security problems:** do not open an issue; see [SECURITY.md](SECURITY.md).
- **Larger changes:** open an issue first so we can agree on the approach.
  Architectural decisions are recorded in [docs/ADR](docs/ADR); a change of
  direction gets a new ADR (template: [0000](docs/ADR/0000-adr-template.md)).

## Development setup

Tools and tasks are managed with [mise](https://mise.jdx.dev) (`mise.toml`):

```sh
mise install                   # Go, golangci-lint, goreleaser, actionlint, govulncheck, gremlins
mise run ci                    # everything CI checks: fmt, tidy, vet, tests (-race), coverage, lint, govulncheck, workflows
```

Single tasks (`mise tasks` lists all):

```sh
mise run test                  # unit and script tests
mise run cover                 # coverage; fails below the minimum in scripts/coverage.sh
mise run mutation:changed      # mutation testing of your changes only (seconds)
mise run mutation              # mutation testing of everything (a few minutes)
mise run compat                # the real pi, opencode and codex read what llmenu generates
mise run release-check         # validate .goreleaser.yaml and build a snapshot into dist/
mise run install               # install your build into $(go env GOPATH)/bin
mise run demo                  # re-record the README demo docs/demo.gif (needs `brew install vhs`)
```

## Trying changes by hand: the sandbox

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

> **macOS:** never redirect `HOME` while llmenu can reach the OS keyring —
> `security` then finds no keychain and opens a dialog. Choose "Cancel", never
> "Reset to defaults". The sandbox avoids this with `LLMENU_KEYRING_FILE`.

## Tests

The testing strategy is described in [ADR-0004](docs/ADR/0004-testing-strategy.md).

- **Unit tests** live next to the code.
- **Script tests** in `internal/cli/testdata/script/*.txtar`
  ([testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript))
  run the real commands against a fake gateway with stand-in tools. Setup
  flows are driven through `LLMENU_ACCESSIBLE=1`.
- **Fuzz tests** for JSON editing and name normalization, e.g.
  `go test -run=^$ -fuzz=FuzzEditJSON ./internal/targets`.
- **Compatibility tests** in `test/compat` (build tag `compat`) check the
  generated configs against the real tools; they run weekly on GitHub.
- **Mutation testing** with [Gremlins](https://github.com/go-gremlins/gremlins);
  known equivalent mutants are listed in [docs/mutation-testing.md](docs/mutation-testing.md).

Tests never touch the OS keyring, real tool configs or paid APIs. Two safety
nets enforce this even if a test (or a mutant) loses an override: under
`go test`, `internal/guard` refuses every path outside the OS temp directory,
and the packages that handle keys mock the keyring in `TestMain`. New code
that reads or writes files must go through `guard.Path`.

## Pull requests

- Keep changes focused; add or update tests for behavior changes.
- `mise run ci` must pass. CI runs it on Linux, macOS and Windows, plus
  CodeQL, dependency review and mutation testing of the changed lines.
- Write commit messages in the imperative mood ("Add …", "Fix …").

## Adding a tool

Create a file under `internal/targets/`, implement `Syncer` (write the model
list into the tool's config) and/or `Launcher` (start the tool with gateway
and model), register it in `init()` and add tests. Commands, help and
completion are generated from the registry. See
[ADR-0002](docs/ADR/0002-command-structure-and-targets.md).

## Project layout

| Path | Contents |
|---|---|
| `cmd/llmenu` | entry point |
| `internal/cli` | commands (cobra), setup forms (huh) |
| `internal/targets` | `Target`, `Syncer`, `Launcher` and one file per tool |
| `internal/gateway` | client for the LiteLLM gateway (`/model/info`) |
| `internal/catalog` | models: normalization, aliases, duplicates, slot suggestions |
| `internal/bench` | fetching and matching benchmarks, cache `bench.json` |
| `internal/menu`, `internal/table` | menu (bubbletea) and table format |
| `internal/config`, `internal/secrets` | `config.toml` and keys ([ADR-0003](docs/ADR/0003-configuration-and-setup.md)) |
| `internal/state`, `internal/fetch` | last selection per target; HTTP GET with readable errors |
| `internal/guard` | keeps tests inside the temp directory |
| `test/compat` | compatibility tests against the real tools |
| `tools/`, `scripts/` | fake gateway and stand-in tools for the sandbox; coverage and mutation scripts |

## Releasing (maintainers)

1. One-time setup: the public repos `schowave/homebrew-tap` and
   `schowave/scoop-bucket`, and a fine-grained personal access token with
   *Contents: read and write* on exactly these two repos, stored here as the
   Actions secret `TAP_GITHUB_TOKEN`.
2. GitHub → Releases → *Draft a new release*, create a new tag like `v0.2.0`
   on `main`, write the notes, *Publish release*.
3. `.github/workflows/release.yml` builds all platforms with GoReleaser,
   attaches the archives and checksums to the release and updates the cask in
   the tap and the manifest in the bucket.

Versions follow [Semantic Versioning](https://semver.org).
