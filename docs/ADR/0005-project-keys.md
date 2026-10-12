# ADR-0005: One gateway key per project, switched globally, the main key without a project

- **Status:** Proposed
- **Date:** 2026-10-12
- **Deciders:** David Schowalter
- **Builds on:** [ADR-0002](0002-command-structure-and-targets.md), [ADR-0003](0003-configuration-and-setup.md)

## Context and Problem Statement

A LiteLLM gateway can hand out one key per project, so each customer's or
cost center's usage is billed and budgeted separately. llmenu knows exactly
one gateway key (keyring entry `gateway`, ADR-0003). Switching projects
meant overwriting it with `llmenu secrets set gateway`, and nothing showed
which project was paying.

Question: **How does llmenu hold several gateway keys and switch between
them quickly, without changing anything for users with a single key?**

## Decision Drivers

1. Users without projects notice nothing: no migration, same commands.
2. Switching is one command, and the active project is visible wherever a key
   is used.
3. No key in plain text in other tools' configurations (ADR-0002, rule 5).
4. A wrong or missing project key never falls back silently to another key,
   which would bill the wrong project.
5. Synced tools (pi) follow a switch without another `llmenu sync`.

## Considered Options

1. Full profiles: URL, key and default models per project.
2. **A project is only a name and a key; one active project, persisted in
   config.toml and overridable per shell with `LLMENU_PROJECT`.**
3. Projects bound to directories by a file in the repository.

## Decision

Option 2.

- **Storage:** keyring entry `gateway:<project>` next to `gateway`, which
  stays the main key. config.toml lists the projects (`gateway.projects`;
  the keyring cannot be enumerated, and completion must not touch it) and
  the active one (`gateway.project`, empty: the main key).
- **Resolution** in one place (`gatewayFrom`): `LLMENU_API_KEY` >
  `LLMENU_PROJECT` (`none`: the main key) > `gateway.project` > main key.
  An unknown project or one without a key is an error naming
  `llmenu project add`.
- **Commands:** `llmenu project` picks from the main key, all projects and
  "add project"; `add`, `use`, `use --none`, `list`, `remove`, `current`.
  `add` checks the key with the gateway before storing it.
- **Tools:** the references stay as they are. pi's `!llmenu secrets get
  gateway` returns the active project's key, so a switch needs no sync.
  Launched tools get the key at start; a running session keeps it.
- **Visibility:** the menu shows the project above the table, a launch prints
  `llmenu: project <name> · <model>` to stderr, and launched tools get
  `LLMENU_PROJECT` (also `none`), so a status line inside them shows the
  session's project rather than one switched to later.
  `llmenu project current` serves status lines and prompts without keyring
  or network. Opt-in `claude.statusline` passes llmenu's status line to
  Claude Code via `--settings`, for the session only.

## Consequences

### Positive

- Single-key users see no change; the main key is a regular choice in the
  picker, not a special case.
- One switch covers llmenu, launched tools and pi.
- Directory binding comes for free via direnv (`LLMENU_PROJECT` in `.envrc`).

### Negative

- `secrets get gateway` now depends on the active project, while `secrets
  set/delete gateway` always mean the main key.
- A global switch affects every shell that does not set `LLMENU_PROJECT`.

### Neutral / Follow-ups

- Different URLs or models per project are out of scope; they would need
  profiles (option 1) and per-gateway caches.
- `llmenu project list` could show spend and budget via LiteLLM's
  `/key/info`.

## Confirmation

`internal/cli/testdata/script/project.txtar` covers adding, switching,
`LLMENU_PROJECT`, the missing-key error, the picker, removal and the status
line; the existing scripts run unchanged without projects.
