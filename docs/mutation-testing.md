# Mutation testing

[Gremlins](https://github.com/go-gremlins/gremlins) changes the code in small
ways (flips `==` to `!=`, `<` to `<=`, `+` to `-`, …) and runs the tests
against every such mutant. A mutant the tests notice is *killed*; one they do
not notice *lived*; one in code no test runs is *not covered*.

```sh
mise run mutation:changed            # only lines changed against origin/main, usually seconds
mise run mutation ./internal/targets # one package, 10–30 s
mise run mutation                    # whole module, a few minutes
```

When it runs:

| Where | What | Fails? |
| --- | --- | --- |
| Locally, on demand | `mutation:changed` while working, `mutation` before bigger merges | `mutation:changed` only reports |
| GitHub, pull requests and pushes to `main` | changed lines only | no; surviving and uncovered mutants become warnings on the lines |
| GitHub, weekly and on demand | whole module | yes, below the thresholds in `scripts/mutation.sh` (efficacy 97 %, mutator coverage 93 %) |

`scripts/mutation.sh` always runs the tests uncached (`-count=1`): Gremlins
derives each mutant's timeout from the coverage run, and a cached, instant run
makes mutants time out en masse.

## Known equivalent mutants

The mutants below survive (or are reported as not covered) without a gap in
the tests: either the mutation cannot change observable behavior, or Gremlins
cannot attribute the code to a test (marked *blind spot*; these were applied
by hand and the named test fails). Line numbers refer to the code at the time
of writing; update them when the code moves.

### internal/bench

| Mutant | Type | Reason |
| --- | --- | --- |
| bench.go:27:18 | ARITHMETIC_BASE | Blind spot: `StaleAfter` is a constant and Go's coverage has no blocks for constant declarations; `TestNote` fails when applied. |
| bench.go:27:23 | ARITHMETIC_BASE | Same as above. |
| bench.go:118:33 | CONDITIONALS_BOUNDARY | `*b > *a` vs `>=` in `maxPtr`: on equal values the other pointer to the same value is returned, which only pointer identity could tell apart. |

### internal/catalog

| Mutant | Type | Reason |
| --- | --- | --- |
| select.go:66:83 | CONDITIONALS_BOUNDARY | `i >= 0` vs `i > 0` in `Resolve`: for index 0 the fallback looks the model up by its key and finds the same model at index 0 (shown is a subset of all). Also a blind spot, see below. |
| select.go:66:83 | CONDITIONALS_NEGATION | Blind spot: the condition after an `if` init with a closure lies in no coverage block; `TestResolve` fails when applied. |
| select.go:101:11 | CONDITIONALS_NEGATION | Blind spot: `case` expressions lie in no coverage block; `TestSortByBench` fails when applied. |
| select.go:101:24 | CONDITIONALS_NEGATION | Swaps the results 0 and 1 for (no value, value) and (no value, no value); `slices.SortStableFunc` only tests `cmp < 0`, so the order cannot change. Also a blind spot. |
| select.go:103:11 | CONDITIONALS_NEGATION | Blind spot (`case` expression); `TestSortByBench` fails when applied. |
| select.go:105:11 | CONDITIONALS_NEGATION | Blind spot (`case` expression); `TestSortByBench` fails when applied. |

### internal/config

| Mutant | Type | Reason |
| --- | --- | --- |
| config.go:179:92 | ARITHMETIC_BASE | `+` between two string constants becomes `-`, which does not compile; reported as not covered because constant declarations have no coverage blocks. |

### internal/fetch

| Mutant | Type | Reason |
| --- | --- | --- |
| fetch.go:17:33 | ARITHMETIC_BASE | `30 * time.Second` becomes `30 / time.Second` = 0, i.e. no timeout for the default client; only a server that hangs for more than 30 s would show it, which is not worth a 30 s test. |

### internal/gateway

None.

### internal/table

| Mutant | Type | Reason |
| --- | --- | --- |
| table.go (`Tokens`, `case` expressions) | CONDITIONALS_* | Blind spot: `case` expressions sit in no coverage block; `TestTokens` fails when they are applied by hand. |

### internal/menu

| Mutant | Type | Reason |
| --- | --- | --- |
| menu.go (`Pick`) | all | `Pick` only starts the bubbletea program on the real terminal; the picker model it runs is tested directly in `picker_test.go`. |

### internal/secrets

None.

### internal/targets

| Mutant | Type | Reason |
| --- | --- | --- |
| codex.go:56:9 | CONDITIONALS_NEGATION | Blind spot (`case` expression); `TestCodexCommand` fails when applied. |
| jsonfile.go:97:9 | CONDITIONALS_BOUNDARY | `k < len(path)` vs `<=` in `editJSON`: the outer loop only runs while the full path is missing, so the inner loop always stops at `k < len(path)` via `Find`; the bound is defensive. |
| jsonfile.go:97:9 | CONDITIONALS_NEGATION | Reported as TIMED OUT (counts as killed): the mutant inserts at the top level forever. The real loop terminates because the check after each insert guarantees one more path element is found per iteration. |
| jsonfile.go:110:62 | ARITHMETIC_BASE | Inside the defensive "cannot insert" error, which is unreachable: `insertMember` always produces a member `Find` locates (covered by `FuzzEditJSON`). |
| jsonfile.go:188:55 | CONDITIONALS_BOUNDARY | `pos < len(orig)` vs `<=`: the object's closing brace follows its last member, so `pos` never reaches the end of the input. |
| jsonfile.go:194:65 | CONDITIONALS_BOUNDARY | `i >= 0` vs `i > 0`: for `i == 0` adding `i` to `lineEnd` changes nothing. |
| target.go:114:61 | CONDITIONALS_BOUNDARY | `<` vs `<=` when sorting targets by name: the registry is keyed by name, so no two names are equal. |

<!-- internal/cli: its section follows below. -->

### internal/cli

No equivalent mutants. The setup dialogs are covered end to end by the
`setup*.txtar` scripts, which answer them on stdin in accessible mode
(`LLMENU_ACCESSIBLE=1`); the scripts' child processes count towards coverage.

| Mutant | Type | Reason |
| --- | --- | --- |
| bench.go (`autoBenchInterval`, `autoBenchTimeout`) | ARITHMETIC_BASE | Blind spot: constants have no coverage block; `TestAutoRefreshBench` fails when the interval is changed by hand. |
| config.go:42:15 | CONDITIONALS_NEGATION | Blind spot (`case` expression); `config.txtar` (broken keyring file) fails when applied. |
| config.go:44:15 | CONDITIONALS_NEGATION | Blind spot (`case` expression); `config.txtar` fails when applied. |
| root.go:27:11 | CONDITIONALS_NEGATION | Blind spot (`case` expression); `TestExecuteExitCodes` fails when applied. |
| setup.go:242:90 | CONDITIONALS_BOUNDARY | Blind spot: the condition after an `if` init with a closure lies in no coverage block; `TestApplyClaude` (non-Claude model first in the list) fails when applied. |
| setup.go:242:90 | CONDITIONALS_NEGATION | Same as above. |
| sync.go:53:14 | CONDITIONALS_NEGATION | Blind spot (`case` expression); `sync.txtar` (broken target config) fails when applied. |
