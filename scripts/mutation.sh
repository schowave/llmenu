#!/usr/bin/env bash
# Mutation testing with Gremlins. Used by `mise run mutation`,
# `mise run mutation:changed` and the mutation workflow.
#
#   scripts/mutation.sh                     everything; fails below the thresholds
#   scripts/mutation.sh ./internal/targets  one package; fails below the thresholds
#   scripts/mutation.sh --diff <ref>        only lines changed against <ref>;
#                                           informational, lists surviving mutants
#
# Known equivalent mutants are listed in docs/mutation-testing.md.
set -euo pipefail

# Gremlins derives each mutant's timeout from the coverage run; a cached
# (instant) run makes nearly every mutant time out. Always run the tests.
export GOFLAGS="${GOFLAGS:+$GOFLAGS }-count=1"

efficacy=${LLMENU_MIN_EFFICACY:-97}
mcover=${LLMENU_MIN_MUTANT_COVERAGE:-93}

paths=()
diff_base=""
while [ $# -gt 0 ]; do
	case $1 in
	--diff) diff_base=$2; shift 2 ;;
	*) paths+=("$1"); shift ;;
	esac
done
[ ${#paths[@]} -gt 0 ] || paths=(.)
args=()

if [ -n "$diff_base" ]; then
	# Gremlins mutates everything when the diff is empty, so check first.
	if git diff --quiet "$diff_base" -- '*.go' ':!*_test.go'; then
		echo "No Go code changed against $diff_base; nothing to mutate."
		exit 0
	fi
	# Small diffs can consist of a single equivalent mutant, so thresholds
	# would be noise here; surviving mutants are reported instead.
	efficacy=0 mcover=0
	args=(--diff "$diff_base")
fi

gremlins unleash \
	--workers "${LLMENU_MUTATION_WORKERS:-0}" \
	--timeout-coefficient 3 \
	--exclude-files 'tools/' --exclude-files 'cmd/' --exclude-files '_test\.go$' \
	--threshold-efficacy "$efficacy" \
	--threshold-mcover "$mcover" \
	--output mutation.json \
	${args[@]+"${args[@]}"} "${paths[@]}"

if [ -n "$diff_base" ]; then
	found=$(jq -r '.files[] | .file_name as $f | .mutations[] | select(.status == "LIVED" or .status == "NOT COVERED") | "\($f)\t\(.line)\t\(.column)\t\(.type)\t\(.status)"' mutation.json)
	if [ -z "$found" ]; then
		echo "All mutants in the changed lines were killed."
		exit 0
	fi
	echo "Mutants in the changed lines that no test catches (add a test, or list equivalents in docs/mutation-testing.md):"
	while IFS=$'\t' read -r file line col type status; do
		echo "  $file:$line:$col $type ($status)"
		if [ -n "${GITHUB_ACTIONS:-}" ]; then
			echo "::warning file=$file,line=$line,col=$col::Mutant $status: $type"
		fi
	done <<<"$found"
fi
