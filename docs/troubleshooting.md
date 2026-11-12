# Troubleshooting

[← README](../README.md)

**"Gateway check failed: HTTP 401"** during setup: the gateway does not
accept the key. Check that the URL is the gateway itself (not its admin UI)
and that the key is not expired or blocked.

**"the gateway key was stored for …, but the gateway URL now points to …"**:
llmenu only sends a key to the gateway it was stored for. If the move is
intended, confirm it with `llmenu config set gateway.url <new URL>`.

**No budget in the menu or status line**: the gateway does not let keys read
`/key/info` (or `/user/info` for user and team budgets). Check with
`curl -H "Authorization: Bearer $KEY" <gateway>/key/info`; llmenu shows
whatever the gateway does answer.

**"There's an issue with the selected model (aws/…)" in a plain `claude`**:
the model was picked with Enter in a session started by llmenu, so Claude Code
saved it as the default for every session. Run `/model` and pick one of
Claude's own models; in llmenu sessions pick with `s` instead.

**No llmenu line in Claude Code's status line**: it is only added to sessions
started with `llmenu claude`, and not with `claude.statusline` off
(`llmenu config get claude.statusline`). It runs `llmenu statusline`, so
`llmenu` must be on the `PATH`.

**Copilot CLI answers with an error about tool calling or streaming**: Copilot
needs a model that supports both; pick another one.
