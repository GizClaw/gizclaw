import { z } from "zod";

import { aggregateLogs, compactLog } from "./logs.ts";
import { formatRoute, type ConsoleRoute } from "./routes.ts";
import type { AssistantRuntime, RuntimeMethod } from "./runtime.ts";

/**
 * One runtime capability exposed to the model as one tool. The catalog below
 * is the single list of everything the assistant can do; every runtime method
 * is used by exactly one entry.
 */
export type ApiDefinition<Schema extends z.ZodObject = z.ZodObject> = {
  tool: string;
  /** The runtime methods this tool calls. */
  uses: RuntimeMethod[];
  description: string;
  params: Schema;
  run(
    runtime: AssistantRuntime,
    input: z.infer<Schema>,
    context: ApiContext,
  ): Promise<unknown> | unknown;
};

export type ApiContext = { now(): number };

/** A tool argument the zod schema cannot express, such as a conditional field. */
export class ApiInputError extends Error {}

/** The most compact log records returned to the model per search. */
export const LOG_RECORD_LIMIT = 50;
/** Conversation entries returned per history read. */
export const HISTORY_LIMIT = 50;

const DEFAULT_LOG_WINDOW_MINUTES = 24 * 60;
const DEFAULT_TELEMETRY_WINDOW_MINUTES = 24 * 60;
const MAX_WINDOW_MINUTES = 7 * 24 * 60;

const define = <Schema extends z.ZodObject>(api: ApiDefinition<Schema>) =>
  api as unknown as ApiDefinition;

const publicKey = z
  .string()
  .trim()
  .min(1)
  .describe("设备公钥（不带 gizclaw_pk_ 前缀）");
const windowMinutes = z.number().int().positive().max(MAX_WINDOW_MINUTES);

