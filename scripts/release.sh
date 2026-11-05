#!/bin/sh
# Release automation used by `make release`.
#
# Publishing is a tag push: .github/workflows/publish.yml builds the Linux/amd64
# binary, its SHA-256 checksum, and a provenance attestation for any strict
# SemVer tag, then opens the matching GitHub Release. `make deploy` later installs
# the newest such tag. So a release is only ever two things: a tag, and that tag
# pointing at the merged default branch.
#
# Tagging by hand invited the same two mistakes repeatedly: tagging a commit that
# was not the tip of the default branch (releasing work nobody merged, or leaving
# merged work out), and bumping the version by eye. Both are settled here before
# anything is written, and nothing is pushed without an explicit yes.
#
# Overridable for testing: GIT, SEMVER, ORIGIN, DEFAULT_BRANCH, DRY_RUN,
# ASSUME_YES, VERSION, MESSAGE.

set -u

GIT=${GIT:-git}
SEMVER=${SEMVER:-semver}
ORIGIN=${ORIGIN:-origin}
DEFAULT_BRANCH=${DEFAULT_BRANCH:-main}
DRY_RUN=${DRY_RUN:-false}
ASSUME_YES=${ASSUME_YES:-}
VERSION=${VERSION:-}
MESSAGE=${MESSAGE:-}

# The same strict SemVer shape the Makefile and publish workflow accept, so a
# tag this script approves is never one the workflow later rejects.
SEMVER_PATTERN='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$'

usage() {
	printf 'usage: %s [BUMP]\n' 'release.sh'
	printf 'Bump the newest strict SemVer tag on %s and push it, which publishes\n' "$ORIGIN"
	printf 'the release through GitHub Actions. BUMP is patch (default), minor, or\n'
	printf 'major. Set VERSION=v1.2.3 to release an exact version instead.\n'
	printf '\nEnvironment: VERSION, MESSAGE, ASSUME_YES, DRY_RUN, GIT, SEMVER,\n'
	printf 'ORIGIN, DEFAULT_BRANCH.\n'
}

# fail MESSAGE [CODE]
# Every rejection names the state it found, because the point of refusing is to
# tell the releaser what to fix, not merely that something was wrong.
fail() {
	printf 'release: %s\n' "$1" >&2
	exit "${2:-1}"
}

run() {
	if [ "$DRY_RUN" = true ]; then
		printf 'dry run: %s\n' "$*"
		return 0
	fi
	printf '+ %s\n' "$*"
	"$@"
}

# count RANGE
# Prints the commit count for RANGE with whitespace removed, and nothing when
# git cannot answer. Callers reject a non-numeric answer rather than guessing.
count() {
	$GIT rev-list --count "$1" 2>/dev/null | tr -d '[:space:]'
}

# require_count VALUE LABEL
# Stops on a count that is empty or holds anything but digits: a release
# decision made on an unusable count is worse than no release at all.
require_count() {
	case $1 in
	'' | *[!0-9]*) fail "cannot $2" ;;
	esac
	return 0
}

# latest_tag
# Prints the newest strict SemVer tag, or nothing when there is none. Tags the
# workflow would reject are filtered out so a stray v1.2 or v1.2.3.4 cannot
# become the base of the next bump.
latest_tag() {
	$GIT for-each-ref --format='%(refname:short)' refs/tags 2>/dev/null |
		grep -E "$SEMVER_PATTERN" |
		sort -V |
		tail -n 1
}

# next_version BASE LEVEL
# Prints LEVEL's successor of BASE. Any prerelease suffix is dropped first so a
# retired v1.2.0-rc.3 can never seed the stable release line below it.
next_version() {
	base=$1
	level=$2

	core=${base#v}
	core=${core%%-*}
	[ -n "$core" ] || core=0.0.0

	major=${core%%.*}
	rest=${core#*.}
	minor=${rest%%.*}
	patch=${rest#*.}

	case $level in
	major)
		major=$((major + 1))
		minor=0
		patch=0
		;;
	minor)
		minor=$((minor + 1))
		patch=0
		;;
	patch) patch=$((patch + 1)) ;;
	esac

	printf 'v%s.%s.%s\n' "$major" "$minor" "$patch"
}

