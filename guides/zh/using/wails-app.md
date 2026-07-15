# Wails App

GizClaw Desktop 首页以卡片显示所有 Pod。点击卡片会打开该 Pod 的详情面板，
首页没有侧边栏或页面导航。

## Pod 类型

- 本地 Pod：桌面版维护一个本地 Server，端口在创建后保持稳定；Server 对
  LAN 监听，Admin 和 Play 仍从本机连接。详情页可启动、停止和重启 Server。
- 远程 Pod：配置多个 Server 和一个 Access Point。Admin 按 Server 使用各自
  identity；Play 使用 Pod 级 Client identity 连接 Access Point。

Admin 或 Play 未配置 identity 时，对应位置显示配置操作；配置完成后，点击会
在系统浏览器打开页面，而不会在 Wails 窗口内嵌业务 UI。Pod 的操作菜单可编辑
声明式配置、在系统文件管理器中显示目录，或确认后删除 Pod。

远程 Pod 的 Server 列表支持按 ID、名称和 Endpoint 搜索，也可按 Admin
是否配置及连接状态筛选。列表采用有界滚动和虚拟化，Server 数量较大时不会
展开到首页卡片或系统托盘。

## 健康状态

打开窗口、打开 Pod 详情或手动刷新时，桌面版会访问目标的 `/server-info`，
显示检测中、可达、不可达或响应无效。窗口隐藏时不会持续轮询。

无法解析的 `pod.json` 会作为“配置无效”的可恢复卡片保留在首页；单个坏 Pod
不会阻止其他 Pod 启动。可从详情打开其目录修复原始 manifest。

## 系统托盘

关闭桌面窗口会隐藏窗口而不是退出。系统托盘提供：

- Open Window；
- 每个 Pod 的 Open Pod…；
- Quit。

Server、Admin、Play 和密钥操作统一在桌面窗口完成，不放入托盘菜单。
