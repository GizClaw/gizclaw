# Wails App

`apps/wails` 是基于 Pod 管理本地和远程 GizClaw Server 的桌面控制面。Wails
窗口只负责环境管理、Server 生命周期和原生桌面集成；Admin UI 与 Play UI
作为独立浏览器应用，通过本机 HTTP 端口提供。

## 模块边界

```text
apps/wails/
├── internal/
│   ├── appconfig/       # pod.json、目录投影和权限
│   ├── bridge/          # 不返回密钥的 Wails capability
│   ├── endpointhealth/  # /server-info 健康探测
│   ├── localserver/     # 本地 Server 生命周期和有界日志
│   ├── tray/            # 系统托盘适配
│   └── webui/           # loopback HTTP 与一次性交接
├── i18n/locales/        # en、zh-CN 文案
└── frontend/            # Pod 桌面首页及 Admin/Play 浏览器入口
```

Desktop App 不复制 `pkgs/gizclaw` 的服务端业务。`api/http/desktop.json` 是
桌面 bridge DTO 的 schema source；更新后通过 `sdk/js` 的 `gen:sdk` 生成
`frontend/src/generated/desktopservice`。

## Pod 投影

`pod.json` 是唯一可编辑的配置来源。每次保存后，`appconfig.Store` 原子更新：

- local Pod 的 `workspace/config.yaml`，其中监听地址是 `0.0.0.0:<port>`，
  对本机 Context 公布的是 `127.0.0.1:<port>`；
- 每个配置了 Admin identity 的 Server 对应一个
  `admin_context/<server-id>/config.yaml`；
- 配置了 Client identity 时生成 Pod 级 `client_context/config.yaml`；
- remote Pod 不创建 `workspace/`，也不提供进程控制。

目录和密钥文件必须保持私有权限。写入采用同目录临时文件、同步、rename 的
原子替换流程。前端响应只能包含 `admin_configured`、`play_configured` 等状态，
不能返回持久化密钥。

## 浏览器 Runtime

Admin 与 Play 的静态产物分别从 `admin.html` 和 `play.html` 启动。每个
Pod/surface 只保留一个 `127.0.0.1:0` listener。每次打开浏览器都生成新的
随机 token，浏览器以同源 POST 一次性领取 Runtime 后立即从地址栏移除 token。
交接结果禁止缓存；密钥不得进入 URL、Web Storage、日志或静态文件。

Go 部分遵循 [Go 编码规范](/zh/coding-styles/go)，frontend 遵循
[JavaScript 与 TypeScript](/zh/coding-styles/js)。

## 打包边界

macOS 分发包通过 `apps/wails/scripts/package-darwin.sh` 构建。脚本先生成 Wails
应用，再把仓库现有的 `cmd/gizclaw` 编译为
`Contents/Resources/gizclaw` companion。桌面进程优先从应用资源目录解析该
程序；环境变量和 `PATH` 只用于开发和测试，不是分发包的运行前提。
