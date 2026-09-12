import type { LogEntry, NodeStatus } from "../src/runtime.ts";
import type { Expectation } from "./expectations.ts";
import type { World, WorldDevice } from "./fake-runtime.ts";
import type { ScriptStep } from "./scripted-model.ts";

export type Scenario = {
  name: string;
  description: string;
  world: World;
  /** User messages, sent in order. */
  turns: string[];
  /** Model steps for the scripted run; the live run ignores them. */
  script: ScriptStep[];
  expect: Expectation;
};

/** The fixed clock every scenario runs at. */
export const SCENARIO_NOW = Date.parse("2026-09-12T08:00:00Z");

const minutesAgo = (minutes: number) => SCENARIO_NOW - minutes * 60_000;

export const LIVING_ROOM = "7kZx2V9pQwErTyUi1oPaSdFgHjKlZxCvBnM3qW5eR8tY";
export const KIDS_ROOM = "4mNb6VcXz8LkJhGfDsA2pOiUyTrEwQ9aS3dF5gH7jKlZ";
export const BALCONY = "9pLo8KiJu7HyGt6FrDe5SwAq4ZxCv3BnMmNbVcXz2QwE";

const nodes: NodeStatus[] = [
  {
    id: "server-sh",
    name: "上海 Server",
    role: "server",
    region: "cn-shanghai",
    status: "online",
    snapshot: {
      version: "v0.42.3",
      uptime_seconds: 864_000,
      goroutines: 1_840,
      heap_bytes: 612_000_000,
      transport: {
        connections: 128,
        services: 40,
        inbound_service_channels: 22,
        rx_bytes: 9_800_000_000,
        tx_bytes: 12_400_000_000,
      },
    },
    rates: { rx_bytes_per_second: 820_000, tx_bytes_per_second: 1_100_000 },
  },
  {
    id: "edge-bj",
    name: "北京 Edge",
    role: "edge",
    region: "cn-beijing",
    status: "online",
    snapshot: {
      version: "v0.42.3",
      uptime_seconds: 432_000,
      goroutines: 6_200,
      heap_bytes: 1_450_000_000,
      transport: {
        connections: 512,
        services: 300,
        inbound_service_channels: 380,
        rx_bytes: 41_000_000_000,
        tx_bytes: 38_000_000_000,
      },
    },
    rates: { rx_bytes_per_second: 5_600_000, tx_bytes_per_second: 5_100_000 },
  },
  {
    id: "edge-gz",
    name: "广州 Edge",
    role: "edge",
    region: "cn-guangzhou",
    status: "online",
    snapshot: {
      version: "v0.42.3",
      uptime_seconds: 86_400,
      goroutines: 900,
      heap_bytes: 240_000_000,
      transport: {
        connections: 64,
        services: 30,
        inbound_service_channels: 12,
        rx_bytes: 1_200_000_000,
        tx_bytes: 1_300_000_000,
      },
    },
    rates: { rx_bytes_per_second: 120_000, tx_bytes_per_second: 140_000 },
  },
];

const forbidden = {
  error: {
    code: "DEBUG_ACCESS_FORBIDDEN",
    message: "device debug mode does not permit this operation",
    httpStatus: 403,
  },
};

function livingRoomLogs(): LogEntry[] {
  const logs: LogEntry[] = [];
  for (let index = 0; index < 17; index++) {
    logs.push({
      time_ms: minutesAgo(30 + index * 45),
      level: "error",
      message: "speech recognition timed out",
      source: "agent",
      fields: {
        operation: "asr.transcribe",
        error_code: "ASR_TIMEOUT",
        duration_ms: 15000,
      },
    });
  }
  for (let index = 0; index < 3; index++) {
    logs.push({
      time_ms: minutesAgo(90 + index * 200),
      level: "error",
      message: "tts upstream returned 502",
      source: "agent",
      fields: {
        operation: "tts.synthesize",
        error_code: "TTS_UPSTREAM_ERROR",
        status: 502,
      },
    });
  }
  for (let index = 0; index < 40; index++) {
    logs.push({
      time_ms: minutesAgo(10 + index * 30),
      level: "info",
      message: "turn completed",
      source: "agent",
      fields: { operation: "agent.turn", duration_ms: 1800 },
    });
  }
  return logs;
}

