# Wails App

GizClaw Desktop 的整个窗口就是 Pod 集合：没有网页式标题、说明区、搜索栏、
侧边栏或页面导航。Pod 与添加入口使用同尺寸的小卡片网格；没有 Pod 时，画面
只保留居中的添加卡片。点击 Pod 后，卡片以淡入和缩放动画打开详情面板，关闭
面板时淡出并回到原来的卡片集合。

创建时不填写 Pod ID、端口或密钥。内部 ID 自动生成；本地 Pod 一键创建并自动
选择稳定端口，创建后可以改名。远程 Pod 首次只填写 Access Point，Server 在
Pod 详情中逐个添加，其内部 ID 同样自动生成。桌面版自动生成本机 Play identity；
远程 Server 的 Admin identity 由目标 Server 配置，添加时需要粘贴对应的 Admin
private key。

## Pod 类型

- 本地 Pod：桌面版维护一个本地 Server，端口在创建后保持稳定；Server 对
  LAN 监听，Admin 和 Play 仍从本机连接。正面二维码用于在其他 GizClaw App
  添加该 Server，背面可启动、停止和重启 Server，并打开 Admin 或 Play。
- 远程 Pod：配置零个或多个 Server 和一个 Access Point。Admin 按 Server 使用各自
  identity；Play 使用 Pod 级 Client identity 连接 Access Point。正面二维码分享
  Access Point，背面维护 Server 列表。

本地 Pod 的 Admin 和 Play identity 自动生成。远程 Server 的 Admin private key
来自目标 Server 的既有配置并只保留在本机；未填写时对应 Admin 保持未配置。
Admin 和 Play 点击后在系统浏览器打开，而不会在 Wails 窗口内嵌业务 UI。Pod 的
操作菜单可编辑声明式配置、在系统文件管理器中显示目录，或确认后删除 Pod。

远程 Pod 的 Server 列表支持按 ID、名称和 Endpoint 搜索。列表采用有界滚动和
虚拟化，Server 数量较大时不会
展开到首页卡片或系统托盘。

## 本地 Bootstrap 环境

首页的 Bootstrap 状态入口列出内嵌资源目录需要的环境变量。值只写入 Desktop
配置目录的私有文件。为了回填表单和 dotenv 文本编辑器，受信任的 Desktop
Renderer/WebView 可以读取文件全文和已保存值；只来自启动进程或资源默认值的内容
不会回传，窗口只看到对应变量已配置或正在使用默认值。输入新值会替换保存值，
“清除已保存值”会删除本地覆盖。Desktop 保存值优先于启动进程的同名环境变量。

缺少必填值时仍可管理现有 Pod 或创建远程 Pod，但不能创建本地 Pod。补齐后创建
本地 Pod 会在 manifest 和投影保存后立即回到首页。Pod 卡片显示“正在初始化数据”；
点开后可查看持续更新的初始化状态，也可以关闭详情稍后再看。后台任务会启动新的
Server、apply 内嵌 Credential/Tenant/Model/Workflow/ACL/Gameplay catalog、同步
所需 Voice，并上传 Workflow 与 PetDef assets。全部完成后详情自动切换为正常界面。

初始化失败会停止 Server，并在 Pod 详情中保留脱敏错误、目录入口和删除操作。退出
Desktop 或崩溃时仍在初始化的 Pod 会在下次启动时清理；已经成功创建的 Pod 在
Desktop 或 Server 重启时不会重新 apply，因此用户后续修改和删除的资源会保留。

## 健康状态

打开窗口、打开 Pod 详情或手动刷新时，桌面版会访问目标的 `/server-info`，
显示检测中、可达、不可达或响应无效。窗口隐藏时不会持续轮询。

无法解析的 `pod.json` 会作为“配置无效”的可恢复卡片保留在首页；单个坏 Pod
不会阻止其他 Pod 启动。可从详情打开其目录修复原始 manifest。

## 系统托盘

无边框窗口左上角提供关闭、最小化和最大化/恢复按钮。关闭按钮和 `Cmd+W` 只
隐藏窗口，不停止本地 Server 或浏览器 HTTP listener。系统托盘使用可辨识的
系统图标并提供：

- Open Window；
- 每个 Pod 的 Open Pod…；
- Quit。

Server、Admin、Play 和密钥操作统一在桌面窗口完成，不放入托盘菜单。
只有托盘中的 Quit 才真正退出进程并清理运行资源。

如果 Desktop 进程异常退出，已经运行的本地 Server 会继续工作。Desktop 在每个本地
Pod 的 `workspace/server.pid` 记录其进程；再次启动时会自动恢复管理该进程，因此不需
要先手动结束 Server，也不会因原端口仍被占用而启动失败。正常使用托盘 Quit 或手动
停止 Server 会同时清除对应 PID 文件。

Admin 的 Resource 编辑页为 Workflow、Workspace 与 GameDef 分别提供 PNG/PIXA icon 上传、下载和删除；Peer 详情页使用 Peer 自己的 Admin icon endpoint。界面不会调用通用 Resource icon 或 Asset API。每个格式使用独立 slot，单个文件上限为 2 MiB。

Admin 的 Workflow catalog 按浏览器 locale 选择现有顶层 `i18n` 文案，并在没有
匹配 locale 时使用 `default_locale`。名称缺失时显示稳定 Workflow ID。列表通过
Workflow owner endpoint 加载 PNG；slot 缺失、下载失败或图片损坏时显示通用图片
占位符，不把 owner-relative object name 当作 URL 或 filesystem path。
