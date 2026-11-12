#!/usr/bin/env bash
# Releases a version: `mise run release vX.Y.Z`.
#
# Before running it: review the changes since the last tag
# (`/code-review high <last tag>`), rename "## Unreleased" in CHANGELOG.md to
# "## vX.Y.Z – YYYY-MM-DD", add an empty "## Unreleased" above it, and push main.
#
# The script checks main, runs `mise run ci`, and publishes the GitHub release
# with the tag's CHANGELOG.md section as notes. Publishing starts
# .github/workflows/release.yml (GoReleaser, tap, bucket, provenance); the
# script follows it and then the install test, which installs the release with
# Homebrew, Scoop and `go install` and checks the version.
#
# Run it again with the same tag to pick up a release that stopped halfway.
set -euo pipefail

fail() { echo "release: $1" >&2; exit 1; }

tag=${1:-}
# vMAJOR.MINOR.PATCH and nothing else; release.yml accepts the same.
[[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "usage: mise run release vX.Y.Z"
cd "$(git rev-parse --show-toplevel)"
[ "$(git rev-parse --abbrev-ref HEAD)" = main ] || fail "not on main"
[ -z "$(git status --porcelain)" ] || fail "uncommitted changes"
git fetch -q origin main --tags
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] ||
	fail "main and origin/main differ (pull, or push your commits first)"

# A section runs from its "## " heading to the next one.
section() { awk -v name="$1" '/^## /{inside = ($2 == name)} inside && !/^## /' CHANGELOG.md | sed '/./,$!d'; }
notes=$(section "$tag")
[ -n "$notes" ] || fail "CHANGELOG.md has no \"## $tag – <date>\" section (rename \"## Unreleased\")"
[ -z "$(section Unreleased | tr -d '[:space:]')" ] ||
	fail "CHANGELOG.md still has entries under \"## Unreleased\"; move them into the $tag section"

if gh release view "$tag" >/dev/null 2>&1; then
	[ "$(git rev-parse "$tag^{commit}")" = "$(git rev-parse HEAD)" ] ||
		fail "release $tag exists already, on another commit"
	echo "release: $tag is already published, picking up where the last run stopped"
else
	git rev-parse -q --verify "refs/tags/$tag" >/dev/null &&
		fail "tag $tag exists without a release; delete it or pick another version"
	mise run ci
	notes_file=$(mktemp)
	trap 'rm -f "$notes_file"' EXIT
	printf '%s\n' "$notes" >"$notes_file"
	gh release create "$tag" --target "$(git rev-parse HEAD)" --title "$tag" --notes-file "$notes_file"
	git fetch -q origin --tags
fi

# wait_run <workflow> <jq filter on a run>: the newest matching run's id.
wait_run() {
	local id=""
	for _ in $(seq 60); do
		id=$(gh run list --workflow "$1" --limit 10 --json databaseId,headBranch,event,createdAt \
			--jq "[.[] | select($2)][0].databaseId // empty")
		[ -n "$id" ] && break
		sleep 5
	done
	[ -n "$id" ] || fail "no $1 run showed up; run this again once it has"
	echo "$id"
}

echo "release: following the release workflow…"
run=$(wait_run release.yml ".headBranch == \"$tag\"") || exit 1
gh run watch "$run" --exit-status >/dev/null || fail "release workflow failed: gh run view $run --log-failed"
finished=$(gh run view "$run" --json updatedAt --jq .updatedAt)

# install.yml starts when release.yml has finished (workflow_run).
echo "release: following the install test…"
run=$(wait_run install.yml ".event == \"workflow_run\" and .createdAt >= \"${finished%Z}\"") || exit 1
gh run watch "$run" --exit-status >/dev/null ||
	fail "install test failed (Homebrew, Scoop or go install): gh run view $run --log-failed"

echo "release: $tag is out; Homebrew, Scoop and go install deliver it"