# validate_version VERSION
# Confirms the shape locally, then, when node-semver is installed, takes it as
# an independent second opinion on the string we produced. The checker is never
# the thing that computes the version, so an absent or odd `semver` cannot
# invent a release number; it can only refuse one.
validate_version() {
	candidate=$1
	printf '%s' "$candidate" | grep -Eq "$SEMVER_PATTERN" ||
		fail "$candidate is not a strict SemVer tag such as v1.2.3"

	if command -v "$SEMVER" >/dev/null 2>&1; then
		normalised=$("$SEMVER" "${candidate#v}" 2>/dev/null | tr -d '[:space:]') || normalised=
		[ "$normalised" = "${candidate#v}" ] ||
			fail "$SEMVER does not accept $candidate as a release version"
	fi
	return 0
}

# trails BASE CANDIDATE
# Succeeds when CANDIDATE sorts strictly above BASE. sort -V is the same ordering
# `make deploy` uses to pick the newest tag, so the two never disagree about
# which release is current.
trails() {
	base=$1
	candidate=$2

	[ -n "$base" ] || return 0
	[ "$base" != "$candidate" ] || return 1
	[ "$(printf '%s\n%s\n' "$base" "$candidate" | sort -V | tail -n 1)" = "$candidate" ]
}

# tag_exists VERSION
# Checks the local repository and the remote. A tag already on the remote would
# make the push fail mid-release, and a lingering local one is usually the
# remains of a push that failed earlier.
tag_exists() {
	$GIT rev-parse -q --verify "refs/tags/$1" >/dev/null 2>&1 && return 0
	$GIT ls-remote --exit-code --tags "$ORIGIN" "refs/tags/$1" >/dev/null 2>&1 && return 0
	return 1
}

# preflight
# Guards the tag's target. A release is published from the tag, so the tag has to
# sit on a clean, committed, pushed, reviewed tip of the default branch, created
# by the repository-local identity this project's policy requires.
preflight() {
	$GIT rev-parse --git-dir >/dev/null 2>&1 || fail 'not a git repository'

	[ -z "$($GIT status --porcelain --untracked-files=no 2>/dev/null)" ] ||
		fail 'working tree has uncommitted changes; commit or stash them first'

	branch=$($GIT symbolic-ref --quiet --short HEAD 2>/dev/null) ||
		fail 'HEAD is detached; check out '"$DEFAULT_BRANCH"' first'
	[ "$branch" = "$DEFAULT_BRANCH" ] ||
		fail "HEAD is on $branch; releases are tagged on $DEFAULT_BRANCH"

	name=$($GIT config --local --get user.name 2>/dev/null)
	email=$($GIT config --local --get user.email 2>/dev/null)
	{ [ -n "$name" ] && [ -n "$email" ]; } ||
		fail 'set the repository-local git user.name and user.email before releasing'

	return 0
}

# sync_refs
# The comparison against origin and the newest tag are only as good as the refs
# behind them, so a failed fetch stops the release instead of planning it from
# stale state.
sync_refs() {
	$GIT fetch --quiet "$ORIGIN" \
		"refs/tags/*:refs/tags/*" \
		"+refs/heads/$DEFAULT_BRANCH:refs/remotes/$ORIGIN/$DEFAULT_BRANCH" \
		>/dev/null 2>&1 ||
		fail "cannot fetch from $ORIGIN; the release point needs the current tags and $DEFAULT_BRANCH"
}

