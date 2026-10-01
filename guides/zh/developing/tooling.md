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

## 仓库发布

仓库只在 push canonical protected tag `vMAJOR.MINOR.PATCH` 时发布正式、非
prerelease Release。push `main` 不会构建或发布 Release。

每个 Release 严格包含两个 Debian package、四个 Terraform provider 包、一个独立 C SDK
源码包及其 checksum sidecar、两个 Flutter SDK hosted pub 包、两个 npm SDK 包、`release-manifest.json` 和
`SHA256SUMS`，加上容器 receipt `container-image.json`，共十五个文件，不发布
Linux raw executable。Debian package 的 `gizclaw_<version>_{amd64,arm64}.deb` 从 tag 取得
`<version>`。平台无关的源码 payload 命名为 `gizclaw-c-sdk-<version>.tar.gz`，相邻的
`.sha256` 保存该源码包的 digest 与规范文件名。Terraform provider 包命名为
`terraform-provider-gizclaw_<version>_{darwin,linux}_{amd64,arm64}.zip`，provider 版本与
Release 版本相同；安装与使用见 [Terraform Provider](/zh/using/terraform)。

Flutter SDK 包命名为 `flutter-gizclaw-<version>.tar.gz` 与
`flutter-gizclaw_control-<version>.tar.gz`，采用 pub hosted archive 布局：archive 根目录直接是
package 根，只包含 `pubspec.yaml`、`LICENSE` 与 Git 跟踪的 `lib/**/*.dart`。
`tools/flutter-sdk/package_archive.sh` 把 `pubspec.yaml` 的 `version` 改写为 Release 版本，
因此每个 tag 对应一个不可变的 hosted package 版本；源码树中的 `version` 只是开发期占位。
`tools/flutter-sdk/verify_archive.sh` 校验成员、元数据与 pubspec identity；
`tools/flutter-sdk/consume_archives.sh` 用本地静态
[Hosted Pub Repository v2](https://github.com/dart-lang/pub/blob/master/doc/repository-spec-v2.md)
布局提供两个包，并让一个 Flutter consumer 以普通 hosted 依赖执行 `flutter pub get` 与
`flutter analyze`。把它们发布到对象存储的 pub 仓库属于 Deploy，不在本仓库 Release contract 内；
使用方式见 [Flutter SDK](/zh/using/sdk/flutter)。

npm SDK 资产命名为 `npm-gizclaw-<version>.tgz` 与
`npm-gizclaw-control-<version>.tgz`，是根为 `package/` 的 npm tarball，包名分别为
`@gizclaw/gizclaw` 与 `@gizclaw/gizclaw-control`。版本从 tag 注入；打包、可复现性与
消费校验见 [TypeScript SDK](./sdk/typescript#发布契约)。Release 是这些包的唯一出货口；
Deploy 从选定的已发布 Release 校验摘要后负责下游托管。本仓库的安装入口为
[Release tarball](/zh/using/sdk/typescript)。

对于正式 Release，Git tag 是唯一 source version：它同时是 Go module version 与
GitHub Release tag；Debian、Terraform provider、C、Flutter 和 npm 包版本移除开头的 `v`。正式 tag 必须是
stable canonical SemVer，不允许数字前导零、prerelease 或 build metadata。
Annotated 与 lightweight tag 都 peel 到完整 source commit，且该 commit 必须已能
从当前受保护的 `main` head 到达。

创建第一个正式 tag 前，repository administrator 必须启用目标为
`refs/tags/v*` 的 tag ruleset：允许创建，但限制更新和删除。发布 workflow 会在
创建正式 Release 前检查受保护 source branch 与 tag ruleset，但不持有或执行仓库
管理权限。

每个 Debian package 只拥有 root/root、mode `0755` 的 `/usr/bin/gizclaw`。
Shared-library dependency 从 packaged ELF 自动推导，并在匹配架构的干净 Ubuntu
24.04 容器中验证安装、执行、删除、重装以及篡改后的同版本重装恢复。

每个 Terraform provider 包由 `build/build-terraform-provider.sh` 在 Linux runner 上以
`CGO_ENABLED=0` 交叉编译，只包含一个 mode `0755` 的 `terraform-provider-gizclaw_v<version>`，
zip 内时间戳取 source commit 时间，重复构建逐字节一致。`build/check-release.sh` 校验每个
zip 的唯一 entry 名称、执行权限，以及 Mach-O/ELF header 与声明的平台和架构一致。

Windows 产物、macOS 上的 `gizclaw` CLI 以及 package-manager repository 发布不属于本仓库的
Release contract。

下载 Release 后，应同时验证 checksum、manifest 与 source identity，不能只信任
文件名：

```sh
tag=v1.2.3
gh release download "$tag" --repo GizClaw/gizclaw --dir ".tmp/$tag"
(cd ".tmp/$tag" && sha256sum --check SHA256SUMS)
build/check-release.sh semver ".tmp/$tag" "$tag" "$(git rev-list -n 1 "$tag")"
```

`release-manifest.json` 标识 stable channel，并将每个 payload 的名称、字节数和 SHA-256
绑定到完整 source commit。Native payload 另外绑定平台和架构；C SDK source entry 绑定
module `gizclaw_c_sdk`、版本与 source commit；`dart-package` entry 绑定 pub package 名称、
版本与 source commit；`npm-package` entry 绑定完整 scoped npm 包名、版本与 source commit；Debian entry 还绑定 package metadata
与 `/usr/bin/gizclaw`；`terraform-provider` entry 还绑定 provider `gizclaw`、版本与 zip 内的
可执行文件名。Formal rerun 只有在现有 published Release 的 metadata 与
全部十五个下载文件逐字节一致时才是 idempotent success。首次上传失败留下的 exact-tag
draft 也必须通过相同的 metadata、inventory、digest 与逐字节校验，workflow 才会发布
同一个 draft。Partial、tag moved、重复 exact-tag Release 或任何 mismatch 都会 fail
closed；workflow 从不删除、替换或覆盖已发布的 SemVer Release。下游 Homebrew 与 APT
channel 各自负责签名、托管、保留策略和 live installation acceptance。

`release-manifest.json` 使用 `schema_version: 7`，`assets` 必须包含 12 个 payload，
按名称以 `LC_ALL=C` 排序。加上 C SDK `.sha256` sidecar、manifest 与 `SHA256SUMS`，
Release 文件总数为 15。消费者必须显式校验 schema 7 和完整资产集合；不同 schema 的资产集合
不可混用。npm entry 只有以下字段：

| 字段 | 类型与约束 |
| --- | --- |
| `name` | 字符串，`npm-gizclaw-<version>.tgz` 或 `npm-gizclaw-control-<version>.tgz` |
| `kind` | 固定字符串 `npm-package` |
| `size` | 正整数，tgz 的字节数 |
| `sha256` | 64 位小写十六进制，整个 tgz 的 SHA-256 |
| `package` | `@gizclaw/gizclaw` 或 `@gizclaw/gizclaw-control`，必须与文件名对应 |
| `version` | tag 去掉 `v` 后的 canonical `MAJOR.MINOR.PATCH` |
| `source_commit` | 40 位小写 Git SHA，与顶层 source commit 相同 |

构建与校验 manifest 时从 tarball 的 `package/package.json` 读取 `name` 和 `version`
交叉校验。npm entry 不含 `os`、`architecture`、`module`、`installed_path`、`provider`
或 `executable`。

### GHCR 运行镜像

正式 tag 的 Linux jobs 从同一份通过 Debian 校验的 package 构建
`build/Dockerfile.runtime`，不再次编译可执行程序。固定 Ubuntu 24.04 base index，
安装 package 声明的 shared libraries、系统 CA 与 curl；镜像以 UID/GID 10001 运行。
Mem0、LiveKit 与 PostgreSQL 由外部服务提供。启动、挂载与 Compose 示例见
[容器镜像](/zh/using/container)。

两个原生 runner 都运行 `build/check-runtime-image.sh`，校验 CLI、`ldd`、CA、
read-only 配置、SQLite/filesystem 持久化、`/server-info` 构建身份、健康检查、重启和
SIGTERM 退出。通过后上传完整 image archive；GHCR publisher 只推送这些已验证的
镜像，不重建二进制或运行层。

`ghcr.io/gizclaw/gizclaw:vMAJOR.MINOR.PATCH` 是两架构 index，仅含 `linux/amd64` 与
`linux/arm64`。不发布浮动 `latest`。`build-<tag>-<source_commit>-<arch>` 是组装 index 的
内部 staging tag。Publisher 只有 `contents: read` 与 `packages: write`，使用官方
`docker/login-action` 和当前仓库的 `GITHUB_TOKEN`。已存在的版本或 staging tag 必须
具有一致的 OCI source/version/base 与 Debian 二进制 SHA-256，否则失败；不会覆盖。
同一版本 rerun 复用现有 digest，运行层的软件包更新通过新正式版本发布。

`container-image.json` 作为 `kind: container-image` payload 进入 schema 7 manifest 与
`SHA256SUMS`。Receipt schema 1 记录 image、tag、version、source_commit、base_image、
index digest、可直接用于 Compose 的 `reference`，以及两个平台的 digest 和
binary_sha256。`build/check-container-receipt.sh` 从 `.deb` 内部重新计算 binary digest。
校验旧 Release 时必须使用该 tag 对应的脚本，不能以 schema 7 校验其他 schema。

GHCR Package 使用 public 可见性，允许 Deploy 无 credential 拉取。首次发布后若 GitHub
仍将包设为 private，管理员在 Package settings 改为 public 并 rerun failed jobs；不扩张
Actions token 权限。两个原生 `container-pull` jobs 使用空 Docker credential config，从
实际 index digest 匿名拉取并重新运行完整容器 gate；成功后才发布正式 GitHub Release。
若包已上传而后续步骤失败，保留现有 tag/digest，再修复或 rerun，不删除或替换资产。
普通 PR/main CI 执行两个原生架构的构建与运行验证，没有 Packages 写权限。

## Mutex scope inventory

`go run ./tools/quality mutexscope` 扫描所有 Git 跟踪的手写 Go 文件和维护 module，也包括当前主机未激活但已跟踪的平台文件。它为可达的 `Lock`/`RLock` 临界区、release 形式、嵌套/风险操作与返回 unlock 的 ownership transfer 建立 fingerprint，并与 `tools/quality/mutexscope.reviewed.jsonl` 精确比较。缺失、stale、重复、未排序、通配或非 intentional 的记录都会 fail closed。

只有逐项审查变更并完成确定性并发测试与 race 测试后才能使用 `-write-reviewed`。该命令是 source inventory，不证明不存在 deadlock、starvation、race 或性能问题。
