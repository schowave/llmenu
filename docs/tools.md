# Using llmenu with your tools

[← README](../README.md)

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

Gateway, key and models are passed in the environment (Claude Code, Copilot
CLI) or via `-c` overrides (Codex CLI); `~/.claude`, `~/.codex` and
`~/.copilot` stay untouched. Defaults: `llmenu setup claude`,
`llmenu setup codex`, `llmenu setup copilot`.

Copilot CLI uses the gateway as its own provider (BYOK) and needs no GitHub
login then. It speaks Chat Completions to `<gateway>/v1` by default;
`llmenu config set copilot.provider_type anthropic` switches to the Messages
API. `copilot.offline true` also turns off telemetry, web tools and
auto-update; a `COPILOT_OFFLINE` from your shell does not count. A
`~/.copilot/providers.json` that declares providers wins over what llmenu
passes; llmenu warns at startup.

In a Claude Code session started by llmenu, `/model` lists all of the
gateway's models with context, prices and benchmark instead of Claude's own,
so you can switch without restarting. Pick with **`s`** (this session only):
**Enter** makes Claude Code save the gateway model as its default in
`~/.claude/settings.json`, and a plain `claude` on your subscription then fails
until you pick another model with `/model` (see
[Troubleshooting](troubleshooting.md)). To keep Claude's own list:
`llmenu config set claude.picker false`.

```sh
llmenu sync --dry-run    # opencode and pi: show what would change
llmenu sync              # write the gateway's models, limits and prices
llmenu opencode          # start opencode with the gateway key
pi                       # pi asks for the key itself: !llmenu secrets get gateway
```

Only llmenu's own provider entry is changed; comments and your other
providers stay, and the previous file is kept as `*.bak`. Run `llmenu sync`
again when the gateway's models change.
