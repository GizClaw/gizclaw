#!/usr/bin/env bash
# Publish verified image archives; preserve and verify any existing version.
set -euo pipefail
[[ $# == 4 ]] || { echo "usage: $0 IMAGE_DIR ASSET_DIR TAG SOURCE_COMMIT" >&2; exit 2; }
image_dir="$1" asset_dir="$2" tag="$3" source_commit="$4"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ && "$source_commit" =~ ^[0-9a-f]{40}$ ]]
image=ghcr.io/gizclaw/gizclaw
work="$(mktemp -d)"
container=
trap '[[ -z "$container" ]] || docker rm -f "$container" >/dev/null 2>&1; rm -rf "$work"' EXIT

# Only a registry's explicit missing-manifest response permits publication.
# Authentication, permission and transport failures never count as absence.
inspect_optional() {
  if docker buildx imagetools inspect --raw "$1" >"$work/index.json" 2>"$work/inspect.stderr"; then
    return 0
  fi
  if grep -Eqi 'manifest unknown|name unknown|MANIFEST_UNKNOWN|NAME_UNKNOWN|: not found' "$work/inspect.stderr"; then
    return 3
  fi
  cat "$work/inspect.stderr" >&2
  exit 1
}
registry_digest() {
  docker buildx imagetools inspect "$1" --format '{{json .Manifest}}' | jq -er '.digest | select(test("^sha256:[0-9a-f]{64}$"))'
}
verify_binary() {
  local ref="$1" arch="$2" expected="$3"
  docker pull --platform "linux/$arch" "$ref" >/dev/null
  docker image inspect "$ref" | jq -e --arg arch "$arch" --arg version "${tag#v}" --arg commit "$source_commit" '
    .[0] | .Os == "linux" and .Architecture == $arch and
    .Config.Labels["org.opencontainers.image.version"] == $version and
    .Config.Labels["org.opencontainers.image.revision"] == $commit and
    .Config.Labels["org.opencontainers.image.base.digest"] == "sha256:008173c23f95b170204355c12626cb5a965d779a7e1283b09e9cffbb1bf33ca3"
  ' >/dev/null
  container="$(docker create --platform "linux/$arch" "$ref")"
  docker cp "$container:/usr/bin/gizclaw" "$work/gizclaw"
  [[ "$(sha256sum "$work/gizclaw" | cut -d ' ' -f1)" == "$expected" ]]
  docker rm "$container" >/dev/null
  container=
}

status=0
inspect_optional "$image:$tag" || status=$?
if [[ "$status" == 3 ]]; then
  sources=()
  for arch in amd64 arm64; do
    local_image="gizclaw-runtime:${source_commit}-${arch}"
    docker load -i "$image_dir/runtime-$arch.tar" >/dev/null
    stage="$image:build-${tag}-${source_commit}-${arch}"
    status=0
    inspect_optional "$stage" || status=$?
    if [[ "$status" == 3 ]]; then
      docker tag "$local_image" "$stage"
      docker push "$stage"
    fi
    digest="$(registry_digest "$stage")"
    expected="$(cat "$image_dir/binary-$arch.sha256")"
    verify_binary "$image@$digest" "$arch" "$expected"
    sources+=("$image@$digest")
  done
  docker buildx imagetools create --tag "$image:$tag" \
    --annotation "index:org.opencontainers.image.version=${tag#v}" \
    --annotation "index:org.opencontainers.image.revision=$source_commit" \
    --annotation "index:org.opencontainers.image.source=https://github.com/GizClaw/gizclaw" "${sources[@]}"
fi
digest="$(registry_digest "$image:$tag")"
reference="$image@$digest"
docker buildx imagetools inspect --raw "$reference" >"$work/index.json"
jq -e --arg commit "$source_commit" --arg version "${tag#v}" '
  (.mediaType == "application/vnd.oci.image.index.v1+json" or .mediaType == "application/vnd.docker.distribution.manifest.list.v2+json") and
  (if .mediaType == "application/vnd.oci.image.index.v1+json" then
    .annotations["org.opencontainers.image.revision"] == $commit and
    .annotations["org.opencontainers.image.version"] == $version
   else true end) and
  [.manifests[].platform | {os,architecture}] == [{os:"linux",architecture:"amd64"},{os:"linux",architecture:"arm64"}]
' "$work/index.json" >/dev/null
platforms='[]'
for arch in amd64 arm64; do
  platform_digest="$(jq -er --arg arch "$arch" '.manifests[] | select(.platform.architecture == $arch) | .digest' "$work/index.json")"
  expected="$(cat "$image_dir/binary-$arch.sha256")"
  verify_binary "$image@$platform_digest" "$arch" "$expected"
  platforms="$(jq -c --arg arch "$arch" --arg digest "$platform_digest" --arg binary "$expected" \
    '. + [{os:"linux",architecture:$arch,digest:$digest,binary_sha256:$binary}]' <<<"$platforms")"
done
jq -n --arg image "$image" --arg tag "$tag" --arg commit "$source_commit" --arg digest "$digest" --arg reference "$reference" --argjson platforms "$platforms" '
  {schema_version:1,image:$image,tag:$tag,version:($tag | ltrimstr("v")),source_commit:$commit,digest:$digest,reference:$reference,
   base_image:"ubuntu:24.04@sha256:008173c23f95b170204355c12626cb5a965d779a7e1283b09e9cffbb1bf33ca3",platforms:$platforms}
' >"$asset_dir/container-image.json"
"$repo_root/build/check-container-receipt.sh" "$asset_dir/container-image.json" "$tag" "$source_commit" "$asset_dir"
[[ -z "${GITHUB_OUTPUT:-}" ]] || echo "reference=$reference" >>"$GITHUB_OUTPUT"
printf 'published and verified %s\n' "$reference"
