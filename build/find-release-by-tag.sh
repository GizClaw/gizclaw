#!/usr/bin/env bash
# Find one GitHub Release, including drafts, by its exact tag.

set -euo pipefail

repository="${1:-}"
tag="${2:-}"

[[ "$repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || {
  echo "repository must be an owner/name pair" >&2
  exit 2
}
[[ "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || {
  echo "tag must be a canonical stable SemVer tag" >&2
  exit 2
}
for command_name in gh jq; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "required command not found: $command_name" >&2
    exit 2
  }
done

release_pages=
if ! release_pages="$(gh api --paginate --slurp "repos/$repository/releases?per_page=100")"; then
  echo "failed to list GitHub Releases" >&2
  exit 1
fi
matches=
if ! matches="$(
  jq -ce --arg tag "$tag" '
    if type != "array" or any(.[]; type != "array") then
      error("GitHub Releases response is not a list of pages")
    else
      [.[][] | select(.tag_name == $tag)]
    end
  ' <<<"$release_pages"
)"; then
  echo "failed to inspect GitHub Releases" >&2
  exit 1
fi

case "$(jq 'length' <<<"$matches")" in
  0)
    exit 3
    ;;
  1)
    jq -c '.[0]' <<<"$matches"
    ;;
  *)
    echo "multiple Releases use tag $tag" >&2
    exit 1
    ;;
esac
