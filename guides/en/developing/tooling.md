# Development Tools and Examples

This page centralizes repository-owned Skills, runnable examples, and native
prebuilt tooling. Documentation that belongs to third-party submodules remains
owned by the corresponding upstream project.

## Agent Skills

`skills/` contains project-level GizClaw CLI skills in the Open Skills layout.
The top-level `gizclaw-cli` skill routes general requests. Other skills cover
context, server, Play, and Admin operations for gears, firmware, resources,
credentials, MiniMax tenants, voices, workspace templates, and workspaces.

Install only the skills you need from the repository root, for example:

```sh
npx skills add . --skill gizclaw-cli
npx skills add . --skill gizclaw-admin-resources
```

Add `-g` for a global installation. Each skill's `SKILL.md` is the source of
truth for its behavior and dependencies; use the actual directories under
`skills/` as the inventory instead of maintaining another fixed list here.

## GenX Model Capability Probe

`examples/genx` runs live capability probes against the OpenAI-compatible
models in `examples/genx/models/*_openai.json`. It checks `GENERATE`, JSON
output, tool calls, and declared expectations. It contacts real providers and
consumes quota. Output describes that run only; historical output is not a
current model capability guarantee.

```sh
cd examples/genx
go run .
```

## Songs Audio Chain

`examples/songs` combines built-in multivoice songs, the PCM mixer, PortAudio,
MP3, Ogg, and optional Opus loopback. Playback and recording require CGO and a
supported native PortAudio platform.

```sh
cd examples/songs
CGO_ENABLED=1 go run . -mode list
CGO_ENABLED=1 go run . -mode play-song -song twinkle_star
CGO_ENABLED=1 go run . -mode play-song -songs twinkle_star,canon
CGO_ENABLED=1 go run . -mode record-mic -timeout 5s -output ./out/mic.mp3
CGO_ENABLED=1 go run . -mode play-mp3 -input ./out/mic.mp3
CGO_ENABLED=1 go run . -mode play-song -song twinkle_star -opus-loopback
CGO_ENABLED=1 go run . -mode record-ogg -timeout 5s -output-ogg ./out/mic.ogg
CGO_ENABLED=1 go run . -mode play-ogg -input-ogg ./out/mic.ogg
```

`play-ogg` is guaranteed for files produced by this example's `record-ogg`;
broader third-party Ogg Opus compatibility depends on Opus header and granule
semantics.

## Native Prebuilt Artifacts

`tools/audio/{mp3,ogg,opus,portaudio}` and `tools/ncnn` build, package, and
verify committed prebuilts from pinned upstream submodules. The common flow is:

1. `build_prebuilt_<os>.sh` writes staging output under `.tmp/<component>-prebuilt/<platform>/`.
2. `package_prebuilt.sh <platform>` copies headers/libraries into `third_party/**/prebuilt` and writes a checksum manifest.
3. `verify_artifacts.sh <platform>` validates files, manifest/checksums, and rejects accidental Git LFS pointer artifacts.

Initialize submodules first:

```sh
git submodule update --init --recursive
```

For example, build MP3 for macOS arm64 with:

```sh
tools/audio/mp3/build_prebuilt_darwin.sh
tools/audio/mp3/package_prebuilt.sh darwin-arm64
tools/audio/mp3/verify_artifacts.sh darwin-arm64
```

| Tool directory | Upstream | Committed artifact |
| --- | --- | --- |
| `tools/audio/mp3` | `third_party/audio/lame` | `third_party/audio/prebuilt/lame/<platform>/lib/libmp3lame.a` |
| `tools/audio/ogg` | `third_party/audio/libogg` | `third_party/audio/prebuilt/libogg/<platform>/lib/libogg.a` |
| `tools/audio/opus` | `third_party/audio/libopus` | `third_party/audio/prebuilt/libopus/<platform>/lib/libopus.a` |
| `tools/audio/portaudio` | `third_party/audio/portaudio` | `third_party/audio/prebuilt/portaudio/<platform>/lib/libportaudio.a` |
| `tools/ncnn` | `third_party/ncnn/upstream` | `third_party/ncnn/prebuilt/<platform>/lib/libncnn.a` |

The tools cover `darwin-arm64`, `darwin-amd64`, `linux-amd64`, and
`linux-arm64`. Apple Silicon may build macOS amd64 with `TARGET_ARCH=amd64`.
Linux audio scripts require the target to match the host architecture and do
not cross-build. Examples:

```sh
TARGET_ARCH=amd64 tools/audio/opus/build_prebuilt_darwin.sh
TARGET_ARCH=arm64 tools/audio/opus/build_prebuilt_linux.sh
```

NCNN uses fixed `NCNN_VULKAN=OFF` and `NCNN_C_API=ON` flags and records the
upstream commit/describe in `build.env`. Native availability remains governed
by each package's build/runtime capability checks; unsupported targets return
explicit errors instead of placeholder output.

