# ADR-0004: Layered tests with a coverage gate, mutation testing and a local sandbox

- **Status:** Proposed
- **Date:** 2026-10-11
- **Deciders:** David Schowalter
- **Builds on:** [ADR-0002](0002-command-structure-and-targets.md), [ADR-0003](0003-configuration-and-setup.md)

## Context and Problem Statement

llmenu edits other tools' configuration files, starts tools with secrets in
their environment and talks to two external APIs. A regression can break
someone's pi or opencode setup, leak a key or start Claude Code with the
wrong model. At the same time most behavior sits in command flows (menu,
setup, launch, sync) that plain unit tests do not reach, and the tools it
integrates change their formats without notice.

After the first implementation, coverage was 41 % and the command package
had no tests at all; correctness of the flows had only been checked by hand.

Question: **How do we make sure that after a change everything still works —
in CI and while developing locally — without real gateways, real keys or
real tool configs?**

## Decision Drivers

1. Command flows are tested end to end, not just the helpers.
2. Tests never touch the OS keyring, real tool configs or paid APIs.
3. Format changes of pi, opencode and Codex CLI are noticed.
4. The tests check behavior, not just execute lines.
5. The same checks run locally with one command and on GitHub.
6. Manual exploration after a refactoring is safe and quick.

## Considered Options

1. Unit tests only, plus manual testing.
2. Unit tests plus end-to-end tests against real tools and a real gateway.
3. **Layered tests: unit, script-based end-to-end against fakes, fuzzing,
   weekly compatibility and mutation runs, a coverage gate, and a local
   sandbox for manual testing.**

## Decision

Option 3. The layers:

| Layer | What | When |
|---|---|---|
| Unit tests | Logic per package, table-driven; huh forms' logic extracted into pure functions | every push (CI, `mise run test`) |
| Script tests | `internal/cli/testdata/script/*.txtar` with [testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript): real commands against a fake gateway and fake Artificial Analysis server, with stand-in `claude`/`codex`/`opencode` that print their arguments and environment; keyring mocked | every push |
| Terminal flows | Menu as a bubbletea model: keys are sent to it and the rendered view is checked; setup through huh's accessible mode (`LLMENU_ACCESSIBLE=1`, line-based stdin) | every push |
| Fuzzing | `editJSON` (other tools' configs must stay valid and untouched outside llmenu's entry) and name normalization; seeds run with every `go test` | seeds every push, longer runs locally |
| Coverage gate | `scripts/coverage.sh`, minimum 85 % | every push |
| Compatibility | `test/compat` (build tag `compat`): the real pi, opencode and Codex CLI read what llmenu generates | weekly and on changes to `internal/targets` |
| Mutation testing | Gremlins via `scripts/mutation.sh` with thresholds for efficacy and mutant coverage; equivalent mutants documented in [docs/mutation-testing.md](../mutation-testing.md) | changed lines on every pull request and push to `main` (warnings only), whole module weekly with thresholds; locally `mise run mutation:changed` |
| Sandbox | `mise run sandbox`: shell with its own HOME, config, cache and a plain-text key file (`LLMENU_KEYRING_FILE`), a fake gateway on localhost and stand-in `claude`/`codex` | manually during development |

Supporting rules:

- Every test sets `LLMENU_API_KEY` or mocks the keyring, so the OS keyring is
  never used. `HOME` is only redirected where the keyring cannot be reached
  (macOS opens a dialog otherwise). Packages that handle keys mock the keyring
  in `TestMain`, not per test.
- **Path guard (`internal/guard`):** under `go test`, every file llmenu reads
  or writes must be inside the OS temp directory, otherwise the call fails.
  Overrides like `LLMENU_CONFIG` alone are not enough: during mutation testing
  a mutant flipped such a check, and the test then wrote into the developer's
  real llmenu config, pi config and opencode config. A mutant changes one spot
  at a time, so the guard and the override cannot both be disabled at once.
- Live checks against a real gateway are manual (`mise run sandbox:live`),
  never in CI, because they need secrets and cost money.
- `mise run ci` runs the same checks as the CI workflow.

## Consequences

### Positive

- Command flows, including setup and the menu, are covered; the fuzzer has
  already found two real bugs in `editJSON` (an endless loop on invalid UTF-8
  keys, and edits landing in a duplicate key that readers ignore).
- Format changes of the integrated tools surface within a week.
- Refactorings can be tried by hand in the sandbox without risk.

### Negative

- More test infrastructure to maintain: fake servers, stand-in tools,
  compat workflow.
- A full mutation run takes a few minutes and therefore only runs weekly;
  per change only the changed lines are mutated. The list of equivalent
  mutants needs care when code changes.
- The compat suite depends on npm packages of third-party tools.

### Neutral / Follow-ups

- Raise the coverage minimum and the mutation thresholds when the numbers
  have been stable for a while.

## Confirmation

- `mise run ci` passes locally and the CI workflow passes on Linux, macOS
  and Windows.
- The weekly compatibility and mutation workflows pass.

## More Information

- testscript: <https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript>
- Go fuzzing: <https://go.dev/doc/security/fuzz/>
- Gremlins: <https://github.com/go-gremlins/gremlins>
