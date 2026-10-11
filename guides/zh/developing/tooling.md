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
`SHA256SUMS`，加上容器交付清单 `container-image.json` 与四个镜像压缩包，共十九个文件，不发布
Linux raw executable。Debian package 的 `gizclaw_<version>_{amd64,arm64}.deb` 从 tag 取得
`<version>`。平台无关的源码 payload 命名为 `gizclaw-c-sdk-<version>.tar.gz`，相邻的
`.sha256` 保存该源码包的 digest 与规范文件名。Terraform provider 包命名为
`terraform-provider-gizclaw_<version>_{darwin,linux}_{amd64,arm64}.zip`，provider 版本与
Release 版本相同；安装与使用见 [Terraform Provider](/zh/using/terraform)。

Runtime 与 Mem0 各交付 `linux/amd64`、`linux/arm64` 两个独立的 Docker save gzip
镜像包；四个包均为正式 Release assets，必须生成并验证成功后才能发布 Release。
Mem0 使用同一 source commit，OCI label 记录 Release version/revision；构建文件为
`build/mem0/Dockerfile`，Python 入口为 `cmd/mem0/gizclaw_mem0`。原生 runner 先执行
`test` target，再导出并加载 `runtime` 镜像，校验版本、CLI 与非 root Python 服务入口。
使用方法见[自托管 Mem0 服务](./stores/memory#自托管-mem0-服务)。

`build/build-mem0.sh [cn]` 选择 `build/mem0/Dockerfile.base` 或
`Dockerfile.cn.base`，然后构建统一的应用 Dockerfile。两个 base 使用相同固定 Python
image digest 和 SDK dependency 版本；标准源为 PyPI，CN 源为清华 PyPI mirror。
`PYTHON_BASE_IMAGE` 可覆盖 Python 镜像仓库，CN 的 `PIP_INDEX_URL` 可覆盖包源。
`PLATFORM` 选择 `linux/amd64|linux/arm64`；`TARGET=base|test|runtime`，
`IMAGE`、`BUILD_VERSION` 与 `BUILD_COMMIT` 控制应用镜像及身份。Release 使用标准
base；CN 是构建源选择，不改变模型、API 或镜像运行行为。

Go 程序和基础镜像的 Dockerfile 位于 `build/gizclaw/`；
`build/build-linux.sh` 选择标准或 CN 基础镜像并导出 `gizclaw` executable。

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
全部十九个下载文件逐字节一致时才是 idempotent success。首次上传失败留下的 exact-tag
draft 也必须通过相同的 metadata、inventory、digest 与逐字节校验，workflow 才会发布
同一个 draft。Partial、tag moved、重复 exact-tag Release 或任何 mismatch 都会 fail
closed；workflow 从不删除、替换或覆盖已发布的 SemVer Release。下游 Homebrew 与 APT
channel 各自负责签名、托管、保留策略和 live installation acceptance。

`release-manifest.json` 使用 `schema_version: 8`，`assets` 必须包含 16 个 payload，
按名称以 `LC_ALL=C` 排序。加上 C SDK `.sha256` sidecar、manifest 与 `SHA256SUMS`，
Release 文件总数为 19。消费者必须显式校验 schema 8 和完整资产集合；不同 schema 的资产集合
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

### 容器镜像归档

Runtime 从同一份通过 Debian 校验的 package 构建 `build/gizclaw/Dockerfile.runtime`，
不再次编译程序。固定 Ubuntu 24.04 base index，安装 package 声明的 shared libraries、
系统 CA 与 curl；以 UID/GID 10001 运行。启动、挂载与 Compose 示例见
[容器镜像](/zh/using/container)。

镜像包命名为 `gizclaw-runtime_<version>_linux_<arch>.tar.gz` 与
`gizclaw-mem0_<version>_linux_<arch>.tar.gz`，其中 `<version>` 为 tag 去掉 `v`，
`<arch>` 为 `amd64` 或 `arm64`。`build/package-container-image.sh` 以 `docker save`
和 `gzip -n` 导出，不重建运行层；可直接使用 `docker load --input <archive>`。
加载后仅恢复一个架构的引用 `gizclaw-runtime:<tag>-<arch>` 或
`gizclaw-mem0:<tag>-<arch>`；没有 multiarch registry tag 或浮动 `latest`。

`container-image.json` 保留 `kind: container-image`，进入 manifest 与 `SHA256SUMS`。
该清单使用 schema 2，顶层字段为 `schema_version`、`tag`、`version`、`source_commit`
和 `images`。`images` 严格按 component（`mem0`、`runtime`）及 architecture
（`amd64`、`arm64`）排序，包含四条记录；这些记录与 schema 8 manifest 的四条
`kind: container-archive` entry 完全相同：

| 字段 | 类型与约束 |
| --- | --- |
| `name` | 上述规范镜像包文件名 |
| `kind` / `component` | `container-archive` / `runtime` 或 `mem0` |
| `os` / `architecture` | `linux` / `amd64` 或 `arm64` |
| `format` | 固定 `docker-save+gzip` |
| `version` / `source_commit` | tag 去掉 `v` / 同一完整 40 位 Git SHA |
| `size` | 压缩文件字节数，正整数 |
| `sha256` | 压缩文件 SHA256，64 位小写十六进制 |
| `image_reference` | 加载后恢复的本地架构 tag |
| `image_id` | `sha256:<64 hex>`，构建端 Docker 实际观测的 `.Id` |
| `config_digest` | `sha256:<64 hex>`，归档内 Docker image config 的 SHA256 |
| `archive_manifest_digest` | 仅 OCI/containerd 布局存在，归档内可运行平台 manifest 的 SHA256 |
| `archive_index_digest` | 归档有 OCI image index 时存在，该 index blob 的 SHA256 |
| `binary_sha256` | 仅 Runtime 存在，镜像与同架构 `.deb` 中 `/usr/bin/gizclaw` 的 SHA256 |

归档 `sha256`、本地 `image_id` 与 registry manifest/index digest 是不同身份，不能互换。
清单不提供 registry `digest`、`reference` 或 `platforms` 字段。Deploy 消费者应校验
schema 8、全部 19 个资产及 source identity，再从 schema 2 的 `images` 选择 component
和架构，校验文件大小与 SHA256 后 `docker load`，核对恢复的引用、image ID、平台及
OCI version/revision，随后按 image ID 运行。传统 Docker 的 `.Id` 对应 `config_digest`，
containerd 存储可能对应 `archive_manifest_digest` 或 `archive_index_digest`；构建端 `image_id` 必须属于这三者，
消费端允许匹配任一经归档验证的身份，并固定该端实际加载的 `.Id`。归档内 manifest
身份不代表曾发布到 registry。若 containerd 导入传统归档时复用压缩 layer，可能生成
新的 manifest ID；验收会对该本地镜像执行有大小/成员/时间限制的 `docker save`，
仅在导出的唯一镜像 config SHA256 与归档 `config_digest` 完全一致时接受该新 ID。TOS 托管与服务器端下载部署由 Deploy
负责。本仓库不发布或删除 GHCR package，历史 GHCR 镜像与 Release 保留供回滚。
Debian、C、Flutter、npm 与 Terraform provider entry 的字段和命名保持不变。
旧 schema 的 Release 必须使用其 source tag 对应的验证器。

`build/container_archive.py` 在不提取文件的情况下检查 gzip、Docker save manifest、
唯一 tag、config SHA256、全部解压后的 layer diff IDs、平台、非 root 用户与 OCI 身份；
OCI index 只允许一个可运行的平台镜像，以及通过 descriptor、config/layer 和 in-toto
subject 绑定到该镜像的 attestation；拒绝额外平台镜像或身份冲突。
`build/check-container-receipt.sh` 另从 `.deb` 重算程序 SHA256。
`build/check-container-archive.sh` 校验实际交付字节后导入镜像，并核对加载后的 image ID。
Runtime 完整运行 gate 保留 CLI、`ldd`、CA、只读配置、SQLite/filesystem 持久化、
`/server-info` 身份、健康检查、重启、workspace 独占、SIGTERM 退出与强制停止恢复。
Mem0 保留 test target 和运行入口验证。正式发布前，`container-load` 在两个新的原生
runner 上从归档再次执行这些运行检查；PR/main CI 同样覆盖四个归档的生成、加载与运行。

正式重跑时，`build/reuse-container-archive.sh` 仅从 exact-tag published Release 或完整
同 tag draft 下载原始镜像包，核对 source、manifest、checksum、receipt、归档与 Debian
程序身份后复用原字节，再次执行运行 gate。只有明确不存在该 tag 的 Release 才能新建
镜像包；权限、网络、部分上传、旧 schema 或身份冲突均失败。最终 publisher 仍下载全部
资产并逐字节比较后才接受幂等发布；不会覆盖已有资产。Actions 中的独立 metadata parts
仅用于组装交付清单，不进入正式 Release。

## Mutex scope inventory

`go run ./tools/quality mutexscope` 扫描所有 Git 跟踪的手写 Go 文件和维护 module，也包括当前主机未激活但已跟踪的平台文件。它为可达的 `Lock`/`RLock` 临界区、release 形式、嵌套/风险操作与返回 unlock 的 ownership transfer 建立 fingerprint，并与 `tools/quality/mutexscope.reviewed.jsonl` 精确比较。缺失、stale、重复、未排序、通配或非 intentional 的记录都会 fail closed。

只有逐项审查变更并完成确定性并发测试与 race 测试后才能使用 `-write-reviewed`。该命令是 source inventory，不证明不存在 deadlock、starvation、race 或性能问题。
