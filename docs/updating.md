# Updating and uninstalling

[← README](../README.md)

Updating keeps settings and keys; `llmenu --version` shows the installed
version.

```sh
brew update && brew upgrade --cask llmenu        # Homebrew
scoop update; scoop update llmenu                # Scoop
go install github.com/schowave/llmenu/cmd/llmenu@latest   # Go
```

A normal uninstall (`brew uninstall --cask llmenu`, `scoop uninstall llmenu`,
or deleting `$(go env GOPATH)/bin/llmenu`) keeps settings, cache and keys, so
a reinstall picks up where you left off.

To start over or remove everything, run `llmenu reset` first, while llmenu is
still installed. It lists what it removes and asks before doing so: settings,
all keys in the keyring (projects included), caches, and the provider entry
`llmenu sync` wrote into pi and opencode. `llmenu reset --dry-run` only shows
the list. Then `llmenu setup` sets it up anew, or uninstall as above.
