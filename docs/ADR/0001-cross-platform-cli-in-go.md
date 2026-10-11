# ADR-0001: Model selection as a cross-platform Go CLI, distributed via Homebrew and Scoop

- **Status:** Proposed
- **Date:** 2026-10-11
- **Deciders:** David Schowalter

## Context and Problem Statement

Selecting an LLM from the company LiteLLM gateway currently lives in zsh functions
in `~/.zshrc`:

| Function          | Purpose                                                                                     |
|-------------------|---------------------------------------------------------------------------------------------|
| `claude-api`      | Starts Claude Code against the company LiteLLM gateway (base URL, token, model mapping via environment variables) |
| `claude-benchmark`| Fetches benchmarks from Artificial Analysis, matches them to the gateway models and writes `~/.cache/claude-pick/bench.tsv` |
| `_gateway-pick`  | fzf selection over all chat models with context, prices and benchmarks                      |
| `claude-pick` / `pi-pick` | Starts Claude Code or pi with the selected model; `pi-pick` also maintains `~/.pi/agent/models.json` |

This works, but it has structural limits:

- **Not portable.** The functions need zsh, curl, jq, awk, fzf and the
  macOS keychain helper `creds`. None of it runs on Windows without WSL or
  Git Bash.
- **Not distributable.** Colleagues would have to copy code into their own
  `.zshrc`. There would be no updates.
- **Not testable.** The core is the matching between gateway and benchmark
  models, a heuristic based on token sets and stop words. It is buried in
  jq one-liners and cannot be checked automatically against sample data.
- **Duplicated logic.** `key()` exists twice, with different stop-word
  lists, and this is documented nowhere.
- **Wrong place.** Around 200 lines of program logic in `.zshrc` are not
  versioned, and a bug in them breaks the shell configuration.
- **Secrets.** API keys are read from the keychain on every shell start,
  exported globally and visible in the process list as `curl -H`.

Question: **In what form do we build the model selection in this repo so
that it stays as convenient in daily use as it is today, also runs on
Windows, and can be distributed to others via the usual package managers?**

## Decision Drivers

1. **Runs on macOS, Linux and Windows** (natively in PowerShell, without WSL).
2. **Distribution via package managers:** `brew install` on macOS and Linux,
   `scoop install` on Windows, updates via `brew upgrade` or
   `scoop update`. Releases should require as little maintenance as
   possible.
3. **Fit for daily use:** one command on the `PATH`, no noticeable startup
   delay, interactive fuzzy selection as today.
4. **Testable matching logic** with unit tests on recorded API responses.
5. **Safe handling of secrets:** stored in the OS keychain, no keys in argv
   and no global export.
6. **Few prerequisites for users:** no runtime such as Python or Node.js and
   ideally no other tools.

## Considered Options

1. **Keep the shell:** zsh scripts in the repo, plus a parallel PowerShell
   port for Windows.
2. **Python CLI**, installed with uv (`uv tool install`).
3. **Go CLI** as a single binary per platform, built and published with
   goreleaser.
4. **TypeScript CLI** on Node.js or Bun, installed via npm.

## Decision

We chose **Option 3: a Go CLI that goreleaser builds into one binary per
platform and distributes via a dedicated Homebrew tap and a dedicated Scoop
bucket.**

Rationale along the drivers:

- **Distribution (driver 2) is decisive.** From a Git tag, goreleaser
  produces the binaries for all platforms, the release, and at the same time
  the entries for the Homebrew tap and the Scoop bucket. With Python, the
  Brew formula would have to list every transitive dependency as a
  `resource` with a checksum and update them on every release. For Scoop
  there would be no ready-made path at all.
- **No runtime (driver 6):** The binary is statically linked. Users need
  neither Python nor uv, Node.js, jq or curl.
- **Platforms (driver 1):** Go has cross-compilation built in. The standard
  library knows the platform-specific directories (`os.UserCacheDir`,
  `os.UserConfigDir`) and finds programs on Windows via `PATHEXT`
  (`exec.LookPath`).
- **Startup (driver 3):** Startup time is in the millisecond range, just like
  a shell function.
- **Tests (driver 4):** table-driven tests with `go test` on fixture files in
  `testdata/`.

The price is more code for the data processing that lives in jq today. In
Python it would be more concise. We accept that because distribution and
the lack of prerequisites for users weigh more.

