# Wails App

`apps/wails` 是基于 Pod 管理本地和远程 GizClaw Server 的桌面控制面。Wails
窗口只负责环境管理、Server 生命周期和原生桌面集成；Admin UI 与 Play UI
作为独立浏览器应用，通过本机 HTTP 端口提供。

## 模块边界

```text
apps/wails/
├── resources/              # 内嵌的新建本地 Server bootstrap catalog 与 assets
├── internal/
│   ├── appconfig/       # pod.json、目录投影和权限
│   ├── bridge/          # Pod 密钥只写；bootstrap.env 可由受信任 Renderer 编辑
│   ├── endpointhealth/  # /server-info 健康探测
│   ├── localserver/     # 本地 Server 生命周期和有界日志
│   ├── tray/            # 系统托盘适配
│   └── webui/           # loopback HTTP 与本地 runtime token
├── i18n/locales/        # en、zh-CN 文案
└── frontend/            # Pod 桌面首页及 Admin/Play 浏览器入口
```

Desktop App 不复制 `pkgs/gizclaw` 的服务端业务。`api/http/desktop.json` 是
桌面 bridge DTO 的 schema source；更新后通过 `sdk/js` 的 `gen:sdk` 生成
`frontend/src/generated/desktopservice`。

## 本地 Server Bootstrap

`resources/local-server` 是新建本地 Server 的版本化只读 bootstrap 数据源。资源
内容来自 deploy，随 Desktop binary 使用 `go:embed` 编译，不在运行时访问 deploy、
Flowcraft、测试 fixture、网络 catalog 或 AI 服务。Catalog 包含 Credential、Tenant、
Model、Workflow、PetDef、唯一的 `RuntimeProfile/default` 与 PetDef PIXA 映射，共 43 个
声明式资源；不包含 Firmware 或
Workspace，Workspace 仍由客户端创建。Workflow 的名称和图标由客户端按 RuntimeProfile
alias 本地映射，不作为 bootstrap asset。

Desktop 配置根目录中的 `bootstrap.env` 以 `0600` 保存未来本地 Pod 创建所需的
dotenv 值。为了支持表单和原始文本两种编辑方式，bridge 会把文件的完整 `content`
以及每个已保存变量的 `value` 返回给受信任的 Desktop Renderer；因此 Desktop
WebView 是 provider credential 的安全边界之一。只来自 process environment 或资源
default 的值不会回传，前端只会看到对应变量已 configured 或 defaulted。

这些值不会写入 `pod.json`、生成的 Server workspace、URL、Web Storage 或日志，
远程 Pod 的创建和更新也不会读取它们。Desktop 保存值优先于 process environment，
资源中的 `${NAME:-default}` 最后生效。

如果用户在 Desktop 外手动写坏 `bootstrap.env`，Bootstrap 仍返回 Pod 列表、原始 dotenv
内容和解析错误。环境编辑器自动进入文本模式以便修复；在内容重新通过解析前，本地
Pod 创建保持禁用。

本地 `CreatePod` 在保留目录前完成环境 preflight，同步生成 manifest 和投影并写入
`.initializing` 状态后立即返回。可取消的后台任务随后启动 companion、等待 Admin
readiness、按顺序 apply 内嵌资源、同步 Volc Voice，并通过 owner API 上传 PetDef assets。
最后创建只映射到 `RuntimeProfile/default` 的
`RegistrationToken/app:com.gizclaw.opensource`，
将 raw token 以 `0600` 仅写入 Pod 的私有 workspace。Bridge 在初始化期间拒绝 update、start、stop、
restart、Admin 和 Play 操作；delete 会先取消并等待后台任务。

`.initializing` 是 `0600` 的持久化 JSON 状态：`initializing` 会出现在 Pod 列表和
详情中，成功后删除；失败时停止进程并原子改写为带脱敏错误的 `failed`，由用户查看
目录或删除。启动 Desktop 时只清理被退出或崩溃中断的 `initializing` 目录，保留
`failed` Pod。状态清除后的 Pod 不会在普通 start、restart 或 Desktop upgrade 时重放
完整 catalog。旧版 local Pod 在 Server ready 后只执行一次 runtime contract 迁移：apply
`RuntimeProfile/default`、创建新的
`RegistrationToken/app:com.gizclaw.opensource`，并把 catalog version 记录到
`pod.json`；其他可能已被用户修改的资源保持不变。

