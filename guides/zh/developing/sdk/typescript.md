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

`serveGiznetWebRTCRPC(pc, handlers)` 应答 Server 发起的 `client.*` RPC。
`GizClawPeerRPCHandlers` 覆盖 `client.info.get`、`client.identifiers.get`、
`client.device.*` / `client.wifi.*` / `client.firmware.update` 设备控制方法（含 `getSettings`、`setSettings`、
`factoryReset`、`setRunWorkspace`）由已注册 handler 推导的 `client.rpc.methods.get`，以及 `tools`：按 invoke name 注册的
`client_rpc` Tool handler，返回值作为 JSON（`data_json`）回给调用方；未提供的 handler 应答 `METHOD_NOT_FOUND`，
Server 据此返回 `501 DEVICE_UNSUPPORTED`。handler 抛出 `GizClawDeviceControlError`
可指定具体 RPC error code。handlers 也可通过 connect option `peerRPCHandlers` 传入，
在 signaling 之前安装。

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
`js-sdk` job 把同一份 tgz 上传为 Release 构建资产，并通过 `npm publish <tgz>`
依次发布到 GitHub Packages：先 `@gizclaw/gizclaw`，再 `@gizclaw/gizclaw-control`。
发布前分别查询两个包的已有版本，任一版本已存在或查询失败都会终止；不覆盖已发布版本。
该 job 使用 `packages: write` 和 `github.token` 作为 `NODE_AUTH_TOKEN`。
`publish-semver` 依赖该 job 成功，再把这些 tgz 上传到正式 Release。
仓库 Actions variable `JS_SDK_NPM_DIST_TAG` 必须显式指定两个包使用的 npm dist-tag，
未设置会在发布前失败。设为 `latest` 会更新默认安装版本；设为其他 tag（例如 `release`）
会保留原 `latest`，使用方可按该 tag 或精确版本安装。npm 对已有更高版本的包拒绝隐式
使用 `latest`，因此 workflow 始终通过 `--tag` 传入已选策略。

`sdk/js/scripts/check-package-release.mjs` 中的 `DEVELOPMENT_VERSION` 是开发占位版本的
单一事实源，值为 `0.0.0`。默认模式检查选定 package 的包名、占位版本及
`package-lock.json` 对应 workspace 版本，并要求 `publishConfig.registry` 严格等于
`https://npm.pkg.github.com`。
源码中的 control → gizclaw 与 console → control 依赖用 `"*"` 匹配本地 workspace。
SDK 内容变化不需要手工递增 package 版本。

`tools/js-sdk/package_npm_tarball.sh` 校验 HEAD、source commit、source epoch 与干净的
owned paths，构建 `dist`，再使用 `npm pack` 选择文件。它只在临时 manifest 中注入
Release 版本，将 control 的 `dependencies["@gizclaw/gizclaw"]` 改为精确相同版本，并保留
指向 GitHub Packages 的 `publishConfig`。`check-package-release.mjs --package sdk/js/<name> --release-version <version> --manifest <path>` 检查注入后的身份和精确内部依赖，不要求 tarball 携带 workspace
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
