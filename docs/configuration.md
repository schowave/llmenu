# Configuration

[← README](../README.md)

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
| `LLMENU_BASE_URL`, `LLMENU_API_KEY` | gateway URL and key (the key wins over a project's) |
| `LLMENU_PROJECT` | project whose key to use, `none` for the main key |
| `LLMENU_AA_API_KEY` | Artificial Analysis key |
| `LLMENU_CONFIG`, `LLMENU_CACHE_DIR` | alternative config file and cache directory |
| `LLMENU_ACCESSIBLE=1` | line-based setup prompts, e.g. for screen readers |

`llmenu help` lists all commands, `llmenu help <command>` explains one.
