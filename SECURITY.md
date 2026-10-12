# Security policy

## Reporting a vulnerability

Please do not open a public issue for security problems. Report them through
GitHub's private vulnerability reporting instead: **Security → Report a
vulnerability** in this repository.

Include what you found, how to reproduce it and which version or commit is
affected. You will get a response as soon as possible.

## Scope

llmenu handles API keys for LLM gateways. Keys are stored in the operating
system's keyring and passed only to the child process it starts. Anything that
leaks a key (into files, process arguments, logs or other tools' configs) is in
scope.

## What to know when using llmenu

- **The key is visible to the tool and everything it starts.** llmenu hands the
  key to Claude Code (`ANTHROPIC_AUTH_TOKEN`), Codex CLI and opencode
  (`LLMENU_API_KEY`) as an environment variable. Every process those tools
  spawn (shell commands, MCP servers, build scripts) inherits it, and other
  processes of the same user can read it. A prompt injection that makes the
  tool run a command can therefore leak the key. Use a gateway key with a
  budget and a short lifetime, and one key per project (`llmenu project`).
- **The key goes to the configured gateway URL.** `llmenu setup` and
  `llmenu config set gateway.url` only accept `https://` (and `http://` for
  localhost). A URL from `config.toml` or `LLMENU_BASE_URL` is not checked that
  strictly; llmenu warns about plain http, but whoever can change either value
  can send the key to a server of their choice.
- **`LLMENU_KEYRING_FILE` stores keys in plain text.** It exists for tests and
  the development sandbox; the file is created with mode 0600. Do not use it
  for real keys.
