# Changelog

What changed for people using llmenu. Each release on GitHub takes its notes from its section here.

## Unreleased

## v0.12.1 – 2026-11-13

- **`/model` in Claude Code shows a score again with a free Artificial Analysis key and `bench.metric = tb4`.** The list was already sorted by the Intelligence Index then, like the menu, but no entry showed a score; now each one names its Intelligence Index.
- **`go install` needs Go 1.27.4 or later**, as `go.mod` has since v0.12.0; the README said 1.27.2.
- The release notes now come from [CHANGELOG.md](https://github.com/schowave/llmenu/blob/main/CHANGELOG.md), which lists every release so far.

## v0.12.0 – 2026-11-12

### Security fix: please update

llmenu is now built with Go 1.27.4, which fixes GO-2026-6859 in the standard library's `net/http`. govulncheck reports the vulnerable code as reachable from llmenu. v0.11.1 and earlier are affected.

```sh
brew update && brew upgrade --cask llmenu        # Homebrew
scoop update; scoop update llmenu                # Scoop
go install github.com/schowave/llmenu/cmd/llmenu@latest   # Go
```

### The Coding Index is gone

Artificial Analysis no longer publishes the Coding Index; since the V2 API it was always empty. llmenu drops it everywhere: the CODE column and the `code` sort key in the menu and in `llmenu list`, the "Coding Index" line in `llmenu bench`, and the coding score in Claude Code's `/model` list.

- **Your config keeps working:** if `bench.metric` is set to `code`, llmenu sorts by INTEL. `llmenu config set bench.metric code` is now rejected; the choices are `intel` and `tb4`.
- **Tab** in the menu cycles INTEL, TB4 (with a Pro key) and price.

### Verifiable releases

- **SBOM:** every archive comes with an SPDX SBOM (`<archive>.sbom.json`) listing the Go modules it was built from.
- **Provenance as a file:** the signed build provenance is now also attached to the release as `llmenu.intoto.jsonl`, so you can check an archive offline:

  ```sh
  gh attestation verify llmenu_0.12.0_darwin_arm64.tar.gz --repo schowave/llmenu --bundle llmenu.intoto.jsonl
  ```

  The SBOMs are listed in `checksums.txt`, so the provenance covers them too.

### Other changes

- **Website:** https://schowave.github.io/llmenu/ with the demo, the tools and the install commands.
- **Shorter README:** projects, budget, models, configuration, troubleshooting and updating moved to their own pages under [docs/](https://github.com/schowave/llmenu/tree/main/docs).
- **Discussions** are open for questions and ideas: https://github.com/schowave/llmenu/discussions

## v0.11.1 – 2026-11-10

### Benchmarks work again

Artificial Analysis retired its legacy data API on 2026-11-04, so `llmenu bench` failed with `HTTP 410 … Gone`. llmenu now reads the new V2 API. Your existing key keeps working.

- **Free and Pro keys:** llmenu tries the full model list first. A free key gets HTTP 403 there, and llmenu falls back to the free list. The free list only has the Intelligence Index and output speed. Terminal-Bench and Long Context Reasoning need a Pro key, and `llmenu bench` says so.
- **Coding Index:** Artificial Analysis no longer publishes it. `llmenu bench` now names only the scores the API delivered.
- **More matches:** a model that only has a measured output speed now gets that speed in the preview.
- **Menu and `list`:** the TB4 column only appears if a model has a Terminal-Bench score. **Tab** skips sort columns without any score. If `bench.metric` names a score no model has, llmenu sorts by INTEL.
- The automatic refresh fetches the list's pages in parallel, so it stays within its short timeout.

### Other changes

- Built with Go 1.27.3, which fixes GO-2026-6809 in `crypto/x509`.

## v0.11.0 – 2026-10-17

### GitHub Copilot CLI

`llmenu copilot` picks a model in the menu and starts [GitHub Copilot CLI](https://github.com/github/copilot-cli) with it, using your gateway as Copilot's own provider (BYOK). No GitHub login is needed, and `~/.copilot` stays untouched.

```sh
llmenu copilot                                  # menu, then Copilot starts
llmenu copilot -p "fix the failing test"        # no terminal: copilot.default
llmenu copilot --model azure/gpt-6-sol          # no menu
```

- **Protocol:** Copilot speaks Chat Completions to `<gateway>/v1` by default. `llmenu config set copilot.provider_type anthropic` switches to the Messages API.
- **Offline:** `llmenu config set copilot.offline true` also turns off telemetry, web tools and auto-update.
- **Model details:** Copilot gets the model's context and output limits from the gateway. Claude models also get their Anthropic ID, so Copilot counts tokens correctly.
- **Setup:** `llmenu setup copilot` asks for the default model, protocol and offline mode. Copilot is also in the list of tools in `llmenu setup tools`.
- **`providers.json` warning:** if a `~/.copilot/providers.json` declares providers, Copilot ignores what llmenu passes, and llmenu warns about it at startup.

### Setup: back to the list

`llmenu setup` without a section now returns to the list after each section, ticks the ones you have done and ends with *Done*. **Esc** in a section goes back to the list without saving it. Errors such as an unreachable gateway no longer end the setup. In other dialogs, Esc now cancels like Ctrl+C.

## v0.10.0 – 2026-10-16

### All gateway models in Claude Code's `/model`

In a session started with `llmenu claude`, `/model` now lists all of the gateway's models instead of Claude's own, with context, prices and benchmark, sorted like the menu. You can switch models without restarting:

```text
2. aws/claude-opus-5-5 ✔          1M · $4.40 in / $22.00 out · intelligence 57.6
3. azure/gpt-6-sol                922k · $2.00 in / $10.00 out · intelligence 47.6
6. gcp/claude-sonnet-4-5          200k · $3.00 in / $15.00 out, more above 200k · intelligence 20.7
```

Pick with **`s`** (this session only). If you press **Enter**, Claude Code saves the gateway model as its default in `~/.claude/settings.json`. A plain `claude` on your subscription then fails with "There's an issue with the selected model". Run `/model` once and pick one of Claude's own models to fix it.

To keep Claude's own list: `llmenu config set claude.picker false`.

### Project and spend in the status line, on by default

Sessions started with `llmenu claude` now show the project and the key's budget below your own status line without any setup:

```text
project acme · team budget $40.00 / $100.00 · this key $12.45 · resets in 19d
```

To turn it off: `llmenu config set claude.statusline false`. If you answered "No" to this in an earlier `llmenu setup claude`, that answer was not stored, so the status line is now on for you too.

## v0.9.0 – 2026-10-16

### Long-prompt prices at a glance

A `*` after the output price marks models that cost more once the prompt is long, in the menu and in `llmenu list`. Claude Haiku 5.5, for example, costs five times as much above 100k tokens, a size Claude Code sessions reach quickly. The preview lists the levels:

```text
aws/claude-haiku-5-5       1M      0.11     0.55*
```

### Free or unknown, not 0.00

A model whose gateway price is 0, e.g. a self-hosted one, now shows `free`. If the gateway names no price at all, the table shows `-` and the preview says the price is unknown. Before, both looked like `0.00`. `llmenu list --json` has a new `no_price` field.

### Speed and long-context reasoning in the preview

The benchmark block in the preview now also shows the model's output speed in tokens per second and Artificial Analysis' Long Context Reasoning score:

```text
Benchmarks (claude-sonnet-5-5)
  Intelligence    56.0
  Terminal-Bench  63.6 %
  Long context    82.7 %
  Output speed   141 tok/s at the provider
```

The speed is measured at the model's own provider, not through your gateway, so take it as a guide. Both values appear after the next benchmark refresh; run `llmenu bench` to get them now.

### The Coding Index column only when you sort by it

Artificial Analysis no longer publishes the Coding Index for most current models, so the column was mostly `-`. It now appears only while you sort by it (Tab in the menu, or `bench.metric = code`); the preview still shows the value.

## v0.8.2 – 2026-10-16

### The budget stays in Claude Code's status line

With a project key, the status line shows the key's budget. On gateways with large teams, LiteLLM's `/user/info` takes over a second, and the status line often gave up before it answered; after an hour without a fresh value the budget disappeared. llmenu now asks `/user/info` at the same time as `/key/info` instead of after it, and the status line waits up to 3 seconds. Thanks to the one-minute cache, that wait comes at most about once a minute.

### Long-prompt prices in opencode

`llmenu sync` now writes opencode's `context_over_200k` price for models whose gateway price rises at 200k tokens. opencode knows only this one threshold, so levels at other sizes (e.g. 100k or 272k) are left out rather than billed at the wrong size.

## v0.8.1 – 2026-10-15

### Faster start

The menu no longer waits for the key's budget. On gateways with large teams, LiteLLM's `/user/info` alone can take over a second, and the menu waited for it on nearly every start. It now opens as soon as the models are loaded; the budget appears in the top right when it arrives.

## v0.8.0 – 2026-10-15

### Correct prices for Vertex AI on regional endpoints

LiteLLM bills Vertex AI models on a regional endpoint (a `vertex_location` other than `global`) with the model's `regional_endpoint_uplift_multiplier` on top, typically +10 %. llmenu showed the price without it, so `gcp/` models looked cheaper than their `aws/` twins although they cost the same. The uplift is now included everywhere: the menu, `llmenu list`, sorting, the slot suggestions for Claude Code, and the prices `llmenu sync` writes into pi and opencode. Where both cost the same, the suggestion may now be the `aws/` model instead of the `gcp/` one.

### Prices for long prompts

Some models cost more once the prompt crosses a size, e.g. Claude Haiku 5.5 five times as much above 100k tokens, or GPT 6 twice the input price above 272k. Above the threshold LiteLLM bills the whole request at the higher price. The preview now lists these levels under the prices:

```text
$ per 1M tokens
  in 0.11 · out 0.55
  prompt > 100k: in 0.55 · out 2.75
```

## v0.7.1 – 2026-10-15

### The same model at several providers stays selectable

If your gateway offers a model at more than one provider, e.g. `aws/claude-haiku-5-5` and `gcp/claude-haiku-5-5`, the menu and `llmenu list` now show both, so you can pick the provider. Before, llmenu showed only one of them. Aliases such as `aws/claude-opus-latest` stay hidden, and copies of a model at the same provider (e.g. several regions) are still shown once.

## v0.7.0 – 2026-10-13

### A project per folder

A folder can now have its own project, so the right key is used in each customer's repository without switching. Outside your home directory, `llmenu project` asks after the choice:

```text
Applies to
> Only in ~/repos/acme and below
  Everywhere
  Remove the project of ~/repos/acme (use the global choice, …)
```

Choosing again in the same folder replaces its project. Subfolders follow it, a global switch leaves it alone, and `LLMENU_PROJECT` still wins. Without the picker: `llmenu project use acme --here` and `llmenu project unpin`. `llmenu project list` shows all bound folders. The bindings live in llmenu's config, not in the repository.

### A default tool for `llmenu`

`llmenu` without a command can now start your tool after the menu, so you need not type `llmenu claude` every time. The setup asks for it; if you are already set up, `llmenu` asks once. Change it with `llmenu default` (a list), `llmenu default codex`, or `llmenu default --none` to only print the model name. Captured as in `"$(llmenu)"`, it always prints only the name, so scripts keep working.

### `llmenu reset`

Removes everything llmenu stored, to start over or before uninstalling: settings, all keys in the keyring (projects included), caches, and the provider entry `llmenu sync` wrote into pi and opencode (the previous file stays as `*.bak`; entries llmenu did not write stay). It lists what it removes and asks first; `--dry-run` only lists, `--yes` skips the question. The README's manual removal steps are replaced by it.

## v0.6.0 – 2026-10-13

### Budget and spend

llmenu now shows the budget of the key you are using and how much of it is gone:

- **In the menu**, next to the project: `user budget $18.00 / $50.00 · this key $12.45 · resets in 19d`. On a narrow table the details are shortened rather than running into the search field.
- **In `llmenu project list`**, a new SPEND column for the main key and every project.
- **In Claude Code's status line** (see below).

A project's team key spends from its team's budget, a personal key from your user budget; a key's own budget is shown if it has less left. Without any budget llmenu shows what the key has spent. The numbers come from LiteLLM's `/key/info` and `/user/info`, which every key may call about itself, so no admin rights are needed; `/team/info` is only a fallback. They are LiteLLM's estimate, not the provider's invoice. If the gateway answers none of this, nothing is shown and everything else works as before.

### Status line adds to yours

`claude.statusline` no longer replaces your own status line. llmenu runs the `statusLine` command from your project or user settings with the same session data and appends one line:

```text
project acme · team budget $40.00 / $100.00 · this key $12.45 · resets in 19d
```

The key comes from the session's environment (never the keyring), the gateway is asked at most once a minute, and the cache holds only a hash of URL and key. `llmenu setup claude` now asks whether to turn the status line on.

### README

New sections *Budget and spend* and *Troubleshooting*; the demo shows the budget in the menu.

## v0.5.0 – 2026-10-12

### Security hardening

- **The key only goes to its gateway.** llmenu records which gateway a key was stored for (`llmenu setup`, `llmenu config set gateway.url`; keys from older versions are bound on first use). If `config.toml` or `LLMENU_BASE_URL` later points elsewhere, llmenu asks on a terminal and otherwise refuses to send the key. A key from `LLMENU_API_KEY` is not bound.
- **No plain http by accident.** `setup` and `config set gateway.url` accept `http://` only for localhost; a URL from the config or environment that uses plain http triggers a warning.
- **Symlink-safe writes.** Config, state, benchmarks, the keyring file and the files of pi and opencode (including their `*.bak` backups) are written via random temporary names, so a planted symlink is never followed.
- **Bounded responses:** gateway and benchmark responses are limited to 16 MB.
- **Keyring file:** `LLMENU_KEYRING_FILE` (tests and sandbox only) is always written with mode 0600.
- **Signed provenance for release archives:** `gh attestation verify <archive> --repo schowave/llmenu`.
- [SECURITY.md](https://github.com/schowave/llmenu/blob/main/SECURITY.md) now explains what a tool started by llmenu can see (the key is in its environment).

### README

More compact; the projects section is titled "one key per project".

## v0.4.0 – 2026-10-12

### Projects: one gateway key per customer

If your gateway hands out one key per project (e.g. for separate billing per customer), llmenu now keeps them side by side and switches with one command. URL and models stay the same; only the key changes.

```sh
llmenu project add acme        # key asked for hidden, checked with the gateway
llmenu project                 # pick: main key, acme, …, or add a project
llmenu project use acme
llmenu project use --none      # back to the main key
LLMENU_PROJECT=acme llmenu claude   # just for this command or shell (none = main key)
```

- **Nothing changes without projects:** the existing key is the main key, no migration.
- **pi follows a switch** without another `llmenu sync`: `llmenu secrets get gateway` returns the active project's key.
- **No silent fallback:** a project without a key is an error, so usage is never billed to the wrong project. `LLMENU_API_KEY` still wins, e.g. in CI.
- **Always visible:** the menu shows the active project, a launch prints `llmenu: project acme · <model>`, and launched tools get `LLMENU_PROJECT`. `llmenu project current` serves your own status line or shell prompt (no keyring, no network).
- **Optional Claude Code status line:** `llmenu config set claude.statusline true` shows project and model in sessions started by llmenu, without touching `~/.claude`.

Details: [README](https://github.com/schowave/llmenu/blob/main/docs/projects.md), [ADR-0005](https://github.com/schowave/llmenu/blob/main/docs/ADR/0005-project-keys.md).

### README

- Leads with why llmenu is useful, for whom, and what it needs; a recorded demo shows the menu.

## v0.3.0 – 2026-10-12

### Setup

- **`llmenu setup` asks what you want to set up:** everything, or one part — e.g. only Claude Code's models for opus/sonnet/haiku and the default model. `llmenu setup claude` still works as a shortcut.
- **First run sets up everything:** without a config (new install, or after removing all data), llmenu offers the full setup — gateway, coding tools, default models, benchmark key — not only the gateway.

### Benchmarks

- Entering the Artificial Analysis key in setup **fetches the benchmarks right away**.
- With a key, missing or older-than-14-days benchmarks are **refreshed automatically** before the menu or `llmenu list` opens (at most once a day, 3 s timeout, silent on errors).
- Without benchmarks, the menu's footer suggests `llmenu bench`.

### Menu

- The selected row keeps the green top values, and the selection bar runs through the whole row.

## v0.2.0 – 2026-10-11

### New model menu

The menu is rebuilt to read like a table:

- Search line and column header on **top**, the **best model first**.
- **Tab** switches the sort column: INTEL → TB4 → CODE → price. The cursor stays on its model.
- Clearer columns: `$IN/1M`, `$OUT/1M`, `TB4 %`, context as `1M` / `1.05M` instead of `1000k`. The preview explains every column.
- Top benchmark values are highlighted; search matches are highlighted in the model name.
- A compact preview with the model's details, prices and benchmarks; it moves below the table in narrow terminals.
- Notes such as "not a Claude model" appear in the preview instead of marking the rows.

### Setup

- Only **Claude Code** is preselected under "Coding tools you use" until you choose.
- The Claude Code section has one page per slot, and each title explains what the slot is for.

### Also

- `llmenu list` shows context as `1M` too.

## v0.1.3 – 2026-10-11

### Changes

- **Remove everything with Homebrew:** `brew uninstall --zap --cask llmenu` now also deletes llmenu's settings and caches. A normal `brew uninstall --cask llmenu` still keeps them for a later reinstall. See the README for removing keys and the entries in pi and opencode.
- README: every command is a separate code block with a copy button; new sections on updating and uninstalling.

## v0.1.2 – 2026-10-11

### Fix

- **Homebrew on macOS:** installing 0.1.1 showed "„llmenu“ nicht geöffnet / “llmenu” Not Opened", because the cask ran llmenu to generate shell completion before the quarantine was removed. Completion files now ship pre-generated in the archives, so Homebrew no longer runs the binary during installation.

## v0.1.1 – 2026-10-11

### Highlights

- **Choose your coding tools in setup.** `llmenu setup` now asks which tools you use (Claude Code, Codex CLI, opencode, pi). It only asks about those, `llmenu sync` writes into those, and the help hides the others. New `llmenu setup codex` for Codex's default model.
- **Shell completion** for commands, settings and their values, key names and model names after `--model` — instant, from a local cache. Homebrew sets it up automatically.
- **Clearer help**: commands grouped by task, examples, a tool overview, `llmenu help environment`.
- **Menu legend**: the header explains what `!` means (e.g. not a Claude model).
- **`llmenu bench`** explains how to get the free Artificial Analysis key when none is set.

### Fixes

- `go install` builds report their real version instead of `dev`.
- The setup shows an example gateway URL as a placeholder instead of a value you have to delete.
- The menu cursor starts on the best model when there is no default.
- An empty tool selection is kept; slots without a gateway model can stay unset.
- Backups of synced configs never get a more open file mode than the config.

## v0.1.0 – 2026-10-11

First release of **llmenu**: pick models from a LiteLLM gateway by price, context and benchmarks, and wire them into your coding tools.

### Highlights

- **Menu** with fuzzy search, prices, context size and benchmarks from Artificial Analysis (`llmenu bench`)
- **Launch** Claude Code and Codex CLI against the gateway: `llmenu claude`, `llmenu codex`
- **Sync** the gateway's models into pi and opencode: `llmenu sync`
- **Setup** that can be re-run at any time; keys are stored in the OS keyring
- Works with any LiteLLM gateway; macOS, Linux and Windows