### Scope

One command `llmenu` with subcommands that replace today's functions. The
name combines "LLM" with "menu": a menu with prices and ratings from which
you pick a model. It deliberately contains neither the name of a particular gateway nor "Claude".
The tool only talks to the endpoints of a LiteLLM proxy (`/model/info`) and
can therefore also be used with other LiteLLM gateways. Besides, "Claude" is
a trademark of Anthropic.

How the commands are structured and how tools such as Claude Code, pi,
opencode and Codex CLI are integrated is covered by
[ADR-0002](0002-command-structure-and-targets.md).

Guardrails for the implementation:

- **Project layout:** `cmd/llmenu` as the entry point. The domain logic
  lives in `internal/` (gateway client, benchmark client, matching,
  selection, process launch).
- **CLI:** `spf13/cobra` for subcommands, help and shell completion.
  Completion for zsh and PowerShell comes along for free.
- **HTTP:** `net/http` from the standard library with timeouts. On errors,
  the HTTP status and the beginning of the response are printed, as
  `_gateway-get` does today.
- **Secrets:** `zalando/go-keyring` uses the macOS Keychain, the Windows
  Credential Manager and the Secret Service on Linux. The service name is
  `llmenu`, the entries are named `mygateway` and `artificialanalysis`.
  Environment variables may override them, e.g. for CI. Keys are only passed
  to the launched child process, never to the global environment.
- **Configuration:** The gateway URL, the mapping for Opus/Sonnet/Haiku and
  the flags for Claude Code (`DISABLE_AUTO_COMPACTION` etc.) are stored in a
  TOML file under `os.UserConfigDir()`. The defaults are embedded in the
  binary. Today they are hard-coded.
- **Selection:** `ktr0731/go-fuzzyfinder` is built in and replaces fzf, so
  the tool has no external dependency. The header line with the benchmark
  source and a preview pane with details (including the matched benchmark
  slug) are kept. Data preparation is separated from presentation so that
  `list --json` delivers the same data.
- **Matching logic:** There is a single normalization function with the
  stop-word list as a parameter. The fact that the picker and the benchmark
  use different lists is then stated explicitly in the code instead of
  being accidental.
- **Process launch:** The tool (e.g. `claude`) is located via
  `exec.LookPath` and started with `exec.Command`. The terminal
  (stdin/stdout/stderr) is inherited and the exit code is passed through.
  While the child process runs, `llmenu` ignores Ctrl-C so that only the
  child reacts to it. We do without `syscall.Exec` because it is missing on
  Windows. This keeps the behavior the same on all platforms.

### Release and distribution

- **goreleaser** builds the binaries for darwin/linux/windows × amd64/arm64
  on every Git tag `vX.Y.Z` and publishes them as a release with checksums.
  This is triggered in CI (GitHub Actions or GitLab CI).
- **Homebrew:** goreleaser updates a tap repo `homebrew-tap`.
  Installation: `brew install <owner>/tap/llmenu`. Whether as a formula or a
  cask depends on the goreleaser version; newer versions prefer casks.
- **Scoop:** goreleaser updates a bucket repo `scoop-bucket`.
  Installation: `scoop bucket add <owner> <url>` followed by
  `scoop install llmenu`.
- Where the repos live (private GitHub account or a company GitLab) is still
  open. With private repos, users need read access, and the tap has to be
  added with an explicit URL.

### Daily use

```sh
# Development: use changes in the repo directly
go install ./cmd/llmenu        # ends up in $(go env GOPATH)/bin

# Everyone else, or a stable version
brew install <owner>/tap/llmenu        # macOS, Linux
scoop install llmenu                   # Windows

llmenu claude          # pick a model, start Claude Code
llmenu bench           # refresh benchmarks
```

Anyone who wants to keep the old names can create thin aliases:
`alias claude-pick='llmenu claude'` in zsh or
`function claude-pick { llmenu claude @args }` in the PowerShell profile.

## Consequences

### Positive

- One binary without runtime dependencies, installable with each platform's
  package manager, including updates.
- A release is a Git tag. The entries in tap and bucket are created
  automatically.
- The matching logic is covered by tests. New gateway or benchmark models
  can be added as fixtures and checked.
