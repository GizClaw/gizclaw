# C SDK

GizClaw publishes its C SDK as a deterministic source archive attached to each canonical `vMAJOR.MINOR.PATCH` GitHub Release. The SDK does not have an independent runtime version: archive version `X.Y.Z` identifies the same source commit as repository tag `vX.Y.Z`.

## Download and verify

Download both the archive and its sidecar before adding it to a build:

```sh
version=1.2.3
base="https://github.com/GizClaw/gizclaw/releases/download/v${version}"
curl --fail --location --remote-name "$base/gizclaw-c-sdk-${version}.tar.gz"
curl --fail --location --remote-name "$base/gizclaw-c-sdk-${version}.tar.gz.sha256"
sha256sum --check "gizclaw-c-sdk-${version}.tar.gz.sha256"
```

The archive has one `gizclaw-c-sdk-X.Y.Z/` root and contains `MODULE.bazel`, `BUILD.bazel`, the public and generated C surface, the exact packaged nanopb runtime, licenses, smoke fixtures, and `SOURCE_PROVENANCE.json`. Provenance binds the version, GizClaw source commit and epoch, and nanopb gitlink commit.

## Bzlmod consumption

Until the module is registered in Bazel Central Registry, the root consumer declares the version and overrides it with the verified Release archive:

```starlark
bazel_dep(name = "gizclaw_c_sdk", version = "1.2.3")

archive_override(
    module_name = "gizclaw_c_sdk",
    urls = [
        "https://github.com/GizClaw/gizclaw/releases/download/v1.2.3/gizclaw-c-sdk-1.2.3.tar.gz",
    ],
    integrity = "sha256-<base64 SHA-256 of the verified archive>",
    strip_prefix = "gizclaw-c-sdk-1.2.3",
)
```

Keep the URL, module version, `strip_prefix`, and integrity value on the same immutable release. Convert the verified hex digest to the Subresource Integrity value required by Bazel, or obtain it from an internal dependency update tool; never omit archive integrity.

The module exports:

- `@gizclaw_c_sdk//:gizclaw_core`: portable SDK and packaged nanopb runtime without `src/gzc_platform.c`.
- `@gizclaw_c_sdk//:default_platform`: the libc/POSIX implementation of `gzc_default_platform()`.
- `@gizclaw_c_sdk//:gizclaw`: desktop composition of the two targets.

Firmware uses `gizclaw_core` and links its PAL-owned implementation of the existing `gzc_default_platform()` function. That implementation returns the firmware `gzc_platform_t` with allocator, clock, entropy, and logging callbacks; the firmware also supplies its HTTP, crypto, and WebRTC vtables. Desktop consumers can depend on `gizclaw` for the existing nullable-platform fallback.

The archive does not own a firmware toolchain, final link, image packaging, flashing, credentials, or provider configuration. Consumers must not patch the extracted SDK or fetch another nanopb copy; upgrade to a release containing the required source fix instead.
