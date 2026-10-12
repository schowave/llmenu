#!/usr/bin/env bash
# Records docs/demo.gif from docs/demo.tape with VHS (https://github.com/charmbracelet/vhs)
# against the fake gateway. Everything lives in a temporary directory: keys
# come from the environment, claude is the sandbox stand-in, the OS keyring
# and real tool configs are never touched.
set -euo pipefail

command -v vhs >/dev/null || { echo "vhs not found: brew install vhs" >&2; exit 1; }
root=$(git rev-parse --show-toplevel)
port=${LLMENU_DEMO_PORT:-4112}
tmp=$(mktemp -d)
trap 'kill "${gw:-}" 2>/dev/null; rm -rf "$tmp"' EXIT

go build -o "$tmp/llmenu" "$root/cmd/llmenu"
go build -o "$tmp/fakegateway" "$root/tools/fakegateway"
go build -o "$tmp/claude" "$root/tools/faketool"
"$tmp/fakegateway" -addr "127.0.0.1:$port" -models "$root/internal/gateway/testdata/model_info.json" 2>"$tmp/fakegateway.log" &
gw=$!
sleep 1

export PATH=$tmp:$PATH
export LLMENU_CONFIG=$tmp/config.toml LLMENU_CACHE_DIR=$tmp/cache
export LLMENU_KEYRING_FILE=$tmp/keyring.json LLMENU_KEYRING_SERVICE=llmenu-demo
export LLMENU_API_KEY=sk-dev LLMENU_AA_API_KEY=aa-dev LLMENU_AA_URL=http://127.0.0.1:$port/aa
llmenu config set gateway.url "http://127.0.0.1:$port/" >/dev/null
llmenu config set gateway.name demo >/dev/null
llmenu config set tools claude >/dev/null
llmenu bench >/dev/null

cd "$root"
vhs docs/demo.tape
echo "Wrote docs/demo.gif"
