#!/bin/sh
# Wait for the tag's publisher before deployment downloads release assets.
set -eu

[ "$#" -eq 2 ] || { echo 'usage: wait-release.sh REPOSITORY VERSION' >&2; exit 2; }
repository=$1
version=$2
command -v gh >/dev/null || { echo 'gh is required; install GitHub CLI and run gh auth login' >&2; exit 2; }

assets_ready() {
	ready=$(gh release view "$version" --repo "$repository" --json assets,isDraft \
		--jq '(.isDraft == false) and ([.assets[].name] | index("infoe-bots-linux-amd64") != null and index("infoe-bots-linux-amd64.sha256") != null)' 2>/dev/null) || return 1
	[ "$ready" = true ]
}

assets_ready && exit 0
echo "Waiting for $version release publication..."

# A newly pushed tag may not have a run yet. Bound discovery to one minute;
# gh run watch then follows the run for as long as the publisher needs.
attempt=0
while :; do
	run_id=$(gh run list --repo "$repository" --workflow publish.yml \
		--branch "$version" --event push --limit 1 --json databaseId \
		--jq '.[0].databaseId // empty') || {
		echo 'Could not query the publish workflow; check GitHub CLI authentication and connectivity' >&2
		exit 1
	}
	[ -z "$run_id" ] || break
	assets_ready && exit 0
	[ "$attempt" -lt 12 ] || {
		echo "No Publish release run found for $version after 60 seconds; check https://github.com/$repository/actions/workflows/publish.yml" >&2
		exit 1
	}
	attempt=$((attempt + 1))
	sleep 5
done

run_url="https://github.com/$repository/actions/runs/$run_id"
echo "Watching $run_url"
gh run watch "$run_id" --repo "$repository" --exit-status --interval 5 || {
	echo "Release publication failed or could not be watched: $run_url" >&2
	exit 1
}

# Allow release metadata to become visible after a successful workflow.
attempt=0
until assets_ready; do
	[ "$attempt" -lt 12 ] || {
		echo "Publish workflow completed but $version release assets are unavailable: $run_url" >&2
		exit 1
	}
	attempt=$((attempt + 1))
	sleep 5
done
