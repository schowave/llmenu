#!/usr/bin/env bash
# Drives llmenu's dialogs and menu in a real terminal, for checks a test
# cannot see (redraws, leftovers, key handling): the sandbox (scripts/sandbox.sh)
# runs in a detached tmux session, keys go in, the screen comes out as text.
#
# Usage: scripts/tui.sh <command>
#   start [fresh|resume|live]  start the sandbox shell (default: resume)
#   run <command line>         type a command into the shell and press Enter
#   keys <key>...              send tmux keys (Enter, Escape, Up, Down, Tab,
#                              BTab for Shift+Tab, C-c, or text)
#   snap                       print the screen, once it stopped changing
#   stop                       end the session and the fake gateway
#
# run and keys print the screen afterwards. Example:
#   scripts/tui.sh start fresh
#   scripts/tui.sh run llmenu setup
#   scripts/tui.sh keys Down Enter
#   scripts/tui.sh keys Escape
#   scripts/tui.sh stop
set -euo pipefail

command -v tmux >/dev/null || { echo "tmux not found: brew install tmux" >&2; exit 1; }
root=$(git rev-parse --show-toplevel)
t() { tmux -L llmenu-tui "$@"; }

# snap waits until two captures 0.3 s apart agree (at most ~6 s): llmenu
# may still be fetching from the gateway or redrawing.
snap() {
	local prev="" cur
	for _ in $(seq 20); do
		sleep 0.3
		cur=$(t capture-pane -p)
		[ "$cur" = "$prev" ] && break
		prev=$cur
	done
	printf '%s\n' "$cur" | grep -v '^[[:space:]]*$' || true
}

case ${1:-} in
start)
	t kill-server 2>/dev/null || true
	t new-session -d -x 120 -y 40 "$root/scripts/sandbox.sh ${2:-resume}"
	for _ in $(seq 60); do
		t capture-pane -p | grep -q '(llmenu sandbox)' && break
		sleep 0.5
	done
	snap
	;;
run)
	shift
	t send-keys -l "$*"
	t send-keys Enter
	snap
	;;
keys)
	shift
	for k in "$@"; do t send-keys "$k"; done
	snap
	;;
snap) snap ;;
stop) t kill-server 2>/dev/null || true ;;
*)
	sed -n '/^# Usage/,/^set -e/p' "$0" | sed '$d; s/^# \{0,1\}//' >&2
	exit 2
	;;
esac
