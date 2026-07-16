# services/system

`pkgs/gizclaw/services/system` 提供多个产品领域共同依赖的系统级服务，包括访问控制、public login 和 declarative resource 管理。

## 目录结构

```text
services/system/
├── acl/               # Role、policy binding、ACL view 和授权判断
├── asset/             # Stable AssetRef、metadata、reverse binding 与 binary lifecycle
├── publiclogin/       # Public HTTP login、assertion 和 session
└── resourcemanager/   # Admin declarative resource 的统一入口
```

## 子目录职责

### acl

拥有 GizClaw 的 role、policy binding、ACL view、subject/resource permission 和授权判断。其他领域可以询问 ACL，但不能在各自 package 中建立互相冲突的第二套通用权限模型。

ACL 不负责 transport peer 是否能打开 giznet service；transport-level policy 与 product resource authorization 是不同边界。

### publiclogin

负责 public HTTP caller 使用 GizClaw identity 完成登录并取得 session。它连接 public HTTP identity 与 Server session，但不拥有 browser route、Edge proxy 或业务资源权限。

最终资源授权仍由 ACL 和对应领域服务执行。登录成功不等于拥有所有资源访问权限。

### resourcemanager

为 Admin apply、show 和通用 resource 操作提供统一的 declarative resource dispatch。它知道不同 resource kind 应交给哪个领域服务，但不重新实现 credential、workflow、firmware、gameplay 或 social 的业务规则。

ResourceManager 是跨领域协调层，不是所有 GizClaw resource 的实际 owner。

Resource owner 写入时，ResourceManager 先为新增 refs 建立 pending binding，owner commit 成功后激活并移除旧 refs；写入失败则回滚 pending binding。删除 Resource 后按 owner 清理 reverse index。binding 只标识 owner kind 与 canonical owner ID，不记录 slot、JSON path 或字段号。

### asset

AssetService 统一拥有 immutable binary asset 的稳定引用、safe metadata、reverse-reference indexes 与 ObjectStore lifecycle。公开 ref 固定为 `asset://<32-lowercase-hex>`，由 CSPRNG 生成；它不携带 backend、bucket、目录、owner、文件名或 media type。

metadata KV 保存 staging、ready、deleting lifecycle record，以及原子维护的 by-asset/by-owner binding indexes；逻辑 ObjectStore 只保存 `blobs/<id-prefix>/<id>` binary。`Put` 流式计算 size 与 SHA-256，`Open` 只读取 ready record，`Delete` 在 owner resolver 确认没有 live refs 后才停止服务并清理 object。`Reconcile` 由 Server lifecycle 显式调用，用于恢复 staging/deleting/pending 状态和报告 ready metadata/object 不一致。

AssetService 不决定业务授权或内容格式。Admin surface 使用 Admin authentication；普通 Peer download 由 adapter 先验证 Resource public display membership 与 ACL；PNG、PIXA、TAR 或 audio 的内容校验仍由 owner 领域负责。

## 依赖与边界

```mermaid
flowchart TB
    Admin["Admin resource surface"] --> ResourceManager["resourcemanager"]
    ResourceManager --> AI["services/ai"]
    ResourceManager --> Device["services/device"]
    ResourceManager --> Gameplay["services/gameplay"]
    ResourceManager --> Social["services/social"]
    ResourceManager --> ACL["acl"]
    ResourceManager --> Asset["asset"]
    Asset --> KV["metadata KV"]
    Asset --> Objects["ObjectStore"]
    Public["Public HTTP"] --> Login["publiclogin"]
    Login --> ACL
```

应该放在 `services/system`：

- 跨领域统一使用的 product authorization 和 session 能力。
- Declarative resource 的跨领域 dispatch 与公共管理边界。
- System-owned migration、validation 和持久化规则。

不应该放在这里：

- 各领域资源自己的业务实现。
- Giznet transport security policy 或 WebRTC signaling crypto。
- Edge proxy token forwarding。
- CLI config、storage backend 创建和进程生命周期。
- 为了避免选择领域 ownership 而放入的通用 helper。
