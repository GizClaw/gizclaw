import type { KnowledgeDocument } from "./knowledge.ts";

// Built-in troubleshooting knowledge. Every statement restates the GizClaw
// guides (guides/zh/developing/monitor.md, guides/zh/developing/api/http/public.md,
// guides/zh/using/api-keys.md); update it together with those contracts.
export const BUILTIN_KNOWLEDGE: KnowledgeDocument[] = [
  {
    id: "builtin/debug-mode",
    title: "设备调试模式与访问权限",
    source: "内置",
    text: `# 设备调试模式与访问权限

## 调试模式
设备通过自身已认证的连接设置调试模式（debug_mode），取值 off（默认）、readonly、fullcontrol。设置由设备所属的 Server 持久化，断线重连后保持，缺失记录按 off 处理。

## 控制台如何读取设备
控制台读取设备状态、telemetry、日志和对话历史时，使用 Authorization: Bearer gizclaw_pk_<设备公钥>。所属 Server 每次都重新读取当前调试模式：readonly 只允许读取（GET），fullcontrol 允许设备与联系人接口的读写和控制，例如音量和重启。

## DEBUG_ACCESS_FORBIDDEN
设备接口返回 403 DEBUG_ACCESS_FORBIDDEN，表示设备的调试模式不允许这次操作：调试模式为 off 时任何读取都会被拒绝，readonly 时写操作会被拒绝。解决办法是在设备端开启只读调试模式（readonly），需要控制时开启 fullcontrol。关闭调试模式后新的请求会被拒绝，已开始的请求不会被撤销。`,
  },
  {
    id: "builtin/device-control-errors",
    title: "设备控制错误码",
    source: "内置",
    text: `# 设备控制错误码

## 离线与超时
设备不在线时控制接口返回 409 DEVICE_OFFLINE。普通控制在 5 秒内没有响应返回 504 DEVICE_TIMEOUT，Wi-Fi 扫描在其请求上限内没有响应同样返回 504。两者都不会改变已存储的设备状态。

## 设备拒绝或不支持
设备拒绝参数（INVALID_PARAMS）映射为 400 DEVICE_REJECTED；设备固件没有实现对应能力（METHOD_NOT_FOUND）映射为 501 DEVICE_UNSUPPORTED；其余设备错误映射为 502 DEVICE_ERROR，只返回脱敏的错误信息。

## 重启、升级和切换 Wi-Fi 之后
reboot、固件升级或切换 Wi-Fi 得到设备确认后，同一连接上的后续控制命令返回 409 DEVICE_OFFLINE，直到设备以新连接重连。切换 Wi-Fi 返回 202 只表示设备接受了凭据：设备随后必然掉线，重连后用 GET /device/wifi 查看 ssid 是否变成目标网络来判断成功，仍是旧网络表示加入失败并已回退。`,
  },
  {
    id: "builtin/node-monitor",
    title: "节点监控与 Monitor Token",
    source: "内置",
    text: `# 节点监控与 Monitor Token

## 节点快照接口
Server 与 Edge 在自己的 HTTP/HTTPS listener 上提供 GET /monitor/api/node，使用独立的 Authorization: Bearer gizclaw_mk_...。在节点配置文件中设置 monitor.token 开启，Token 以 gizclaw_mk_ 开头，后面至少 32 个字符，建议用 openssl rand -hex 32 生成。

## 503 与 401
节点没有配置 monitor.token 时接口关闭，返回 503（MONITOR_DISABLED）；控制台配置里的 Monitor Token 不对时返回 401（INVALID_MONITOR_TOKEN）。这两种情况都不代表节点宕机。

## 流量与连接数的含义
连接数是进程内的 WebRTC 关联数，包含上游连接；服务流单独计数。RX/TX 是进程启动以来的 WebRTC 服务负载字节，不含 ICE/DTLS 开销。控制台每 5 秒轮询一次，用累计计数换算速率，节点重启后速率读作 0 而不是尖峰；每个节点最多保留 600 个采样。`,
  },
  {
    id: "builtin/telemetry-logs",
    title: "Telemetry 与设备日志",
    source: "内置",
    text: `# Telemetry 与设备日志

## Telemetry 字段
指标字段写入采样存储，可以按时间区间查询历史：battery.*（例如 battery.percent、battery.charging、battery.voltage_mv）、network.rssi_dbm、network.signal_level、network.connected、system.*（例如 system.uptime_seconds、system.free_memory_bytes、system.temperature_c）和 gnss.*。只反映状态的值（固件与软件版本、蜂窝 rat、operator、imei、imsi、音频播放器状态、OTA 上报）只保留最新值，出现在设备状态字段里。

## 设备日志
设备日志来自持久化的 LogStore，只能按设备查询。查询支持时间区间、文本（最长 512 字节）、DEBUG/INFO/WARN/ERROR 级别和游标分页，每页最多 500 条；续页不能改用其他设备的游标。`,
  },
  {
    id: "builtin/workspace-history",
    title: "对话 Workspace 与对话历史",
    source: "内置",
    text: `# 对话 Workspace 与对话历史

## Workspace
设备的 Workspace 列表只包含该设备明确拥有的 Workspace（含系统 Workspace），不包含共享、无 owner 和正在删除的空间；每项用 collection 与 workflow_name 标识所属 Workflow。

## 对话历史
对话历史从持久化 History 读取，默认最新在前（desc），也可以按时间正序（asc），可以用创建时间区间过滤。游标格式非法返回 400 INVALID_HISTORY_CURSOR，时间区间等参数非法返回 400 INVALID_REQUEST。浏览历史不会启动 Agent。`,
  },
  {
    id: "builtin/api-keys",
    title: "设备 API Key",
    source: "内置",
    text: `# 设备 API Key

API Key 以 gizclaw_sk_v1_ 开头，长期有效、绑定一台设备，用来访问公开的 GizClaw API 和 OpenAI 兼容接口 /openai/v1。任何 Key 都能读取和控制它绑定的设备。完成注册的设备通过 Peer RPC 创建、列出和撤销 Key；Key 可以通过 DELETE /gizclaw/v1/api-keys/self 撤销自己。Key 不会自动过期，丢失或不再使用时应主动撤销；删除设备会撤销它的全部 Key。诊断助手用配置里的设备 API Key 调用该设备 RuntimeProfile 中的模型。`,
  },
];
