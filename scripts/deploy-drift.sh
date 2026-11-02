#!/bin/sh
# Advisory drift check used by `make deploy`.
#
# Deployment installs the newest published release tag, never the tip of the
# default branch. A feature can therefore be merged, reviewed, tested, and still
# not be the code that is actually running. This script compares the version
# about to be deployed against the default branch and warns when it is behind.
#
# It is advisory only and always exits 0, so a deployment is never blocked by a
# failed comparison. Whenever the count cannot be determined reliably it stays
# silent rather than guessing.
#
# Overridable for testing: GIT, ORIGIN, DEFAULT_BRANCH.

set -u

GIT=${GIT:-git}
ORIGIN=${ORIGIN:-origin}
DEFAULT_BRANCH=${DEFAULT_BRANCH:-main}

usage() {
	printf 'usage: %s VERSION\n' 'deploy-drift.sh'
	printf 'Prints a warning on stderr when VERSION is behind %s/%s.\n' "$ORIGIN" "$DEFAULT_BRANCH"
}

# drift_message VERSION COUNT
# Writes the warning for a positive COUNT. Emits nothing for zero, empty, or
# non-numeric counts so an unusable count can never fabricate a warning.
#
# Two independent checks guard the count. The case pattern is the portable one;
# the arithmetic test is the backstop. POSIX leaves `[ x -gt 0 ]` with a
# non-numeric operand as an error whose reporting differs between shells, so
# neither layer is relied on alone.
drift_message() {
	version=$1
	behind=$2

	case $behind in
	'' | *[!0-9]*) return 0 ;;
	esac

	[ "$behind" -gt 0 ] 2>/dev/null || return 0

	plural=s
	[ "$behind" -eq 1 ] && plural=

	printf 'warning: release %s is %s commit%s behind %s/%s\n' \
		"$version" "$behind" "$plural" "$ORIGIN" "$DEFAULT_BRANCH" 1>&2
}

# sync_refs VERSION
# Best-effort refresh of the two refs the comparison needs. `make deploy`
# discovers tags with `git ls-remote`, which does not create local objects, so
# without this the count would silently fail for tags that were never fetched.
# Every failure is ignored; a later count simply stays silent.
sync_refs() {
	version=$1
	$GIT fetch --quiet "$ORIGIN" \
		"refs/tags/$version:refs/tags/$version" \
		"refs/heads/$DEFAULT_BRANCH:refs/remotes/$ORIGIN/$DEFAULT_BRANCH" \
		>/dev/null 2>&1 || true
}

# commits_behind VERSION
# Prints the number of commits present on ORIGIN/DEFAULT_BRANCH but absent from
# VERSION. Prints nothing when git cannot answer. Only whitespace is stripped:
# deleting every non-digit would turn unusable output such as "0.5" into the
# plausible-looking "05" and fabricate a warning, so anything else is left intact
# for drift_message to reject.
commits_behind() {
	version=$1
	$GIT rev-list --count "$version..$ORIGIN/$DEFAULT_BRANCH" 2>/dev/null |
		tr -d '[:space:]'
}

main() {
	if [ "$#" -ne 1 ]; then
		usage 1>&2
		return 0
	fi

	version=$1
	case $version in
	-h | --help)
		usage
		return 0
		;;
	esac

	sync_refs "$version"
	behind=$(commits_behind "$version")
	drift_message "$version" "$behind"
	return 0
}

main "$@"
