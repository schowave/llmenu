Adding a tool (target): follow docs/ADR/0002-command-structure-and-targets.md, "Adding a target".
Before committing: `mise run ci`; the pre-commit hook (`mise run hooks`) covers only fmt, tidy, vet and lint.
Changed a dialog, the menu or other terminal output: drive it in a real terminal with `scripts/tui.sh` (sandbox with fake gateway, in tmux).
A change users notice: add a line under "## Unreleased" in CHANGELOG.md in the same commit, written for users.
Release: review the changes since the last tag (`/code-review high <last tag>`), rename "## Unreleased" in CHANGELOG.md to "## vX.Y.Z – YYYY-MM-DD" with a new empty "## Unreleased" above, push main, then `mise run release vX.Y.Z`.
