#!/usr/bin/env bash
# Runs the tests with coverage and fails if the total is below the threshold
# (LLMENU_MIN_COVERAGE, default 85). Used by `mise run cover` and CI.
# tools/ and cmd/ are thin wrappers without logic and are left out.
set -euo pipefail

min=${LLMENU_MIN_COVERAGE:-85}
pkgs=$(go list ./... | grep -v -e '/tools/' -e '/cmd/')
# shellcheck disable=SC2086
go test -covermode=atomic -coverprofile=coverage.out $pkgs >/dev/null
go tool cover -func=coverage.out | awk '/^total:/ { print }'
total=$(go tool cover -func=coverage.out | awk '/^total:/ { sub("%", "", $3); print $3 }')
if awk -v t="$total" -v m="$min" 'BEGIN { exit !(t + 0 < m + 0) }'; then
	echo "Coverage ${total}% is below the minimum of ${min}%." >&2
	exit 1
fi
echo "Coverage ${total}% (minimum ${min}%)."
