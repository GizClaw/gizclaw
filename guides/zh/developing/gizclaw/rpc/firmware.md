# Firmware RPC

设备从自己已绑定的 Firmware 查询发布渠道，再自行下载并校验包。Admin 负责维护 Firmware 资源与绑定；GizClaw 的 RPC 提供发布信息，不代理包下载。

[RPC Schema](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/firmware.proto) · [Firmware Resource Schema](https://github.com/GizClaw/gizclaw/blob/main/api/http/resources/firmware.json) · [查询实现](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/services/runtime/peerresource/firmware.go)

版本用于展示发布身份，URL、digest 与大小用于完成下载和完整性校验。设备应校验下载到的压缩包，再交给自己的安装流程；得到发布信息不代表 OTA 已完成。

## 固件 Metadata

Metadata 为使用方提供不依赖发布渠道的 JSON 配置。例如，设备可以从一个 key 读取调制解调器的附属发布信息：

```yaml
spec:
  metadata:
    modem:
      version: vendor-release
      urls:
        - https://firmware.example.com/modem/image.bin
```

Server 不解释这些值的业务含义。客户端按 key 读取后，将返回的 JSON 文本解码成应用自己的结构；显式 JSON null 与缺少 key 是不同情况。

请求、响应、支持的渠道和校验限制直接查阅上述 Schema。SDK 的具体方法见 [RPC Reference](/references/rpc#firmware) 与各语言的 API Reference；C 调用方还需遵守 [request 生命周期](/references/streams#binary-streams)。
