# 开发工具与示例

本页集中说明仓库自有的 Skills、可执行示例以及 native prebuilt 工具。第三方
submodule 自带文档仍属于对应 upstream，不迁入本指引。

## Agent Skills

`skills/` 按 Open Skills layout 提供项目级 GizClaw CLI skills。顶层
`gizclaw-cli` 负责通用请求路由；其余 skills 分别覆盖 context、server、Play 以及
Admin 的 gear、firmware、resource、credential、MiniMax tenant、voice、workspace
template 和 workspace 操作。

从仓库根目录按需安装，例如：

```sh
npx skills add . --skill gizclaw-cli
npx skills add . --skill gizclaw-admin-resources
```

增加 `-g` 可全局安装。每个 skill 的 `SKILL.md` 是自身行为和依赖的 source of truth；
清单以 `skills/` 下实际目录为准，不在文档中维护第二份固定列表。

## GenX Model Capability Probe

`examples/genx` 对 `examples/genx/models/*_openai.json` 中的 OpenAI-compatible
模型执行 live capability probe，检查 `GENERATE`、JSON output、tool calls 和声明的
expectation。它会访问真实 provider 并消耗额度；输出是本次运行结果，不能把历史输出
当作当前模型能力。

```sh
cd examples/genx
go run .
```

## Songs Audio Chain

`examples/songs` 串联内置多声部 songs、PCM mixer、PortAudio、MP3、Ogg 和可选
Opus loopback。Playback/recording 需要受支持平台的 CGO 与 native PortAudio。

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

`play-ogg` 保证读取本示例 `record-ogg` 产生的文件；更广泛的第三方 Ogg Opus
兼容性取决于 Opus header 和 granule semantics。

## Native Prebuilt Artifacts

`tools/audio/{mp3,ogg,opus,portaudio}` 和 `tools/ncnn` 从固定 upstream submodule
构建、打包并验证 committed prebuilt。共同流程为：

1. `build_prebuilt_<os>.sh` 写入 `.tmp/<component>-prebuilt/<platform>/` staging。
2. `package_prebuilt.sh <platform>` 复制 header/library 到 `third_party/**/prebuilt` 并生成 checksum manifest。
3. `verify_artifacts.sh <platform>` 验证文件、manifest/checksum，并拒绝误提交的 Git LFS pointer。

先初始化 submodule：

```sh
git submodule update --init --recursive
```

以 MP3 macOS arm64 为例：

```sh
tools/audio/mp3/build_prebuilt_darwin.sh
tools/audio/mp3/package_prebuilt.sh darwin-arm64
tools/audio/mp3/verify_artifacts.sh darwin-arm64
```

组件与产物：

| 工具目录 | Upstream | Committed artifact |
| --- | --- | --- |
| `tools/audio/mp3` | `third_party/audio/lame` | `third_party/audio/prebuilt/lame/<platform>/lib/libmp3lame.a` |
| `tools/audio/ogg` | `third_party/audio/libogg` | `third_party/audio/prebuilt/libogg/<platform>/lib/libogg.a` |
| `tools/audio/opus` | `third_party/audio/libopus` | `third_party/audio/prebuilt/libopus/<platform>/lib/libopus.a` |
| `tools/audio/portaudio` | `third_party/audio/portaudio` | `third_party/audio/prebuilt/portaudio/<platform>/lib/libportaudio.a` |
| `tools/ncnn` | `third_party/ncnn/upstream` | `third_party/ncnn/prebuilt/<platform>/lib/libncnn.a` |

所有工具支持 `darwin-arm64`、`darwin-amd64`、`linux-amd64` 和 `linux-arm64`。
macOS Apple Silicon 可通过 `TARGET_ARCH=amd64` 构建 amd64；Linux audio 脚本要求
target 与 host architecture 一致，不提供跨架构构建。示例：

```sh
TARGET_ARCH=amd64 tools/audio/opus/build_prebuilt_darwin.sh
TARGET_ARCH=arm64 tools/audio/opus/build_prebuilt_linux.sh
```

NCNN 固定使用 `NCNN_VULKAN=OFF` 与 `NCNN_C_API=ON`，并在 `build.env` 记录 upstream
commit/describe。Native package 是否可用仍以对应 package 的 build/runtime capability
检查为准；unsupported target 必须返回明确错误，不能产生占位输出。
