# App

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/app)

App 是安装在设备上的 Lua 程序。Admin 使用 `kind: App`、canonical `metadata.id` 和 `spec.package: {url, sha256, size}`。App 没有 enabled 或 version。通用 Resource apply/get/put/delete 管理服务器目录；删除目录资源不会卸载设备程序。

Apply 下载 HTTPS `.tar.zlib`，校验压缩包的精确大小和小写 SHA-256，再将解析的 manifest 存入 `apps` SQL 表。限制为压缩 16 MiB、解压 64 MiB、1024 个归档条目、app.json 256 KiB、128 个方法。路径必须相对，拒绝空段、`.`、`..`、反斜杠、链接和特殊文件。Lua 文件必须是 UTF-8 文本，拒绝字节码；允许二进制资产。entry 必须指向存在的 `.lua` 文件。

根目录 `app.json` 包含 `app_name`、`runtime`、`entry`、`methods`。App 和方法名匹配 `^[A-Za-z_][A-Za-z0-9_-]{0,63}$`，app_name 不可修改。runtime 是一个受 pattern 约束的不透明 profile ID，例如 `runtime.lua.gizos`，只进行精确相等比较。方法包含 name、mode（call 或 job）、description（最多 1024 字符）和 object input JSON Schema。入口模块返回按方法名索引的函数表，每个函数接收解码后的参数表，返回可 JSON 编码的值。

`RuntimeProfile.spec.resources.apps` 是 alias 到 `{resource_id, i18n}` 的映射。连接后服务器调用 `client.app.list`，为 runtime 精确匹配、requires 是上报 capabilities 子集且 SHA-256 缺失或不同的 profile App 请求安装。协调失败记录日志，不拒绝连接。PeerConn 缓存 runtime、capabilities 和已安装包摘要，并在安装成功后更新缓存；协调完成前或失败时不暴露 App 方法。模型工具集和调用只读取此缓存，不再请求设备列表，仅暴露符合当前 runtime、所需 capabilities 和包摘要的方法，命名为 `<app_name>__<method>`。与服务器 Tool 调用名或其他 App 方法重名时，工具集构造失败。

call 方法走 `client.app.invoke`；job 方法走 `client.app.job.start`，返回 `{job_id}`。调用绑定当前 accepted peer connection，默认 10 秒超时，timeout/unavailable 返回 recoverable JSON error。参数必须是 JSON 对象，编码后最多 4096 字节；结果必须是有效 JSON，具有相同的字节上限。RPC 同时提供卸载与 job 取消，payload 由 `api/proto/rpc/payload/app.proto` 定义。

App 展示名称和 i18n 在部署时由 RuntimeProfileBinding 提供，不属于 App。


## Capability 要求与宿主 Hooks

manifest 可选 `requires` 是最多 64 个不重复的 capability 名称数组，名称匹配 `^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`，例如 `"requires": ["litelink.notify"]`。省略或空数组表示没有 capability 要求；不接受 null。服务器校验并在 App 读取模型中保存该字段。`client.app.list` 返回宿主已注册的全部 capability 名称。任何要求缺失或 runtime 不匹配时，该 App 不安装，也不向模型暴露，包括设备已安装同一 SHA-256 的情况。

Hooks 遵循固件 GizOS `libs/lua` 的 `h2_lua_host` contract。嵌入应用在 `h2_lua_host_start()` 前调用 `h2_lua_register_capability(name, call, cancel)`，启动后注册表冻结。Lua 使用 `capability.call(name, payload, options)`，观察三个返回值 `ok, output, error`。异步 handler 返回 `H2_PAL_ERR_WOULD_BLOCK`，随后通过 `h2_lua_capability_complete` 完成。capability 使用命名空间，例如 `litelink.notify`。

display、button、touch、audio 等 PAL 能力由 board 提供，不按 App 注册。Lua App 使用 `runtime.components.on/off` 和 `runtime.event.*` 注册事件回调。Board 描述 display_width、display_height、buttons（名称到按键的映射）、touch 和 audio 数据；skin 提供 display、controls、status 和 button(name) 展示槽位，runtime 不读取 skin。
