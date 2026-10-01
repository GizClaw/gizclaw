#!/usr/bin/env bash
# Recheck protected source/tag policy immediately before external publication.
set -euo pipefail
[[ $# == 3 ]] || { echo "usage: $0 REPOSITORY TAG SOURCE_COMMIT" >&2; exit 2; }
GH_REPO="$1" TAG="$2" SOURCE_COMMIT="$3"
GITHUB_API_URL="${GITHUB_API_URL:-https://api.github.com}"
[[ "$GH_REPO" == GizClaw/gizclaw ]]
[[ "$TAG" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ && "$SOURCE_COMMIT" =~ ^[0-9a-f]{40}$ ]]
test "$(gh api "repos/$GH_REPO/branches/main" --jq .protected)" = true

ruleset_ok=false
creation_blocked=false
ruleset_ids=()
rulesets_page=1
while true; do
  rulesets="$(curl \
    --fail-with-body \
    --silent \
    --show-error \
    --header 'Accept: application/vnd.github+json' \
    --header 'X-GitHub-Api-Version: 2022-11-28' \
    "$GITHUB_API_URL/repos/$GH_REPO/rulesets?includes_parents=true&per_page=100&page=$rulesets_page")"
  while IFS= read -r ruleset_id; do
    ruleset_ids+=("$ruleset_id")
  done < <(jq -r '.[] | select(.target == "tag" and .enforcement == "active") | .id' <<<"$rulesets")
  [[ "$(jq 'length' <<<"$rulesets")" -eq 100 ]] || break
  ((rulesets_page += 1))
done
for ruleset_id in "${ruleset_ids[@]}"; do
  ruleset="$(curl \
    --fail-with-body \
    --silent \
    --show-error \
    --header 'Accept: application/vnd.github+json' \
    --header 'X-GitHub-Api-Version: 2022-11-28' \
    "$GITHUB_API_URL/repos/$GH_REPO/rulesets/$ruleset_id")"
  if jq -e '
    (.conditions.ref_name.include | index("refs/tags/v*") != null) and
    (.conditions.ref_name.exclude | length == 0) and
    ([.rules[].type] | index("creation") != null)
  ' <<<"$ruleset" >/dev/null; then
    creation_blocked=true
  fi
  if jq -e '
    (.conditions.ref_name.include | index("refs/tags/v*") != null) and
    (.conditions.ref_name.exclude | length == 0) and
    ([.rules[].type] | index("update") != null) and
    ([.rules[].type] | index("deletion") != null) and
    ([.rules[].type] | index("creation") == null)
  ' <<<"$ruleset" >/dev/null; then
    ruleset_ok=true
  fi
done
if [[ "$ruleset_ok" != true || "$creation_blocked" != false ]]; then
  echo "an active refs/tags/v* ruleset allowing creation and restricting update and deletion is required" >&2
  exit 1
fi

remote_tag_commit="$(
  git ls-remote origin "refs/tags/$TAG" "refs/tags/$TAG^{}" |
    awk '$2 ~ /\^\{\}$/ { peeled=$1 } $2 !~ /\^\{\}$/ { direct=$1 } END { print peeled ? peeled : direct }'
)"
test "$remote_tag_commit" = "$SOURCE_COMMIT"
git fetch --force --no-tags origin refs/heads/main:refs/remotes/origin/main
git merge-base --is-ancestor "$SOURCE_COMMIT" refs/remotes/origin/main
