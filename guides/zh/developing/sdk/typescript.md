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

`serveGiznetWebRTCRPC(pc, handlers)` 应答 Server 发起的设备 RPC。`GizClawPeerRPCHandlers` 安装 `mhs/v0` 状态 handler 和预定义的 `ClientTool` 过程。`client.rpc.methods.list` 声明支持的协议族，`client.tool.v0.list` 只列出实际安装的过程。未安装的工具应答 `METHOD_NOT_FOUND`，Server 映射为 `501 DEVICE_UNSUPPORTED`。handler 可抛出 `GizClawDeviceControlError` 指定 RPC 状态。`peerRPCHandlers` connect option 会在 signaling 前安装 handler。

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

两个 package 只通过 `.github/workflows/release.yml` 的 `js-sdk` job 在 canonical
`vMAJOR.MINOR.PATCH` tag 发布时出货。`.github/workflows/js-sdk-release.yml` 在 PR 与
push `main` 时仅验证开发期 manifest、运行 SDK 测试和 tarball 契约测试。

`sdk/js/scripts/check-package-release.mjs` 中的 `DEVELOPMENT_VERSION` 是开发占位版本的
单一事实源，值为 `0.0.0`。默认模式检查选定 package 的包名、占位版本及
`package-lock.json` 对应 workspace 版本，并拒绝指向旧 npm 托管服务的发布配置。
源码中的 control → gizclaw 与 console → control 依赖用 `"*"` 匹配本地 workspace。
SDK 内容变化不需要手工递增 package 版本。

`tools/js-sdk/package_npm_tarball.sh` 校验 HEAD、source commit、source epoch 与干净的
owned paths，构建 `dist`，再使用 `npm pack` 选择文件。它只在临时 manifest 中注入
Release 版本，将 control 的 `dependencies["@gizclaw/gizclaw"]` 改为精确相同版本，并移除
`publishConfig`。`check-package-release.mjs --package sdk/js/<name> --release-version <version> --manifest <path>` 检查注入后的身份和精确内部依赖，不要求 tarball 携带 workspace
lockfile。打包与 tarball 校验都复用此模式。

最终 tarball 保留 npm 的条目集合，根为 `package/`；条目按路径排序，USTAR 格式，
uid/gid 为 0、uname/gname 为 root，文件 0644、目录 0755，tar 与 gzip 的 mtime
统一为 source epoch。`verify_npm_tarball.sh` 检查结构、归一化元数据与所有公开入口的
JS/类型声明；`consume_tarballs.sh` 在临时项目中安装两个本地 tgz 并 import 全部入口；
`archive_test.sh` 连打两次并用 `cmp` 检查字节一致，同时覆盖错误资产的拒绝路径。

`prepare-published-sdk.mjs <package>` 在 `tsc` 之后复制生成的 Protobuf JavaScript
（如存在）并把 `.d.ts` 里的 `.ts` import 改写为 `.js`。资产命名、manifest schema 与
下游分发边界见[仓库发布](../tooling#仓库发布)；安装方式见
[TypeScript SDK](/zh/using/sdk/typescript)。

## Monitor 客户端

`@gizclaw/gizclaw-control` 导出 `createGizClawPeerMonitorClient`、
`createGizClawNodeMonitorClient` 和 `createGizClawDiscoveryClient`。
Peer 监控使用 `Authorization: Bearer gizclaw_pk_<public key>`，由所属 Server
执行 runtime 的 readonly/fullcontrol/off 权限。Node 监控使用独立 Monitor Token。
公开 SN/IMEI 查询不发送凭证，并返回全部匹配设备。

Peer client 提供设备快照、Telemetry、`listWorkspaces`、
`listWorkspaceHistory`、`searchLogs` 和 `downloadHistoryAudio`。
API Key 用户可以通过 `createGizClawControlClient(...).device` 使用同样的读取方法。`listWorkspaces` 接受可选的 `collection` 与 `workflow_name` 过滤，`deleteWorkspace` 在 Server 返回 `202` 后 resolve（公钥 bearer 需要 fullcontrol）。
客户端接受 AbortSignal；HTTP 错误由 `GizClawControlError` 保留状态码和错误代码。
Ogg 下载返回 Blob。

Node Monitor 生成代码归 `gizclaw-control/generated/monitor` 所有。
Peer wire contract 仍只在 `gizclaw/generated/peerhttp` 生成一次；
网页通过 control SDK 调用，不直接导入生成 client。
