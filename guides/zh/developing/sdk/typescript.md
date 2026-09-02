# TypeScript SDK <Badge type="warning" text="WIP" />

> 本页说明两个 npm package 的目录、contract 边界、生成流程与发布契约；`@gizclaw/gizclaw` 的公开 surface 与 runtime 行为仍待逐项展开。

`sdk/js/` 是一个 npm workspace，包含两个可发布 package 与共享的生成脚本：

- `sdk/js/gizclaw` 是设备端 `@gizclaw/gizclaw`，覆盖 Admin HTTP、Public HTTP、RPC、signaling 和 Telemetry，并拥有全部生成 client。它与 `sdk/c/gizclaw` 覆盖同一侧能力。
- `sdk/js/gizclaw-control` 是控制端 `@gizclaw/gizclaw-control`，把一个 API Key 绑定到生成的 Public HTTP client，暴露 `/gizclaw/v1/*` 的 route group 与错误分类。它依赖 `@gizclaw/gizclaw/peerhttp`，不再生成第二份 contract。
- `sdk/js/scripts` 保存由 OpenAPI、Protobuf 与 method registry 生成 SDK surface、修整生成结果，以及发布检查所需的工具。

```text
sdk/js/
├── gizclaw/                 # @gizclaw/gizclaw：设备端 SDK 与 generated client
│   ├── peerhttp.ts          # Public HTTP 生成 surface；额外导出 createPeerHTTPClient
│   └── generated/           # 只能由 gen:sdk / gen:telemetry 更新
├── gizclaw-control/         # @gizclaw/gizclaw-control：控制端 SDK
│   ├── index.ts             # createGizClawControlClient、GizClawControlError
│   └── index.test.ts        # 注入 fetch stub 的 route 与错误映射测试
└── scripts/                 # Contract generation、生成结果修整、发布检查
```

生成内容的 source of truth 位于 [API Design](../api/overview)，不能直接把 generated output 当作手写实现维护。

## `@gizclaw/gizclaw`

Browser/Desktop 通过 encrypted `/webrtc/v1/offer` signaling 建立连接，并在 ordered
`giznet/v1/service/0` DataChannel 上传输 protobuf RPC envelope、body frames 和 EOS。
`connectGiznetWebRTC` 在 offer 前创建 packet DataChannel 和 Opus-capable audio
transceiver；调用方注入 identity、crypto、fetch 等 runtime-specific primitives。

`createWebRTCFetch` 是 generated client 的 fetch adapter boundary。当前 WebRTC bridge
按 GizClaw RPC method 映射 HTTP request，并不是任意 HTTP proxy。

## `@gizclaw/gizclaw-control`

`createGizClawControlClient` 用 `createPeerHTTPClient` 创建独立的生成 client（`baseUrl`、`auth`、可选 `fetch`），每个 API Key 一个实例，不使用 `peerHTTPClient` 单例。route 方法以 `throwOnError: false` 调用 `sdk.gen.ts` 函数，再把 `{ error, response }` 转成 `GizClawControlError`：`response` 缺失是 `network`，否则先按 body 的 `error.code` 匹配 `DEVICE_*`，再按 status 分类；code 常量以 `pkgs/gizclaw/peer_service_serve_peer_http_device_control.go` 为准。路径参数由生成 client 做 `encodeURIComponent`。

## 生成与验证

```sh
npm ci
npm --prefix sdk/js run gen:sdk
npm --prefix sdk/js test
npm test --workspace @gizclaw/gizclaw-control
npm run quality:typescript
npm run quality:lint
npm run quality:format
```

`gizclaw-control` 的 `pretest` 与 `prebuild` 会先构建 `@gizclaw/gizclaw`，因为它通过 package exports 解析到 `dist/peerhttp.*`。

## 发布契约

两个 package 都由 `.github/workflows/js-sdk-release.yml` 发布到 GitHub Packages。`sdk/js/scripts/check-package-release.mjs --package sdk/js/<name>` 对每个 package 分别验证：

- 发布路径（package 目录内除 `package.json` 版本、`*.test.ts`、`tsconfig.json` 以外的文件，以及 `scripts/prepare-published-sdk.mjs`）有变化时，版本必须高于 base commit；
- 没有发布路径变化时，版本不能改变；
- `package-lock.json` 中的 workspace 版本必须与 manifest 一致。

修改 `@gizclaw/gizclaw` 的公开 surface（例如新增 export）需要提升它的版本；只改 `gizclaw-control` 不会触发 `@gizclaw/gizclaw` 发布。`prepare-published-sdk.mjs <package>` 在 `tsc` 之后复制生成的 Protobuf JavaScript（如存在）并把 `.d.ts` 里的 `.ts` import 改写为 `.js`。
