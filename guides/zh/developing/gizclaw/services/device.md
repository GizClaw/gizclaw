# services/device

`pkgs/gizclaw/services/device` 保存由设备领域拥有的服务端资源。目前该目录只有 `firmware/`，负责 Firmware catalog 和 OTA channel 配置。

## 目录结构

```text
services/device/
└── firmware/    # Firmware metadata 和 external channel package
```

## firmware

`firmware` 拥有：

- Firmware catalog 和 channel metadata。
- 校验和保存每个 channel 的 HTTPS `.tar.zlib` URL、SHA-256 和 archive size。
- pending、stable 与 beta slot 之间的 release 和 rollback transition。

它不拥有设备连接、peer registration、runtime status 或 telemetry。设备通过什么 transport 连接、当前是否在线、上报了什么状态，属于根 peer 接线与 `services/runtime`。

## 依赖与边界

```mermaid
flowchart LR
    GizClaw["pkgs/gizclaw<br/>Admin surface"] --> Firmware["services/device/firmware"]
    Firmware --> KV["KV metadata store"]
```

应该放在 `services/device/firmware`：

- Firmware 和 channel 的领域规则。
- External package metadata 与 release/rollback 行为。
- Firmware 配置作为不可信输入时的 validation。

不应该放在这里：

- WebRTC connection、device signaling 或 telemetry transport。
- Peer identity、RegistrationToken 或通用 resource ownership。
- Board-specific flash、bootloader 或 firmware implementation。
- Package download、proxy、unpack 或 binary storage。
- CLI storage backend 和 filesystem root 的创建。

未来新增 device 领域服务时，应先确认它是否拥有独立资源和生命周期，再决定新增 `services/device/<service>`，不要把所有与设备有关的逻辑都放进 `firmware/`。
