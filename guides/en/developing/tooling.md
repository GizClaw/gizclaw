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

The repository publishes the same release asset classes through two channels:

- `latest` is a mutable snapshot replaced by every push to `main`.
- A canonical protected tag `vMAJOR.MINOR.PATCH` publishes an immutable formal,
  non-prerelease version.

Both channels contain exactly two Debian packages, the two Darwin executables,
`release-manifest.json`, and `SHA256SUMS`; neither channel publishes raw Linux
executables. A formal Release uses `<version>` from its tag in
`gizclaw_<version>_{amd64,arm64}.deb`. A `latest` package uses the
deterministic Debian version
`0.0.0+main.<source-epoch>.<12-character-source-commit>` so each moving snapshot
still has package metadata bound to its exact source commit.

For formal releases, the Git tag is the only source version. It is both the Go
module version and GitHub Release tag; removing its leading `v` gives the Debian
package version. Formal tags must be stable canonical SemVer, without leading
zeroes, prerelease identifiers, or build metadata. Both annotated and
lightweight tags resolve to their full peeled commit, and that commit must
already be reachable from the current protected `main` head. The mutable
`latest` tag is a snapshot pointer, not a Go module SemVer.

Before creating the first formal tag, a repository administrator must activate
a tag ruleset for `refs/tags/v*` that permits creation while restricting update
and deletion. Repository-wide immutable Releases must remain disabled because
they would also lock the required moving `latest` Release. The publication
workflow checks these conditions and fails before creating a formal Release; it
does not own repository administration.

Each Debian package owns only root-owned mode-`0755` `/usr/bin/gizclaw`.
Shared-library dependencies are derived from the packaged ELF and validated by
a clean matching-architecture Ubuntu 24.04 install, removal, reinstall, and
tamper/reinstall cycle. The Darwin assets are separate single-architecture
command-line executables built and run on native macOS 15 Intel and Apple
Silicon runners. Windows, universal macOS binaries, application bundles,
installers, notarization, and package-manager repository publication are not
part of this repository release contract.

Download and verify a Release without trusting filenames alone:

```sh
tag=v1.2.3
gh release download "$tag" --repo GizClaw/gizclaw --dir ".tmp/$tag"
(cd ".tmp/$tag" && sha256sum --check SHA256SUMS)
chmod +x ".tmp/$tag"/gizclaw-darwin-*
build/check-release.sh semver ".tmp/$tag" "$tag" "$(git rev-list -n 1 "$tag")"
```

`release-manifest.json` identifies the snapshot or stable channel and binds
every payload name, platform, architecture, byte size, and SHA-256 to the full
source commit. Debian entries also bind package metadata and
`/usr/bin/gizclaw`. A formal rerun accepts an existing Release only when its
published metadata and all six downloaded files match byte-for-byte. Any draft,
partial, moved, or mismatched formal Release fails closed. A failed first upload
can leave a draft; an administrator must inspect and delete that draft before
retrying. The workflow never deletes or overwrites a published SemVer Release.
Downstream Homebrew and APT channels independently own their signing, hosting,
retention, and live installation acceptance.
