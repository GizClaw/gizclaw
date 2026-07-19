# Flutter App <Badge type="warning" text="WIP" />

本页将说明 GizClaw Flutter App 的安装、权限、连接设备和常用操作。

App 通过 `source=runtime` 请求当前 RuntimeProfile 的 Workflow catalog。返回的 Workflow
ID 是稳定 alias，App 用本地 mapping 提供名称和图标；Server 不下发 Workflow icon 或
i18n。Workspace 创建时保存同一个 runtime source 与 alias。

Flutter SDK 提供 Workspace 的 PNG/PIXA icon 下载方法。当前设备的
Peer profile PNG icon 由 Identity 页头像入口上传或删除；self RPC 不接受 public key，
因此只能修改当前连接 identity 自己的 icon。PNG 与 PIXA 单个文件上限均为 2 MiB。
