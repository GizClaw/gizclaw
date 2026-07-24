# Flutter App <Badge type="warning" text="WIP" />

本页将说明 GizClaw Flutter App 的安装、权限、连接设备和常用操作。

App 固定拥有 `assistants`、`translates`、`raids`、`story-teller` 和 `role-play`
五个导航 Collection。App 每次调用 `server.workflow.list` 都必须传一个 Collection，
并展示当前 RuntimeProfile 动态提供且本版本支持的 Workflow alias。选择 Workflow 后，
App 使用对应 `collection` 和 `workflow_alias` 创建新的 Workspace 并直接进入；不会再让
用户选择具体 Model 或 Voice。

添加 Server 时，用户可手动输入可选的 registration token，或扫描包含该 token 的 Desktop
本地 Pod 二维码。App 按 Server 将 token 保存到安全存储，并把连接注册到
`RuntimeProfile/default`。App 使用固定的应用 token identity
`app:com.gizclaw.opensource`；同一 Server 重新扫码时可以替换 Desktop 更新资源后的 token。

在“群组”页面，用户可输入邀请码加入群组。群主可从群组操作菜单创建、复制、轮换或
撤销当前邀请码；邀请码的有效期由 Server 策略决定。

Flutter SDK 提供 Workspace 的 PNG/PIXA icon 下载方法。当前设备的
Peer profile PNG icon 由 Identity 页头像入口上传或删除；self RPC 不接受 public key，
因此只能修改当前连接 identity 自己的 icon。PNG 与 PIXA 单个文件上限均为 2 MiB。