function balconyLogs(): LogEntry[] {
  const logs: LogEntry[] = [];
  for (let index = 0; index < 9; index++) {
    logs.push({
      time_ms: minutesAgo(20 + index * 130),
      level: "warn",
      message: "peer connection closed: heartbeat timeout",
      source: "giznet",
      fields: { operation: "peer.disconnect", error_code: "HEARTBEAT_TIMEOUT" },
    });
    logs.push({
      time_ms: minutesAgo(18 + index * 130),
      level: "info",
      message: "peer connected",
      source: "giznet",
      fields: { operation: "peer.connect" },
    });
  }
  return logs;
}

function storyHistory() {
  const entries = [];
  for (let index = 0; index < 3; index++) {
    entries.push(
      {
        name: `h-${index}-user`,
        actor_name: "user",
        type: "text",
        text: "给我讲一个恐龙的故事",
        created_at: new Date(minutesAgo(40 - index * 5)).toISOString(),
      },
      {
        name: `h-${index}-assistant`,
        actor_name: "assistant",
        type: "text",
        text: "抱歉，我没听清，请再说一遍。",
        created_at: new Date(minutesAgo(40 - index * 5) + 16_000).toISOString(),
      },
    );
  }
  return entries;
}

const devices: WorldDevice[] = [
  {
    publicKey: LIVING_ROOM,
    label: "客厅音箱",
    sn: "SN-LIVING-01",
    status: {
      info: { sn: "SN-LIVING-01", name: "客厅音箱", firmware: "1.8.2" },
      runtime: {
        online: true,
        debug_mode: "readonly",
        last_seen_at: new Date(minutesAgo(1)).toISOString(),
      },
      status: { battery_percent: 100, charging: true },
    },
    telemetry: [
      {
        field: "network.rssi_dbm",
        value: -52,
        observed_at_unix_ms: minutesAgo(1),
      },
      {
        field: "network.connected",
        value: true,
        observed_at_unix_ms: minutesAgo(1),
      },
    ],
    workspaces: [
      {
        id: "ws-story",
        name: "讲故事",
        collection: "assistants",
        workflow_name: "storyteller",
        available: true,
        system: false,
        last_active_at: new Date(minutesAgo(30)).toISOString(),
      },
    ],
    history: { "ws-story": storyHistory() },
    logs: livingRoomLogs(),
  },
  {
    publicKey: KIDS_ROOM,
    label: "儿童房故事机",
    sn: "SN-KIDS-02",
    status: forbidden,
    telemetry: forbidden,
    logs: forbidden,
  },
  {
    publicKey: BALCONY,
    label: "阳台音箱",
    sn: "SN-BALCONY-03",
    status: {
      info: { sn: "SN-BALCONY-03", name: "阳台音箱", firmware: "1.8.2" },
      runtime: {
        online: true,
        debug_mode: "readonly",
        last_seen_at: new Date(minutesAgo(18)).toISOString(),
      },
      status: { battery_percent: 64, charging: false },
    },
    telemetry: [
      {
        field: "network.rssi_dbm",
        value: -89,
        observed_at_unix_ms: minutesAgo(18),
      },
      {
        field: "network.signal_level",
        value: 1,
        observed_at_unix_ms: minutesAgo(18),
      },
      {
        field: "network.connected",
        value: true,
        observed_at_unix_ms: minutesAgo(18),
      },
      {
        field: "battery.percent",
        value: 64,
        observed_at_unix_ms: minutesAgo(18),
      },
    ],
    telemetryHistory: {
      "network.rssi_dbm": Array.from({ length: 24 }, (_, hour) => ({
        observed_at_unix_ms: minutesAgo((24 - hour) * 60),
        value: -66 - hour,
      })),
    },
    wifi: {
      status: {
        connected: true,
        ssid: "Home-5G",
        rssi_dbm: -89,
        ip: "192.168.1.42",
      },
      saved: ["Home-5G", "Home-2.4G"],
    },
    logs: balconyLogs(),
  },
];

