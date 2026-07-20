# Flutter App <Badge type="warning" text="WIP" />

本页将说明 GizClaw Flutter App 的安装、权限、连接设备和常用操作。

App 内置与 `RuntimeProfile/default` 对应的固定 catalog：`doubao-realtime`、四个
`translate-*` alias、`chat`、`journey`、`murder-mystery`，以及内部使用的 `chatroom`。
App 不调用 `server.workflow.list` 发现产品能力。一个 Workspaces 入口统一列出全部
Workspace；唯一的 `+` 操作按 App 固定顺序展示八个可选 alias，并使用 App 自己的
i18n、icon 与 typed parameters 创建 `source=runtime` Workspace。

扫描 Desktop 本地 Pod 二维码后，App 将 raw registration credential 按 Server 保存到
安全存储，并把连接注册到 `RuntimeProfile/default`。App 使用固定的应用 token identity
`app:com.gizclaw.opensource`，不提供任意 RegistrationToken 编辑或选择；同一 Server
重新扫码时可以替换轮换后的 raw credential。

Flutter SDK 提供 Workspace 的 PNG/PIXA icon 下载方法。当前设备的
Peer profile PNG icon 由 Identity 页头像入口上传或删除；self RPC 不接受 public key，
因此只能修改当前连接 identity 自己的 icon。PNG 与 PIXA 单个文件上限均为 2 MiB。
