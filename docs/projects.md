# Projects: one key per project

[← README](../README.md)

If your gateway hands out a separate key per project, e.g. to bill each
customer or cost center on its own, store them side by side and switch with
one command. URL and models stay the same; only the key changes.

```sh
llmenu project add acme        # asks for the key hidden, checks it with the gateway
llmenu project                 # pick: main key, acme, …, or add a project
llmenu project use acme        # or switch directly
llmenu project use --none      # back to the main key
llmenu project use acme --here # only in this folder and below
llmenu project unpin           # the folder follows the global choice again
LLMENU_PROJECT=acme llmenu claude   # just for this command or shell
```

A folder can have its own project: outside your home directory,
`llmenu project` asks after the choice whether it applies everywhere or only
in the current folder. Choosing again in the same folder replaces its
project. The binding lives in llmenu's config, not in the repository, and
`llmenu project list` shows all bound folders.

The switch holds for every later command and for pi; there is nothing to sync
again. Running sessions keep their key. The active project is shown in the
menu and when a tool starts (`llmenu: project acme · aws/claude-opus-5-5`).
`llmenu project current` prints it for your own prompt without touching the
keyring or the network.

<details>
<summary>The project in your shell prompt</summary>

In your own prompt or status line script:

```sh
project=$(llmenu project current) && [ -n "$project" ] && printf 'project %s · ' "$project"
```

starship (`~/.config/starship.toml`):

```toml
[custom.llmenu]
command = "llmenu project current"
when = "llmenu project current | grep -q ."
format = "[$output]($style) "
```

</details>
