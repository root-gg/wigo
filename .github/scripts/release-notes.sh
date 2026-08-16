#!/usr/bin/env bash
#
# Write the release notes for a tag on stdout : the list of the commits since
# the previous tag, one link per line, then the compare link.
#
# Usage : release-notes.sh <tag> [<revision>]
#
set -euo pipefail

tag="${1:?usage: release-notes.sh <tag> [<revision>]}"
revision="${2:-HEAD}"
repository="${GITHUB_REPOSITORY:-root-gg/wigo}"

# The tag being released is not created yet, so the most recent reachable tag
# is the previous release. It is excluded anyway to stay correct when the
# notes are regenerated for an existing tag. There is none for the very first
# release.
previous="$(git describe --tags --abbrev=0 --exclude="$tag" "$revision" 2>/dev/null || true)"
range="$revision"
if [ -n "$previous" ]; then
	range="$previous..$revision"
fi

echo "## What's Changed"

# The version bumps themselves are not worth a changelog entry, and a subject
# that shows up several times is only listed once, at its most recent commit.
git log --no-merges --invert-grep --grep='^version ' --pretty=format:'%H%x09%s' "$range" |
	awk -F'\t' -v repository="$repository" '
		!seen[$2]++ {
			printf "* [%s](https://github.com/%s/commit/%s)\n", $2, repository, $1
		}
	'

if [ -n "$previous" ]; then
	echo
	echo "**Full Changelog**: https://github.com/$repository/compare/$previous...$tag"
fi