## Repository Releases

The repository publishes a formal, non-prerelease Release only when a canonical
protected tag `vMAJOR.MINOR.PATCH` is pushed. A push to `main` does not build or
publish a Release.

Each Release contains exactly two Debian packages, four Terraform provider
packages, one standalone C SDK source archive, its checksum sidecar, two
Flutter SDK hosted pub archives, two npm SDK packages, `release-manifest.json`, and `SHA256SUMS`,
nineteen files in total, including `container-image.json` and four image archives; it does not
publish raw Linux executables. The Debian packages use `<version>` from the tag
in `gizclaw_<version>_{amd64,arm64}.deb`. The platform-neutral source payload is
named `gizclaw-c-sdk-<version>.tar.gz`; its adjacent `.sha256` contains the
archive digest and canonical filename. The Terraform provider packages are named
`terraform-provider-gizclaw_<version>_{darwin,linux}_{amd64,arm64}.zip`, and the
provider version equals the Release version; see
[Terraform Provider](/en/using/terraform) for installation and usage.

Runtime and Mem0 each deliver separate `linux/amd64` and `linux/arm64` Docker
save gzip archives. All four are mandatory formal Release assets and must pass
validation before publication. Mem0 uses the same source commit and OCI
version/revision labels. Its build definition is `build/mem0/Dockerfile` and
Python entry point is `cmd/mem0/gizclaw_mem0`. Native runners execute the `test`
target before exporting/loading the `runtime` image and checking version, CLI
and non-root Python service entry point. See the
[self-hosted Mem0 service](./stores/memory#self-hosted-mem0-service) for usage.

`build/build-mem0.sh [cn]` selects `build/mem0/Dockerfile.base` or
`Dockerfile.cn.base` before building the common application Dockerfile. Both use
the same pinned Python image digest and SDK dependencies. Standard uses PyPI; CN
uses the Tsinghua PyPI mirror. `PYTHON_BASE_IMAGE` overrides the Python registry
and CN permits a `PIP_INDEX_URL` override. `PLATFORM` selects
`linux/amd64|linux/arm64`; `TARGET=base|test|runtime`, `IMAGE`, `BUILD_VERSION`
and `BUILD_COMMIT` select the target/image identity. Release uses the standard
base. CN changes build sources while keeping model, API and runtime behavior.

Go build and base-image Dockerfiles live in `build/gizclaw/`.
`build/build-linux.sh` chooses the standard/CN base and exports the `gizclaw`
executable.

The Flutter SDK archives are named `flutter-gizclaw-<version>.tar.gz` and
`flutter-gizclaw_control-<version>.tar.gz` and use the pub hosted archive
layout: the archive root is the package root and contains only `pubspec.yaml`,
`LICENSE`, and the Git-tracked `lib/**/*.dart`.
`tools/flutter-sdk/package_archive.sh` rewrites the `pubspec.yaml` `version` to
the Release version, so every tag owns one immutable hosted package version; the
`version` in the source tree is a development placeholder.
`tools/flutter-sdk/verify_archive.sh` validates members, metadata, and pubspec
identity. `tools/flutter-sdk/consume_archives.sh` serves both archives from a
local static
[Hosted Pub Repository v2](https://github.com/dart-lang/pub/blob/master/doc/repository-spec-v2.md)
layout and has a Flutter consumer run `flutter pub get` and `flutter analyze`
with them as ordinary hosted dependencies. Publishing them to the object-store
pub repository belongs to Deploy and is outside this repository's Release
contract; see [Flutter SDK](/en/using/sdk/flutter) for usage.

The npm SDK assets are `npm-gizclaw-<version>.tgz` and
`npm-gizclaw-control-<version>.tgz`, with a `package/` root and package names
`@gizclaw/gizclaw` and `@gizclaw/gizclaw-control`. Their versions are injected from
the tag; see [TypeScript SDK](./sdk/typescript#release-contract) for packaging,
reproducibility, and consumer checks. Releases are their sole distribution output.
Deploy verifies digests from a selected published Release before downstream
hosting. This repository's installation entry is the
[Release tarball](/en/using/sdk/typescript).

For formal releases, the Git tag is the only source version. It is both the Go
module version and GitHub Release tag; removing its leading `v` gives the Debian,
Terraform provider, C, Flutter, and npm package versions. Formal tags must be stable canonical SemVer, without leading
zeroes, prerelease identifiers, or build metadata. Both annotated and
lightweight tags resolve to their full peeled commit, and that commit must
already be reachable from the current protected `main` head.

Before creating the first formal tag, a repository administrator must activate
a tag ruleset for `refs/tags/v*` that permits creation while restricting update
and deletion. The publication workflow checks the protected source branch and
tag ruleset before creating a formal Release; it does not own repository
administration.

Each Debian package owns only root-owned mode-`0755` `/usr/bin/gizclaw`.
Shared-library dependencies are derived from the packaged ELF and validated by
a clean matching-architecture Ubuntu 24.04 install, removal, reinstall, and
tamper/reinstall cycle.

`build/build-terraform-provider.sh` cross-compiles each Terraform provider
package with `CGO_ENABLED=0` on a Linux runner. Each zip contains only the
mode-`0755` executable `terraform-provider-gizclaw_v<version>`, with its
timestamp taken from the source commit, so rebuilds are byte-identical.
`build/check-release.sh` validates each zip's single entry name, execute mode,
and that its Mach-O/ELF header matches the declared platform and architecture.

Windows artifacts, the `gizclaw` CLI on macOS, and package-manager repository
publication are not part of this repository release contract.

Download and verify a Release without trusting filenames alone:

```sh
tag=v1.2.3
gh release download "$tag" --repo GizClaw/gizclaw --dir ".tmp/$tag"
(cd ".tmp/$tag" && sha256sum --check SHA256SUMS)
build/check-release.sh semver ".tmp/$tag" "$tag" "$(git rev-list -n 1 "$tag")"
```

`release-manifest.json` identifies the stable channel and binds every payload
name, byte size, and SHA-256 to the full source commit. Native payloads also
bind platform and architecture; the C SDK source entry binds module
`gizclaw_c_sdk`, version, and source commit; `dart-package` entries bind the pub
package name, version, and source commit; `npm-package` entries bind the full scoped
npm package name, version, and source commit. Debian entries also bind package metadata and
`/usr/bin/gizclaw`; `terraform-provider` entries also bind provider `gizclaw`,
the version, and the executable name inside the zip. A formal rerun accepts an
existing published Release only when its metadata and all nineteen downloaded files
match byte-for-byte. An
exact-tag draft left by an interrupted first upload must pass the same metadata,
inventory, digest, and byte-for-byte checks before the workflow publishes that
same draft. Partial, moved, duplicate exact-tag, or mismatched Releases fail
closed. The workflow never deletes, replaces, or overwrites a published SemVer
Release. Downstream Homebrew and APT channels independently own their signing,
hosting, retention, and live installation acceptance.

`release-manifest.json` uses `schema_version: 8`. Its `assets` contains exactly
16 payloads sorted by name with `LC_ALL=C`. The C SDK `.sha256` sidecar, manifest,
and `SHA256SUMS` bring the Release inventory to 19 files. Consumers must explicitly
validate schema 8 and the complete asset set; asset sets from different schemas
cannot be mixed. Each npm entry has only these fields:

| Field | Type and constraint |
| --- | --- |
| `name` | String: `npm-gizclaw-<version>.tgz` or `npm-gizclaw-control-<version>.tgz` |
| `kind` | Constant string `npm-package` |
| `size` | Positive integer: tgz size in bytes |
| `sha256` | 64 lowercase hex digits: SHA-256 of the entire tgz |
| `package` | `@gizclaw/gizclaw` or `@gizclaw/gizclaw-control`, matching the filename |
| `version` | Canonical `MAJOR.MINOR.PATCH` from the tag without `v` |
| `source_commit` | 40 lowercase hex digits: Git SHA matching the top-level source commit |

Manifest generation and validation cross-check `name` and `version` read from
`package/package.json` inside each tarball. npm entries have no `os`, `architecture`,
`module`, `installed_path`, `provider`, or `executable` fields.

### Container image archives

Runtime packages the same verified Debian executable using
`build/gizclaw/Dockerfile.runtime`, without recompiling it. The pinned Ubuntu
24.04 base supplies package-derived shared libraries, CA certificates and curl;
the image runs as UID/GID 10001. See [Container Image](/en/using/container) for
startup, mounts and Compose usage.

Archives are named `gizclaw-runtime_<version>_linux_<arch>.tar.gz` and
`gizclaw-mem0_<version>_linux_<arch>.tar.gz`, where `<version>` is the tag without
`v` and `<arch>` is `amd64` or `arm64`. `build/package-container-image.sh` exports
with `docker save` and `gzip -n`, without rebuilding runtime layers. Load them
directly with `docker load --input <archive>`. Each archive restores exactly one
local architecture reference: `gizclaw-runtime:<tag>-<arch>` or
`gizclaw-mem0:<tag>-<arch>`. There is no multiarch registry tag or floating `latest`.

`container-image.json` remains a `kind: container-image` manifest payload covered
by `SHA256SUMS`. Receipt schema 2 has top-level fields `schema_version`, `tag`,
`version`, `source_commit` and `images`. Its four `images` entries are ordered by
component (`mem0`, `runtime`) and architecture (`amd64`, `arm64`). They exactly
match the four `kind: container-archive` entries in manifest schema 8:

| Field | Type and constraint |
| --- | --- |
| `name` | Canonical image archive filename above |
| `kind` / `component` | `container-archive` / `runtime` or `mem0` |
| `os` / `architecture` | `linux` / `amd64` or `arm64` |
| `format` | Constant `docker-save+gzip` |
| `version` / `source_commit` | Tag without `v` / same full 40-character Git SHA |
| `size` | Positive integer: compressed file size in bytes |
| `sha256` | Compressed file SHA256: 64 lowercase hex digits |
| `image_reference` | Local architecture tag restored by loading |
| `image_id` | `sha256:<64 hex>`: build Docker's observed `.Id` |
| `config_digest` | `sha256:<64 hex>`: SHA256 of the archived image config |
| `archive_manifest_digest` | OCI/containerd layout only: SHA256 of the runnable platform manifest |
| `archive_index_digest` | Present when the archive has an OCI image index: SHA256 of that index blob |
| `binary_sha256` | Runtime only: `/usr/bin/gizclaw` SHA256 in the image and matching `.deb` |

Archive `sha256`, local `image_id` and registry manifest/index digests identify
different objects and cannot be interchanged. The receipt has no registry
`digest`, `reference` or `platforms` fields. Deploy consumers must validate schema
8, all 19 assets and source identity, select a component/architecture in schema
2 `images`, verify size/SHA256, load, check the restored reference, image ID,
platform and OCI version/revision, then run by image ID. Classic Docker's `.Id`
is `config_digest`; containerd storage may use `archive_manifest_digest` or
`archive_index_digest`. The build's `image_id` must match one of these three. Consumers may match any verified
archive identity and pin their actual loaded `.Id`. An archived manifest digest
does not imply prior registry publication. A containerd import of a classic archive may
reuse compressed layers and produce a new manifest ID. Acceptance then re-exports that
local image with bounded `docker save` and permits the new ID only if its sole
config SHA256 exactly matches the archived `config_digest`. Deploy owns TOS hosting
and server downloads/deployment. This repository does not publish or delete GHCR
packages; historical GHCR images and Releases remain available for rollback.
Debian, C, Flutter, npm and Terraform provider entry fields and names are unchanged.
Validate older schemas using scripts from their source tags.

`build/container_archive.py` checks gzip, Docker save manifest, the sole tag,
config SHA256, all uncompressed layer diff IDs, platform, non-root user and OCI identity
without extracting files. OCI indexes permit one runnable platform image plus
attestations bound through descriptors, config/layers and in-toto subjects;
extra runnable platforms or identity conflicts fail. Config filenames may be
`<sha256>.json`, `sha256:<sha256>` or `blobs/sha256/<sha256>`; each must match the
config's content hash. Docker Hub short names, `docker.io/` and `index.docker.io/`
aliases must retain the same repository and explicit tag. Empty-config OCI
attestations require the exact `{}` config bytes, a content-verified native
manifest subject and matching in-toto subjects. `build/check-container-receipt.sh` also recomputes the
Debian executable hash. `build/check-container-archive.sh` verifies the delivered
bytes before importing and checking the loaded image ID. The complete Runtime
gate retains CLI, `ldd`, CA, read-only config, SQLite/filesystem persistence,
`/server-info` identity, health, restart, exclusive workspace ownership, SIGTERM
exit and forced-stop recovery. Mem0 retains its test target and runtime entry
checks. Before formal publication, `container-load` repeats runtime acceptance
from archives on two fresh native runners. PR/main CI also exports, loads and
runs all four archives.

For reruns, `build/reuse-container-archive.sh` downloads the original compressed
bytes only from an exact-tag published Release or complete draft. It checks
source, manifest, checksums, receipt, archive and Debian identity before reuse
and repeats runtime acceptance. Only an explicitly absent exact-tag Release
permits a new archive build. Permission/transport errors, partial uploads,
older schemas or identity conflicts fail closed. The final publisher still
downloads and compares every asset byte-for-byte before accepting an idempotent
Release and never replaces assets. Separate Actions metadata parts only assemble
the receipt and are excluded from the formal inventory.

## Mutex scope inventory

`go run ./tools/quality mutexscope` scans every Git-tracked handwritten Go file in maintained modules, including tracked platform files that are inactive on the current host. It fingerprints reachable `Lock`/`RLock` critical sections, their release form, nested/risk operations, and returned unlock ownership transfers, then compares them with `tools/quality/mutexscope.reviewed.jsonl`. Missing, stale, duplicate, unsorted, wildcard, or non-intentional entries fail closed.

Use `-write-reviewed` only after inspecting every reported change and running deterministic concurrency plus race tests. The command is a source inventory, not a deadlock, starvation, race, or performance proof.
