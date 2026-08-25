# C SDK

GizClaw 会在每个规范 `vMAJOR.MINOR.PATCH` GitHub Release 中附带一个可复现的 C SDK 源码包。C SDK 没有独立的 runtime version：源码包版本 `X.Y.Z` 与仓库 tag `vX.Y.Z` 指向同一个 source commit。

## 下载与校验

接入构建前同时下载源码包及其 sidecar：

```sh
version=1.2.3
base="https://github.com/GizClaw/gizclaw/releases/download/v${version}"
curl --fail --location --remote-name "$base/gizclaw-c-sdk-${version}.tar.gz"
curl --fail --location --remote-name "$base/gizclaw-c-sdk-${version}.tar.gz.sha256"
sha256sum --check "gizclaw-c-sdk-${version}.tar.gz.sha256"
```

源码包只有一个 `gizclaw-c-sdk-X.Y.Z/` 根目录，包含 `MODULE.bazel`、`BUILD.bazel`、C public/generated surface、精确的 nanopb runtime、license、smoke fixture 和 `SOURCE_PROVENANCE.json`。Provenance 将版本、GizClaw source commit/epoch 与 nanopb gitlink commit 绑定在一起。

## Bzlmod 消费

模块进入 Bazel Central Registry 前，根 consumer 声明版本，并用已经校验的 Release 源码包覆盖来源：

```starlark
bazel_dep(name = "gizclaw_c_sdk", version = "1.2.3")

archive_override(
    module_name = "gizclaw_c_sdk",
    urls = [
        "https://github.com/GizClaw/gizclaw/releases/download/v1.2.3/gizclaw-c-sdk-1.2.3.tar.gz",
    ],
    integrity = "sha256-<已校验源码包 SHA-256 的 base64 值>",
    strip_prefix = "gizclaw-c-sdk-1.2.3",
)
```

URL、module version、`strip_prefix` 与 integrity 必须来自同一个不可变 Release。将已校验的十六进制 digest 转换为 Bazel 要求的 Subresource Integrity 值，或由内部依赖更新工具生成；不能省略 archive integrity。

模块导出：

- `@gizclaw_c_sdk//:gizclaw_core`：portable SDK 与内置 nanopb runtime，不包含 `src/gzc_platform.c`。
- `@gizclaw_c_sdk//:default_platform`：libc/POSIX `gzc_default_platform()` 实现。
- `@gizclaw_c_sdk//:gizclaw`：组合以上两个 target 的 desktop 入口。

Firmware 使用 `gizclaw_core`，并链接 PAL 拥有的现有 `gzc_default_platform()` 实现。该实现返回带 allocator、clock、entropy 与 logging callback 的 firmware `gzc_platform_t`；firmware 仍负责 HTTP、crypto 和 WebRTC vtable。Desktop consumer 可以依赖 `gizclaw`，继续使用现有 nullable-platform fallback。

源码包不拥有 firmware toolchain、最终链接、image packaging、烧录、credential 或 provider 配置。Consumer 不能 patch 解压后的 SDK 或另取一份 nanopb；需要 portability 修复时应升级到包含修复的 GizClaw Release。
