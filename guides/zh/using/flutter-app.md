# Flutter App <Badge type="warning" text="WIP" />

本页将说明 GizClaw Flutter App 的安装、权限、连接设备和常用操作。

App 使用当前语言请求 Workflow catalog，并展示 Server 返回的本地化名称和说明；
名称缺失时回退到稳定 Workflow ID，说明缺失时保持为空。存在 `icon.png` slot 时，
App 通过 Workflow owner RPC 下载并缓存 PNG；slot 缺失、下载失败或图片损坏时使用
driver 占位图标，不影响列表、选择和 Workspace 页面。

Flutter SDK 同时提供 Workflow 与 Workspace 的 PNG/PIXA icon 下载方法。当前设备的
Peer profile PNG icon 由 Identity 页头像入口上传或删除；self RPC 不接受 public key，
因此只能修改当前连接 identity 自己的 icon。PNG 与 PIXA 单个文件上限均为 2 MiB。
