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