export const ASSISTANT_APIS: ApiDefinition[] = [
  define({
    tool: "get_current_page",
    uses: ["page.current", "view.snapshot"],
    description: "读取用户当前所在的控制台页面：路由、链接和页面上显示的数据。",
    params: z.object({}),
    run: (runtime) => {
      const route = runtime.page.current();
      return { route, hash: formatRoute(route), view: runtime.view.snapshot() };
    },
  }),
  define({
    tool: "navigate",
    uses: ["page.navigate"],
    description:
      "把控制台跳转到指定页面：overview 集群总览；server 节点详情（需要 node_id）；peers 设备列表；peer 设备详情（需要 public_key）；logs 日志查询，可用 public_key、error_code、level、text 设置过滤条件。",
    params: z.object({
      page: z.enum(["overview", "server", "peers", "peer", "logs"]),
      node_id: z
        .string()
        .trim()
        .min(1)
        .nullish()
        .describe("page=server 时必填"),
      public_key: publicKey
        .nullish()
        .describe("page=peer 时必填；page=logs 时按设备过滤"),
      error_code: z
        .string()
        .trim()
        .min(1)
        .nullish()
        .describe("page=logs 时按错误码过滤"),
      level: z
        .enum(["debug", "info", "warn", "error"])
        .nullish()
        .describe("page=logs 时按级别过滤"),
      text: z.string().trim().min(1).nullish().describe("page=logs 时的关键词"),
    }),
    run: (runtime, input) => {
      const route = consoleRoute(input);
      runtime.page.navigate(route);
      return { navigated: true, route, hash: formatRoute(route) };
    },
  }),
  define({
    tool: "open_link",
    uses: ["page.openLink"],
    description:
      "请用户确认后在新窗口打开一个 http(s) 链接。只在用户需要访问控制台以外的页面时使用。",
    params: z.object({
      url: z
        .url({ protocol: /^https?$/ })
        .describe("完整的 http 或 https 链接"),
    }),
    run: async (runtime, { url }) => ({
      opened: await runtime.page.openLink(url),
    }),
  }),
  define({
    tool: "list_nodes",
    uses: ["fleet.nodes"],
    description:
      "列出所有 Server/Edge 节点的状态、连接数、服务数、内存、运行时长和最近的收发速率。",
    params: z.object({}),
    run: async (runtime) => ({ nodes: await runtime.fleet.nodes() }),
  }),
  define({
    tool: "get_node_traffic",
    uses: ["fleet.traffic"],
    description:
      "读取某个节点最近一段时间的收发速率采样（控制台内存中最多保留约 50 分钟），用于判断流量趋势和突变。",
    params: z.object({
      node_id: z.string().trim().min(1),
      window_minutes: z
        .number()
        .int()
        .positive()
        .max(60)
        .nullish()
        .describe("默认 10 分钟"),
    }),
    run: async (runtime, { node_id, window_minutes }) => {
      const samples = await runtime.fleet.traffic(
        node_id,
        (window_minutes ?? 10) * 60,
      );
      return { node_id, samples, summary: rateSummary(samples) };
    },
  }),
  define({
    tool: "list_devices",
    uses: ["devices.watched"],
    description: "列出控制台关注列表中的设备。",
    params: z.object({}),
    run: async (runtime) => ({ devices: await runtime.devices.watched() }),
  }),
  define({
    tool: "find_device",
    uses: ["devices.find"],
    description:
      "按 SN 或 IMEI 查找设备公钥。IMEI 需要 value=TAC、serial=序列号。",
    params: z.object({
      kind: z.enum(["sn", "imei"]),
      value: z.string().trim().min(1),
      serial: z.string().trim().min(1).nullish(),
    }),
    run: async (runtime, { kind, value, serial }) => ({
      public_keys: await runtime.devices.find(kind, value, serial ?? undefined),
    }),
  }),
  define({
    tool: "get_device_status",
    uses: ["devices.status"],
    description:
      "读取设备在线状态、调试模式（debug_mode）、最后在线时间、地址以及设备上报的信息和状态。",
    params: z.object({ public_key: publicKey }),
    run: (runtime, { public_key }) => runtime.devices.status(public_key),
  }),
  define({
    tool: "get_device_telemetry",
    uses: ["devices.telemetry"],
    description:
      "读取设备最新的 telemetry，例如 battery.percent、network.rssi_dbm、network.connected、system.temperature_c。不填 fields 时读取全部。",
    params: z.object({
      public_key: publicKey,
      fields: z.array(z.string().min(1)).nullish(),
    }),
    run: async (runtime, { public_key, fields }) => ({
      values: await runtime.devices.telemetry(public_key, fields ?? undefined),
    }),
  }),
  define({
    tool: "query_device_telemetry",
    uses: ["devices.telemetryRange"],
    description:
      "读取设备某个 telemetry 字段在一段时间内的变化（服务端降采样），例如信号强度、电量、温度的走势。",
    params: z.object({
      public_key: publicKey,
      field: z.string().trim().min(1),
      since_minutes: windowMinutes.nullish().describe("默认 24 小时"),
    }),
    run: async (runtime, { public_key, field, since_minutes }, context) => {
      const now = context.now();
      const range = await runtime.devices.telemetryRange({
        publicKey: public_key,
        field,
        startMs:
          now - (since_minutes ?? DEFAULT_TELEMETRY_WINDOW_MINUTES) * 60_000,
        endMs: now,
      });
      return { ...range, summary: pointSummary(range.points) };
    },
  }),
  define({
    tool: "get_device_wifi",
    uses: ["devices.wifi"],
    description:
      "读取设备当前的 Wi-Fi 连接（SSID、信号、IP）和已保存的网络。设备必须在线。",
    params: z.object({ public_key: publicKey }),
    run: (runtime, { public_key }) => runtime.devices.wifi(public_key),
  }),
  define({
    tool: "list_device_workspaces",
    uses: ["devices.workspaces"],
    description:
      "列出设备的对话 Workspace（每个 Workflow 的存档），用于找到要查看对话历史的 Workspace。",
    params: z.object({ public_key: publicKey }),
    run: async (runtime, { public_key }) => ({
      workspaces: await runtime.devices.workspaces(public_key),
    }),
  }),
  define({
    tool: "get_conversation_history",
    uses: ["devices.history"],
    description: `读取设备某个 Workspace 的对话历史（用户和助手说了什么），默认最近的 ${HISTORY_LIMIT} 条，按时间倒序。`,
    params: z.object({
      public_key: publicKey,
      workspace_id: z.string().trim().min(1),
      text: z
        .string()
        .trim()
        .min(1)
        .nullish()
        .describe("只看包含该关键词的对话"),
      since_minutes: windowMinutes.nullish(),
      cursor: z
        .string()
        .nullish()
        .describe("上一次结果的 next_cursor，用于继续向前读取"),
    }),
    run: async (
      runtime,
      { public_key, workspace_id, text, since_minutes, cursor },
      context,
    ) => {
      const page = await runtime.devices.history({
        publicKey: public_key,
        workspaceId: workspace_id,
        query: text ?? undefined,
        order: "desc",
        startMs: since_minutes
          ? context.now() - since_minutes * 60_000
          : undefined,
        cursor: cursor ?? undefined,
        limit: HISTORY_LIMIT,
      });
      return { entries: page.items, next_cursor: page.nextCursor };
    },
  }),
  define({
    tool: "search_logs",
    uses: ["logs.search"],
    description: `查询设备日志，返回按级别、操作、错误码、RPC 状态码和 HTTP 状态统计的汇总，以及最多 ${LOG_RECORD_LIMIT} 条精简记录。默认查询最近 24 小时。结果里的 navigate_to_logs 可以直接作为 navigate 的参数，把用户带到对应的日志页。`,
    params: z.object({
      public_key: publicKey,
      text: z.string().nullish().describe("关键词，服务端全文匹配"),
      level: z.enum(["debug", "info", "warn", "error"]).nullish(),
      since_minutes: windowMinutes.nullish(),
      cursor: z
        .string()
        .nullish()
        .describe("上一次结果的 next_cursor，用于继续向前查询"),
    }),
    run: async (
      runtime,
      { public_key, text, level, since_minutes, cursor },
      context,
    ) => {
      const now = context.now();
      const page = await runtime.logs.search({
        publicKey: public_key,
        query: text ?? undefined,
        level: level ?? undefined,
        sinceMs: now - (since_minutes ?? DEFAULT_LOG_WINDOW_MINUTES) * 60_000,
        untilMs: now,
        cursor: cursor ?? undefined,
      });
      const summary = aggregateLogs(page.items);
      return {
        summary,
        records: page.items.slice(0, LOG_RECORD_LIMIT).map(compactLog),
        next_cursor: page.nextCursor,
        // Ready-made navigate arguments for showing these logs in the console.
        navigate_to_logs: {
          page: "logs",
          public_key,
          error_code: summary.top_errors[0]?.error_code ?? null,
        },
      };
    },
  }),
];