本地 Play 打开时，Bridge 通过每次 launch 独立保护的 Browser Runtime handoff 传递 raw RegistrationToken；
Play 在同一条持久 WebRTC 连接上先调用 `server.register`，再加载 RuntimeProfile 资源。
本地 Pod 分享二维码通过既有 `registration_token` 字段携带 raw token，GizClaw App 扫码后
即可完成注册。RegistrationToken 不进入 URL、`pod.json`、Web Storage 或日志。远程 Pod 不由 Desktop
生成 RegistrationToken。

每个运行中的本地 Server 在自己的 `workspace/server.pid` 保存 PID，文件以 `0600`
原子写入。正常停止、退出或 Desktop 的 Quit 会清除该文件；Desktop 异常退出时
Server 与 PID 文件都保留。下次启动会扫描有效的本地 Pod，验证 PID 文件为普通文件
且进程仍存活，并要求该 Pod 的 loopback `/server-info` 公钥与 workspace identity 一致，
然后才恢复进程管理。`/server-info` 会在 5 秒内有界重试；超时等瞬时验证失败会保留
PID，明确的 identity mismatch 才会清除 PID。未验证的 PID 不会被 signal，从而避免
PID 被系统复用后误杀其他进程；验证通过的 Server 不会因为占用既有端口而被重新启动或终止。
失效 PID 会在恢复时清除，恢复的非子进程通过存活探测更新生命周期状态。若崩溃发生
在 bootstrap 尚未完成时，Desktop 会先接管并停止该 Server，再按现有约定清理不完整
Pod；若身份验证仍未完成，则保留 PID 与 workspace 并中止清理，避免留下无法管理的进程。
普通本地 Pod 的瞬时恢复错误会显示为 failed process/health 状态而不阻止 Desktop 启动；
delete、stop、restart、start 和 update 会先重试验证，PID 仍未验证时拒绝操作，明确的
identity mismatch 则清除 stale PID 并按 stopped 处理。

## Pod 投影

`pod.json` 是唯一可编辑的配置来源。每次保存后，`appconfig.Store` 原子更新：

- local Pod 的 `workspace/config.yaml`，其中监听地址是 `0.0.0.0:<port>`，
  Server endpoint 使用当前可用的 LAN 地址，对本机 Context 公布的仍是
  `127.0.0.1:<port>`；LAN 地址不写入 `pod.json`；
- 每个配置了 Admin identity 的 Server 对应一个
  `admin_context/<server-id>/config.yaml`；
- 配置了 Client identity 时生成 Pod 级 `client_context/config.yaml`；
- remote Pod 不创建 `workspace/`，也不提供进程控制。

Pod ID 与新增 remote Server ID 由 bridge 生成，只用于目录和稳定引用，不作为
桌面创建表单字段。remote Pod 可以先只保存 Access Point，随后从详情添加
Server；投影逻辑必须支持空的 `remote_servers`。

Local Server 的完整 workspace 默认值由 binary 内嵌的
`internal/appconfig/templates/local_server_workspace.yaml.gotmpl` 拥有；运行时不读取
source tree。Renderer 保留已生成的 Server identity，更新 listen、LAN endpoint、Admin
key 和 store inventory，并以 `0600` 原子写入。模板显式使用 info-level stderr
`system_log`，不创建 LogStore、Volc credential、store sink 或 `query_store`；需要持久化和
查询日志时由用户显式配置。

目录和密钥文件必须保持私有权限。写入采用同目录临时文件、同步、rename 的
原子替换流程。前端响应只能包含 `admin_configured`、`play_configured` 等状态，
不能返回持久化密钥。

## 浏览器 Runtime

Admin 与 Play 的静态产物分别从 `admin.html` 和 `play.html` 启动。每个 Pod/surface
只保留一个 `127.0.0.1:0` listener，每次打开生成独立随机 token，并将该 token 与
本次选择的 Runtime 绑定。token 保留在本地 URL query 中，浏览器以同源 POST 领取
Runtime；同一 URL 页面刷新时继续使用自己的 token，直到对应 listener 关闭。交接
结果禁止缓存；
Runtime 私钥不得进入 URL、Web Storage、日志或静态文件。

Go 部分遵循 [Go 编码规范](/zh/coding-styles/go)，frontend 遵循
[JavaScript 与 TypeScript](/zh/coding-styles/js)。

## 打包边界

macOS 分发包通过 `apps/wails/scripts/package-darwin.sh` 构建。脚本先生成 Wails
应用，再把仓库现有的 `cmd/gizclaw` 编译为
`Contents/Resources/gizclaw` companion。桌面进程优先从应用资源目录解析该
程序；环境变量和 `PATH` 只用于开发和测试，不是分发包的运行前提。
