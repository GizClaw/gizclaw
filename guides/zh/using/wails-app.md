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
  添加该 Server，并用本地 App registration credential 完成注册。新建本地 Pod 只包含
  `RuntimeProfile/default`，动态创建 `RegistrationToken/app:com.gizclaw.opensource`，
  不创建 Firmware。背面可启动、停止和重启 Server，并打开 Admin 或 Play。
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

首页的 Bootstrap 状态入口列出当前 RuntimeProfile 所选 Raids 资源需要的环境变量。值只写入 Desktop
配置目录的私有文件。为了回填表单和 dotenv 文本编辑器，受信任的 Desktop
Renderer/WebView 可以读取文件全文和已保存值；只来自启动进程或资源默认值的内容
不会回传，窗口只看到对应变量已配置或正在使用默认值。输入新值会替换保存值，
“清除已保存值”会删除本地覆盖。Desktop 保存值优先于启动进程的同名环境变量。

缺少必填值时仍可管理现有 Pod 或创建远程 Pod，但不能创建本地 Pod。补齐后创建
本地 Pod 会在 manifest 和投影保存后立即回到首页。Pod 卡片显示“正在初始化数据”；
点开后可查看持续更新的初始化状态，也可以关闭详情稍后再看。后台任务会启动新的
Server、下载或复用私有缓存的 Raids `v0.2.1` archive，apply 当前 RuntimeProfile 所选的
Credential/Tenant/Model/Voice/Workflow/PetDef，上传匹配的内置 PIXA 二进制，再 apply
唯一的 `RuntimeProfile/default`。全部完成后详情自动切换为正常界面。

初始化失败会停止 Server，并在 Pod 详情中保留脱敏错误、目录入口和删除操作。退出
Desktop 或崩溃时仍在初始化的 Pod 会在下次启动时清理；已经成功创建的 Pod 在
Desktop 或 Server 重启时不会重放完整 catalog，因此用户后续修改和删除的资源会保留。
旧版 local Pod 会在 Server ready 后执行一次兼容迁移，只安装
`RuntimeProfile/default`、轮换固定 App RegistrationToken 并删除旧 Desktop token，不改动
其他资源。若 Desktop 恢复到旧版 Server 进程，会先使用当前 companion 重启；旧翻译
alias 会继续保留，已有 Workspace 不需要重建。
迁移完成前分享页不会展示旧 token 的二维码；点击打开 Play 会自动启动当前 Server、
完成迁移并交付新 token。

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

Admin 的 Resource 编辑页为 Workspace 与 GameDef 提供 PNG/PIXA icon 上传、下载和删除；Peer 详情页使用 Peer 自己的 Admin icon endpoint。界面不会调用通用 Resource icon 或 Asset API。每个格式使用独立 slot，单个文件上限为 2 MiB。

Workflow 不包含服务端 icon 或 i18n。GizClaw App 使用与
`RuntimeProfile/default` 对应的固定 alias catalog 展示名称与图标，不通过动态 Workflow
list 构建产品菜单。