function consoleRoute(input: {
  page: ConsoleRoute["page"];
  node_id?: string | null;
  public_key?: string | null;
  error_code?: string | null;
  level?: string | null;
  text?: string | null;
}): ConsoleRoute {
  switch (input.page) {
    case "server":
      if (!input.node_id) throw new ApiInputError("page=server 需要 node_id");
      return { page: "server", id: input.node_id };
    case "peer":
      if (!input.public_key)
        throw new ApiInputError("page=peer 需要 public_key");
      return { page: "peer", publicKey: input.public_key };
    case "logs": {
      // The console log search syntax: field clauses plus free text.
      const query = [
        input.public_key && `peer_public_key:${input.public_key}`,
        input.error_code && `error_code:${input.error_code}`,
        input.level && `level:${input.level}`,
        input.text,
      ]
        .filter(Boolean)
        .join(" ");
      return query ? { page: "logs", query } : { page: "logs" };
    }
    case "overview":
    case "peers":
      return { page: input.page };
  }
}

function rateSummary(
  samples: { rx_bytes_per_second: number; tx_bytes_per_second: number }[],
) {
  if (samples.length === 0) return undefined;
  const rx = samples.map((sample) => sample.rx_bytes_per_second);
  const tx = samples.map((sample) => sample.tx_bytes_per_second);
  return {
    samples: samples.length,
    rx_bytes_per_second: {
      min: Math.min(...rx),
      max: Math.max(...rx),
      last: rx.at(-1),
    },
    tx_bytes_per_second: {
      min: Math.min(...tx),
      max: Math.max(...tx),
      last: tx.at(-1),
    },
  };
}

function pointSummary(points: { value: number }[]) {
  if (points.length === 0) return undefined;
  const values = points.map((point) => point.value);
  return {
    points: values.length,
    min: Math.min(...values),
    max: Math.max(...values),
    average: values.reduce((sum, value) => sum + value, 0) / values.length,
    last: values.at(-1),
  };
}
