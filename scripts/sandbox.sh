#!/usr/bin/env bash
# Development sandbox for llmenu: a shell in which llmenu, pi and opencode only
# see .sandbox/ (config, cache, keys, HOME), claude, codex and copilot are stand-ins
# that print how they would be started, and by default a fake gateway answers
# on localhost. Nothing outside .sandbox/ is read or written, the OS keyring
# is never used.
#
# Usage: scripts/sandbox.sh fresh|resume|live
#   fresh   wipe .sandbox/ and start with the fake gateway (first-run experience)
#   resume  keep .sandbox/ from the last session, fake gateway
#   live    keep .sandbox/, no fake gateway: run `llmenu setup` against your
#           real gateway. Keys entered there are stored in PLAIN TEXT in
#           .sandbox/keyring.json; prefer exporting LLMENU_API_KEY instead.
set -euo pipefail

mode=${1:-fresh}
root=$(git rev-parse --show-toplevel)
sb=$root/.sandbox
port=${LLMENU_SANDBOX_PORT:-4111}

case $mode in
fresh) rm -rf "$sb" ;;
resume | live) ;;
*) echo "usage: $0 fresh|resume|live" >&2; exit 2 ;;
esac

mkdir -p "$sb/bin" "$sb/home"
echo "Building llmenu and sandbox tools ..."
go build -o "$sb/bin/llmenu" "$root/cmd/llmenu"
go build -o "$sb/bin/fakegateway" "$root/tools/fakegateway"
go build -o "$sb/bin/claude" "$root/tools/faketool"
cp "$sb/bin/claude" "$sb/bin/codex"
cp "$sb/bin/claude" "$sb/bin/copilot"

# Isolate every place the tools look at.
export HOME=$sb/home ZDOTDIR=$sb/home
export XDG_CONFIG_HOME=$HOME/.config XDG_CACHE_HOME=$HOME/.cache XDG_DATA_HOME=$HOME/.local/share XDG_STATE_HOME=$HOME/.local/state
export CODEX_HOME=$HOME/.codex
export COPILOT_HOME=$HOME/.copilot
export LLMENU_CONFIG=$sb/config.toml LLMENU_CACHE_DIR=$sb/cache LLMENU_KEYRING_FILE=$sb/keyring.json
export LLMENU_KEYRING_SERVICE=llmenu-sandbox
export PATH=$sb/bin:$PATH
export LLMENU_SANDBOX=$mode
unset LLMENU_BASE_URL LLMENU_AA_API_KEY LLMENU_AA_URL
if [ "$mode" != live ]; then
	unset LLMENU_API_KEY
	export LLMENU_AA_URL=http://127.0.0.1:$port/aa
	"$sb/bin/fakegateway" -addr "127.0.0.1:$port" -models "$root/internal/gateway/testdata/model_info.json" 2>"$sb/fakegateway.log" &
	gw=$!
	trap 'kill $gw 2>/dev/null' EXIT
fi

# A prompt that makes the sandbox obvious; the sandbox HOME has no dotfiles.
cat >"$HOME/.zshrc" <<'RC'
PROMPT='%F{yellow}(llmenu sandbox)%f %1~ %# '
RC
printf 'PS1="(llmenu sandbox) \\W \\$ "\n' >"$HOME/.bashrc"

cat <<MSG

llmenu sandbox ($mode) — everything lives in .sandbox/, exit the shell to leave.
MSG
if [ "$mode" != live ]; then
	cat <<MSG
  Fake gateway:  http://127.0.0.1:$port/   key: sk-dev
  Benchmarks:    key for "llmenu setup benchmarks": aa-dev
MSG
else
	cat <<MSG
  Real gateway: run "llmenu setup". Keys typed there are stored in plain text
  in .sandbox/keyring.json; export LLMENU_API_KEY before starting to avoid that.
MSG
fi
cat <<'MSG'
  claude, codex and copilot are stand-ins that only print how they would be started;
  pi and opencode (if installed) read their configs from the sandbox HOME.

  Try: llmenu claude · llmenu list · llmenu setup · llmenu bench · llmenu sync pi opencode

MSG

shell=${SHELL:-/bin/bash}
"$shell" -i || true