- `.zshrc` shrinks to a few aliases. Keys are no longer read on every shell
  start and no longer exported globally.
- fzf, jq and curl are no longer prerequisites.

### Negative

- The data processing, concise jq expressions today, becomes considerably
  longer in Go: structs for the API responses, explicit handling of missing
  fields. Adjusting the heuristic therefore takes a bit longer.
- Three repos instead of one (tool, tap, bucket), plus a token in CI that may
  write to tap and bucket.
- The unsigned binaries may trigger warnings: Gatekeeper on macOS
  (especially for casks because of the quarantine attribute) and SmartScreen
  or virus scanners on Windows. Signing and notarization cost effort and
  certificates.
- go-fuzzyfinder is less mature than fzf. Anyone used to fzf key bindings
  will notice differences.
- The rewrite costs a one-time effort. Until everything works equally well,
  both variants exist side by side.

### Neutral / Follow-ups

- The secrets move from `creds` to go-keyring. On macOS both live in the
  Keychain, just under a different service name.
- While porting, fix the findings from the review of the zsh functions right
  away: when removing duplicates, the cheapest model wins even if it has
  less context; the benchmark match is not shown in the picker; plus the
  cost question with compaction disabled and 1M context.
- Decide where the tool repo, tap and bucket live and how CI gets the
  permissions to write to tap and bucket.
- The zsh functions are only removed once `llmenu` works in daily use on
  macOS and Windows.

## Confirmation

- `brew install` on macOS and `scoop install` on a Windows machine install
  the tool. `llmenu claude` starts Claude Code there with the selected
  model, on Windows in PowerShell without WSL.
- `go test ./...` checks the matching against recorded responses from
  `/model/info` and Artificial Analysis. The results match today's
  `bench.tsv`, or every deviation is a deliberate decision.
- CI runs the tests on macOS, Linux and Windows.
  `goreleaser release --snapshot` succeeds on every merge.

## Pros and Cons of the Options

### Option 1: Keep the shell, plus a PowerShell port

- Good: no rewrite for macOS, works immediately, familiar tools.
- Bad: two implementations of the same heuristic that will drift apart. The
  problem already exists today within *one* file, see `key()`.
- Bad: hardly testable and hardly distributable. jq, awk and fzf are
  additional installs on Windows.

### Option 2: Python CLI with uv

- Good: concise, readable data logic and fast iteration on the heuristic;
  mature libraries (keyring, platformdirs, httpx);
  `uv tool install --editable .` is very convenient for one's own
  development.
- Bad: the Brew formula has to list every transitive dependency as a
  `resource` with a checksum and be updated on every release. There is no
  ready-made path for Scoop, and users need uv or Python.
- Bad: startup of roughly 100 to 300 ms is noticeably slower.

### Option 3: Go CLI with goreleaser (chosen)

- Good: one static binary, effortless cross-compilation; Brew and Scoop are
  maintained by the release tool; very fast startup; no external
  dependencies when the picker is built in.
- Bad: more code for JSON processing and heuristics; tap and bucket as
  additional repos; signing issues on macOS and Windows.

### Option 4: TypeScript on Node.js or Bun

- Good: cross-platform; Node.js is often already installed.
  `bun build --compile` can even produce single binaries.
- Bad: these binaries are large (tens of MB), and there is no established
  tool for releases with tap and bucket. For the keychain there are only
  native modules (`keytar` is archived).

## More Information

- Starting point: functions `claude-api`, `claude-benchmark`,
  `_gateway-get`, `_gateway-pick`, `claude-pick` and `pi-pick` in
  `~/.zshrc`
- goreleaser: <https://goreleaser.com/> (Homebrew: <https://goreleaser.com/customization/homebrew_casks/>, Scoop: <https://goreleaser.com/customization/scoop/>)
- go-keyring: <https://github.com/zalando/go-keyring>
- go-fuzzyfinder: <https://github.com/ktr0731/go-fuzzyfinder>
- cobra: <https://github.com/spf13/cobra>
- Homebrew taps: <https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap>
- Scoop buckets: <https://github.com/ScoopInstaller/Scoop/wiki/Buckets>
- Revisit: if go-fuzzyfinder does not match fzf in daily use (alternative:
  declare fzf as a dependency in tap and bucket and call it externally) or
  if the signing warnings hinder distribution.