const traffic = {
  "edge-bj": Array.from({ length: 120 }, (_, index) => ({
    time: new Date(SCENARIO_NOW - (119 - index) * 5_000).toISOString(),
    rx_bytes_per_second: 5_000_000 + index * 5_000,
    tx_bytes_per_second: 4_600_000 + index * 4_000,
  })),
};

const world = (partial: Pick<World, "route"> & Partial<World>): World => ({
  nodes,
  traffic,
  devices,
  ...partial,
});

export const scenarios: Scenario[] = [
  {
    name: "debug-mode-forbidden",
    description: "设备未开启调试模式，助手应查明原因并只给出开启指引",
    world: world({ route: { page: "overview" } }),
    turns: ["SN 是 SN-KIDS-02 的设备日志看不到了，帮我看看怎么回事"],
    script: [
      {
        call: [
          {
            name: "find_device",
            arguments: { kind: "sn", value: "SN-KIDS-02" },
          },
        ],
      },
      {
        call: [
          { name: "get_device_status", arguments: { public_key: KIDS_ROOM } },
        ],
      },
      {
        reply:
          "这台设备（儿童房故事机）没有开启调试模式，控制台读取它的状态和日志时返回 DEBUG_ACCESS_FORBIDDEN。请在设备端开启只读调试模式（readonly），开启后就能在它的设备详情页和日志页查看日志。",
      },
    ],
    expect: {
      tools: [
        { name: "find_device", arguments: { kind: "sn", value: "SN-KIDS-02" } },
        {
          // Any device read reveals the missing debug mode.
          name: ["get_device_status", "get_device_telemetry", "search_logs"],
          arguments: { public_key: KIDS_ROOM },
          error: "DEBUG_ACCESS_FORBIDDEN",
        },
      ],
      replyIncludes: ["调试模式", ["readonly", "只读"]],
      replyExcludes: ["已为你开启", "已为您开启", "已帮你开启", "已帮您开启"],
    },
  },
  {
    name: "device-recent-errors",
    description:
      "用户在设备详情页询问最近的错误，助手应查日志、归纳错误并跳到过滤后的日志页",
    world: world({
      route: { page: "peer", publicKey: LIVING_ROOM },
      view: {
        page: "设备详情",
        device: {
          public_key: LIVING_ROOM,
          label: "客厅音箱",
          online: true,
          debug_mode: "readonly",
        },
      },
    }),
    turns: ["这个设备最近有什么错误？"],
    script: [
      { call: [{ name: "get_current_page", arguments: {} }] },
      {
        call: [
          {
            name: "search_logs",
            arguments: { public_key: LIVING_ROOM, level: "error" },
          },
        ],
      },
      {
        call: [
          {
            name: "navigate",
            arguments: {
              page: "logs",
              public_key: LIVING_ROOM,
              error_code: "ASR_TIMEOUT",
            },
          },
        ],
      },
      {
        reply:
          "最近 24 小时客厅音箱有 20 条错误：ASR_TIMEOUT 17 次（语音识别超时，每次 15 秒），TTS_UPSTREAM_ERROR 3 次（TTS 上游返回 502）。主要问题是语音识别超时。我已跳转到按 ASR_TIMEOUT 过滤的日志页。",
      },
    ],
    expect: {
      tools: [
        { name: "search_logs", arguments: { public_key: LIVING_ROOM } },
        { name: "navigate", arguments: { page: "logs" } },
      ],
      route: {
        page: "logs",
        queryIncludes: [`peer_public_key:${LIVING_ROOM}`],
      },
      replyIncludes: ["ASR_TIMEOUT", "17"],
    },
  },
  {
    name: "busiest-node",
    description:
      "用户在集群总览询问哪个节点最忙，助手应比较节点并跳到该节点详情",
    world: world({
      route: { page: "overview" },
      view: { page: "集群总览", online_nodes: 3 },
    }),
    turns: ["现在哪个节点最忙？"],
    script: [
      { call: [{ name: "list_nodes", arguments: {} }] },
      {
        call: [{ name: "get_node_traffic", arguments: { node_id: "edge-bj" } }],
      },
      {
        call: [
          {
            name: "navigate",
            arguments: { page: "server", node_id: "edge-bj" },
          },
        ],
      },
      {
        reply:
          "北京 Edge（edge-bj）最忙：512 个连接、380 个入站服务通道，收发约 5.6/5.1 MB/s，远高于上海 Server 的 128 个连接和广州 Edge 的 64 个。我已打开它的节点详情页。",
      },
    ],
    expect: {
      tools: [{ name: "list_nodes" }],
      route: { page: "server", id: "edge-bj" },
      // The reply must carry at least one of edge-bj's own figures.
      replyIncludes: [
        ["edge-bj", "北京"],
        ["512", "380", "300", "5.6", "5.1", "5600000", "5,600,000"],
      ],
    },
  },
  {
    name: "offline-device-analysis",
    description: "用户说设备老掉线，助手应结合状态、telemetry 和日志分析原因",
    world: world({
      route: { page: "peers" },
      view: { page: "设备列表", watched: 3 },
    }),
    turns: ["阳台那台音箱老是掉线，帮我分析一下原因"],
    script: [
      { call: [{ name: "list_devices", arguments: {} }] },
      {
        call: [
          { name: "get_device_status", arguments: { public_key: BALCONY } },
          { name: "get_device_telemetry", arguments: { public_key: BALCONY } },
          { name: "search_logs", arguments: { public_key: BALCONY } },
          { name: "get_device_wifi", arguments: { public_key: BALCONY } },
        ],
      },
      {
        call: [
          {
            name: "query_device_telemetry",
            arguments: { public_key: BALCONY, field: "network.rssi_dbm" },
          },
        ],
      },
      {
        reply:
          "阳台音箱最近 24 小时断线 9 次，每次都是 HEARTBEAT_TIMEOUT（心跳超时），随后很快重连。它的 Wi-Fi 信号只有 -89 dBm（信号等级 1），明显偏弱，心跳包丢失导致断线。建议把设备靠近路由器或增加中继。",
      },
    ],
    expect: {
      tools: [
        { name: "get_device_status", arguments: { public_key: BALCONY } },
        {
          name: [
            "get_device_telemetry",
            "get_device_wifi",
            "query_device_telemetry",
          ],
          arguments: { public_key: BALCONY },
        },
        { name: "search_logs", arguments: { public_key: BALCONY } },
      ],
      replyIncludes: ["-89", ["HEARTBEAT_TIMEOUT", "心跳"]],
    },
  },
  {
    name: "conversation-review",
    description: "用户说设备回答得奇怪，助手应读对话历史、结合日志找出原因",
    world: world({
      route: { page: "peer", publicKey: LIVING_ROOM },
      view: {
        page: "设备详情",
        device: { public_key: LIVING_ROOM, label: "客厅音箱", online: true },
      },
    }),
    turns: ["这台音箱刚才回答得很奇怪，帮我看看它最近的对话"],
    script: [
      { call: [{ name: "get_current_page", arguments: {} }] },
      {
        call: [
          {
            name: "list_device_workspaces",
            arguments: { public_key: LIVING_ROOM },
          },
        ],
      },
      {
        call: [
          {
            name: "get_conversation_history",
            arguments: { public_key: LIVING_ROOM, workspace_id: "ws-story" },
          },
          {
            name: "search_logs",
            arguments: { public_key: LIVING_ROOM, level: "error" },
          },
        ],
      },
      {
        reply:
          "最近 40 分钟里用户 3 次请求讲恐龙故事，助手每次都回答「抱歉，我没听清，请再说一遍」。同一时间日志里有 ASR_TIMEOUT（语音识别超时），说明是语音识别超时导致助手没拿到用户的话。",
      },
    ],
    expect: {
      tools: [
        {
          name: "get_conversation_history",
          arguments: { public_key: LIVING_ROOM, workspace_id: "ws-story" },
        },
      ],
      replyIncludes: [["没听清", "听不清", "没有听清"]],
    },
  },
];
