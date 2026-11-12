# Models and benchmarks

[← README](../README.md)

```sh
llmenu list                        # table, best benchmark first
llmenu list --sort price --all     # by price, including aliases and duplicates
llmenu list --json                 # for scripts
llmenu bench                       # refresh benchmark scores
```

In the menu, type to search, **Tab** switches the sort column (INTEL, TB4,
price; a column without any score is skipped, e.g. TB4 with a free
Artificial Analysis key) and Enter picks. The preview shows details and notes when a model
does not suit the tool (e.g. a non-Claude model in `llmenu claude`).

## How it works

llmenu calls the gateway's `/model/info`, normalizes model names across
providers (`bedrock/eu.anthropic.claude-sonnet-4-5-20250929-v1:0` and
`vertex_ai/claude-sonnet-4-5@20250929` are the same model), folds aliases and
duplicates and matches benchmark results. Budgets come from `/key/info` and
`/user/info`. The design decisions are recorded in [docs/ADR](ADR).