# check_synced
# HEAD must equal the remote default branch: not ahead (that is unreviewed work
# on a branch that may never merge) and not behind (that silently drops the
# commits someone else already merged).
check_synced() {
	ahead=$(count "$ORIGIN/$DEFAULT_BRANCH..HEAD")
	require_count "$ahead" 'compare HEAD with the remote default branch'
	behind=$(count "HEAD..$ORIGIN/$DEFAULT_BRANCH")
	require_count "$behind" 'compare the remote default branch with HEAD'

	[ "$ahead" -eq 0 ] ||
		fail "HEAD has $ahead commit(s) missing from $ORIGIN/$DEFAULT_BRANCH; merge them through a pull request first"
	[ "$behind" -eq 0 ] ||
		fail "$ORIGIN/$DEFAULT_BRANCH has $behind commit(s) missing from HEAD; pull before releasing"
	return 0
}

# preview BASE NEXT DESCRIPTOR SINCE
# Prints exactly what is about to become public, because a published release is
# the one action here that cannot be quietly undone.
preview() {
	base=$1
	next=$2
	descriptor=$3
	since=$4

	printf 'base      %s\n' "${base:-(none)}"
	printf 'release   %s (%s)\n' "$next" "$descriptor"
	printf 'tagger    %s <%s>\n' \
		"$($GIT config --local --get user.name)" \
		"$($GIT config --local --get user.email)"
	if [ -n "$base" ]; then
		printf '%s commit(s) since %s:\n' "$since" "$base"
		$GIT log --format='  %h %s' "$base..HEAD"
	else
		printf 'first release; latest commits:\n'
		$GIT log --format='  %h %s' -n 20
	fi
	return 0
}

main() {
	level=${1:-patch}
	case $level in
	-h | --help)
		usage
		return 0
		;;
	esac
	case $level in
	patch | minor | major) ;;
	*)
		printf 'release: unknown bump level %s\n' "$level" >&2
		usage >&2
		exit 2
		;;
	esac
	case $DRY_RUN in
	true | false) ;;
	*) fail 'DRY_RUN must be true or false' ;;
	esac

	preflight
	sync_refs
	check_synced

	base=$(latest_tag)
	if [ -n "$VERSION" ]; then
		next=$VERSION
		descriptor='explicit VERSION'
	else
		next=$(next_version "$base" "$level")
		descriptor="$level bump"
	fi
	validate_version "$next"

	tag_exists "$next" && fail "$next already exists; choose another VERSION or BUMP"
	trails "$base" "$next" ||
		fail "$next does not trail the newest release $base; releases only move forward"

	since=
	if [ -n "$base" ]; then
		since=$(count "$base..HEAD")
		require_count "$since" 'count the commits since '"$base"
		[ "$since" -gt 0 ] || fail "$base already points at HEAD; there is nothing new to release"
	fi

	preview "$base" "$next" "$descriptor" "$since"

	if [ "$ASSUME_YES" != true ] && [ "$ASSUME_YES" != yes ]; then
		printf 'Tag %s and publish it? [y/N] ' "$next"
		read -r answer || answer=
		case $answer in
		y | Y | yes | YES) ;;
		*)
			printf 'release cancelled\n'
			return 0
			;;
		esac
	fi

	tag_message=${MESSAGE:-$next}
	run $GIT tag -a "$next" -m "$tag_message" ||
		fail 'creating the tag failed; nothing was pushed'
	run $GIT push "$ORIGIN" "refs/tags/$next" ||
		fail "pushing $next failed; remove the local tag with 'git tag -d $next' and retry"

	if [ "$DRY_RUN" = true ]; then
		printf '\nDry run: %s was not tagged and nothing was pushed.\n' "$next"
		return 0
	fi

	printf '\nTagged %s and pushed it to %s.\n' "$next" "$ORIGIN"
	printf 'The Publish release workflow builds and attests the binary, then opens\n'
	printf 'the GitHub Release. Once it succeeds, deploy with:\n\n'
	printf '  make deploy VERSION=%s\n' "$next"
	return 0
}

main "$@"
