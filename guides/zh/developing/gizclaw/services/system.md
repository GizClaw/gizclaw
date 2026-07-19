# services/system

`pkgs/gizclaw/services/system` 提供多个产品领域共同依赖的系统级服务，包括 RuntimeProfile、设备注册、resource ownership、public login 和 declarative resource 管理。

## 目录结构

```text
services/system/
├── ownership/         # owner context、owner index key 和写入规则
├── publiclogin/       # Public HTTP login、assertion 和 session
├── resourcemanager/   # Admin declarative resource 的统一入口
└── runtimeprofile/    # RuntimeProfile 与 RegistrationToken
```

## 子目录职责

### ownership

为 Workspace、Model、Credential 和 Tool 提供统一 owner context 与 KV index 约定。Owner 可以读取、使用、更新和删除资源；非 owner 不能通过 owner 机制访问。Friend、FriendGroup 和 Pet 的 system Workspace 由各自领域关系补充可见性。

### runtimeprofile

拥有 RuntimeProfile 和 RegistrationToken 的 KV 状态、校验、hash 索引和注册解析。RuntimeProfile allow list 与 owner 资源取并集，不定义只读、成员或管理员等 role。完整结构见 [RuntimeProfile 与设备注册](./runtime-profile)。

### publiclogin

负责 public HTTP caller 使用 GizClaw identity 完成登录并取得 typed session。Primary session 表示当前 Peer；Side Control session 使用单次 device token 授权，并同时绑定 controller identity 与目标 Peer。该 package 不拥有 browser route、Edge proxy 或业务资源实现。

最终资源访问仍由 RuntimeProfile、owner 和对应领域关系共同判断。登录成功不等于拥有所有资源访问权限。

### resourcemanager

为 Admin apply、show 和通用 resource 操作提供统一的 declarative resource dispatch。它知道不同 resource kind 应交给哪个领域服务，但不重新实现 credential、workflow、firmware、gameplay 或 social 的业务规则。

ResourceManager 是跨领域协调层，不是所有 GizClaw resource 的实际 owner。

## 依赖与边界

```mermaid
flowchart TB
    Admin["Admin resource surface"] --> ResourceManager["resourcemanager"]
    ResourceManager --> AI["services/ai"]
    ResourceManager --> Device["services/device"]
    ResourceManager --> Gameplay["services/gameplay"]
    ResourceManager --> Social["services/social"]
    ResourceManager --> Profile["runtimeprofile"]
    ResourceManager --> Ownership["ownership"]
    Public["Public HTTP"] --> Login["publiclogin"]
    Login --> Profile
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
