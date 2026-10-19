Adding a tool (target): follow docs/ADR/0002-command-structure-and-targets.md, "Adding a target".
Before committing: `mise run ci`; the pre-commit hook (`mise run hooks`) covers only fmt, tidy, vet and lint.
Changed a dialog, the menu or other terminal output: drive it in a real terminal with `scripts/tui.sh` (sandbox with fake gateway, in tmux).
