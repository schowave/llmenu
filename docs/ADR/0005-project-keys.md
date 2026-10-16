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

Option 2, plus folder bindings in config.toml instead of option 3's file in
the repository: project names and keys are personal and must not be
committed.

- **Storage:** keyring entry `gateway:<project>` next to `gateway`, which
  stays the main key. config.toml lists the projects (`gateway.projects`;
  the keyring cannot be enumerated, and completion must not touch it) and
  the active one (`gateway.project`, empty: the main key).
  `gateway.dirs` binds folders to a project (`none`: the main key); a
  binding applies in the folder and below, the nearest one wins. Folders are
  stored absolute with symlinks resolved.
- **Resolution** in one place (`gatewayFrom`): `LLMENU_API_KEY` >
  `LLMENU_PROJECT` (`none`: the main key) > folder binding of the current
  directory > `gateway.project` > main key.
  An unknown project or one without a key is an error naming
  `llmenu project add`.
- **Commands:** `llmenu project` picks from the main key, all projects and
  "add project"; outside the home directory it then asks whether the choice
  applies everywhere or only in the current folder (which replaces the
  folder's earlier choice), and offers to remove the folder's own project.
  `add`, `use`, `use --none`, `use --here`, `unpin`, `list`, `remove`,
  `current`. `add` checks the key with the gateway before storing it;
  `remove` also drops the project's folders.
- **Tools:** the references stay as they are. pi's `!llmenu secrets get
  gateway` returns the active project's key, so a switch needs no sync.
  Launched tools get the key at start; a running session keeps it.
- **Visibility:** the menu shows the project above the table, a launch prints
  `llmenu: project <name> · <model>` to stderr, and launched tools get
  `LLMENU_PROJECT` (also `none`), so a status line inside them shows the
  session's project rather than one switched to later.
  `llmenu project current` serves status lines and prompts without keyring
  or network. `claude.statusline` (on by default) passes llmenu's status line to
  Claude Code via `--settings`, for the session only. It runs the user's
  own status line (project, then user settings) with the same session JSON
  and appends the project and the key's spend from LiteLLM's `/key/info`,
  cached for a minute. The key comes from the session's environment, so the
  status line never reads the keyring.
- **Spend:** the menu and `llmenu project list` show the budget that binds
  each key, plus the key's share (`total_spend`, the figure LiteLLM's UI
  shows per key). A team key spends from its team's max_budget, a personal
  key from its user's (both read from `/user/info`, which any key may call;
  `/team/info` is often admin-only); the key's own budget wins if it has less
  left. Keys are only sent to the gateway they are bound to.

## Consequences

### Positive

- Single-key users see no change; the main key is a regular choice in the
  picker, not a special case.
- One switch covers llmenu, launched tools and pi.
- Directory binding needs neither direnv nor a file in the repository.

### Negative

- `secrets get gateway` now depends on the active project, while `secrets
  set/delete gateway` always mean the main key.
- A global switch affects every shell that does not set `LLMENU_PROJECT`,
  except in folders with their own project.

### Neutral / Follow-ups

- Different URLs or models per project are out of scope; they would need
  profiles (option 1) and per-gateway caches.
- `llmenu project list` could show spend and budget via LiteLLM's
  `/key/info`.

## Confirmation

`internal/cli/testdata/script/project.txtar` covers adding, switching,
`LLMENU_PROJECT`, the missing-key error, the picker, removal, folder
bindings and the status line; `config_test.go` covers the folder lookup; the existing scripts run unchanged without projects.
