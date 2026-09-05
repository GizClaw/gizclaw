import {
  createGizClawPeerMonitorClient,
  createGizClawNodeMonitorClient,
  createGizClawDiscoveryClient,
} from "@gizclaw/gizclaw-control";
import { z } from "zod";
const record = z.record(z.string(), z.unknown());
export const logSchema = z.object({
  id: z.number(),
  time: z.string(),
  level: z.string(),
  message: z.string(),
  error: z.string().optional(),
  peer_public_key: z.string().optional(),
});
export type LogEntry = z.infer<typeof logSchema>;
export const nodeSchema = z.object({
  public_key: z.string(),
  role: z.string(),
  time: z.string(),
  uptime_seconds: z.number(),
  goroutines: z.number(),
  heap_bytes: z.number(),
  transport: z.object({
    connections: z.number(),
    services: z.number(),
    rx_bytes: z.number(),
    tx_bytes: z.number(),
  }),
  logs: z.array(logSchema),
});
export type NodeSnapshot = z.infer<typeof nodeSchema>;
const runtimeSchema = z
  .object({
    online: z.boolean(),
    rx_bytes: z.number().optional(),
    tx_bytes: z.number().optional(),
    debug_mode: z.string().optional(),
    last_addr: z.string().optional(),
    last_seen_at: z.string(),
  })
  .passthrough();
export type PeerSnapshot = {
  info: Record<string, unknown>;
  runtime: z.infer<typeof runtimeSchema>;
  status: Record<string, unknown>;
  telemetry: Record<string, unknown>;
  logs: LogEntry[];
};
const connectionOptions = (signal: AbortSignal) => ({
  baseUrl: window.location.origin,
  allowInsecureTransport: window.location.protocol === "http:",
  signal,
});
const peerClient = (publicKey: string, signal: AbortSignal) =>
  createGizClawPeerMonitorClient({ ...connectionOptions(signal), publicKey });
export async function loadNode(
  token: string,
  signal: AbortSignal,
): Promise<NodeSnapshot> {
  return nodeSchema.parse(
    await createGizClawNodeMonitorClient({
      ...connectionOptions(signal),
      token,
    }).get(),
  );
}
export async function loadPeer(
  key: string,
  signal: AbortSignal,
): Promise<PeerSnapshot> {
  const peer = peerClient(key, signal);
  const [info, runtime, status, telemetry] = await Promise.all([
    peer.get(),
    peer.getRuntime(),
    peer.getStatus(),
    peer.getTelemetryLatest(),
  ]);
  return {
    info: record.parse(info),
    runtime: runtimeSchema.parse(runtime),
    status: record.parse(status),
    telemetry: record.parse(telemetry),
    logs: [],
  };
}
export async function findPeers(
  kind: string,
  value: string,
  serial: string,
  signal: AbortSignal,
): Promise<string[]> {
  if (kind === "key") return [value.replace(/^gizclaw_pk_/, "")];
  const discovery = createGizClawDiscoveryClient(connectionOptions(signal));
  return kind === "sn"
    ? discovery.findBySn(value)
    : discovery.findByImei(value, serial);
}
export async function controlPeer(
  key: string,
  action: "reboot" | "volume",
  volume: number,
  signal: AbortSignal,
) {
  const peer = peerClient(key, signal);
  if (action === "reboot") await peer.reboot();
  else await peer.setVolume({ level: volume, muted: false });
}
export type Sample = {
  timestamp: number;
  time: string;
  rx: number;
  tx: number;
};
export function rates(
  previous: { time: number; rx: number; tx: number } | undefined,
  current: { time: number; rx: number; tx: number },
): Sample {
  const seconds = previous ? (current.time - previous.time) / 1000 : 0;
  return {
    timestamp: current.time,
    time: new Date(current.time).toLocaleTimeString(),
    rx:
      previous && seconds > 0
        ? Math.max(0, current.rx - previous.rx) / seconds
        : 0,
    tx:
      previous && seconds > 0
        ? Math.max(0, current.tx - previous.tx) / seconds
        : 0,
  };
}
export function bytes(value: number) {
  if (value < 1024) return `${Math.round(value)} B`;
  if (value < 1048576) return `${(value / 1024).toFixed(1)} KiB`;
  return `${(value / 1048576).toFixed(1)} MiB`;
}

export const telemetryValues = z.object({
  values: z.array(
    z.object({
      field: z.string(),
      value: z.number().finite(),
      observed_at_unix_ms: z.number(),
    }),
  ),
});
export const workspaceList = z.array(
  z.object({
    id: z.string(),
    name: z.string(),
    workflow_id: z.string(),
    last_active_at: z.string(),
  }),
);
export const historyPage = z.object({
  available: z.boolean(),
  items: z.array(
    z.object({
      name: z.string(),
      actor_name: z.string(),
      type: z.string(),
      text: z.string(),
      created_at: z.string(),
      replay_available: z.boolean(),
    }),
  ),
  has_next: z.boolean(),
  next_cursor: z.string().optional(),
});
export const persistentLogs = z.object({
  items: z.array(
    z.object({
      time_ms: z.number(),
      level: z.string(),
      message: z.string(),
      source: z.string(),
      path: z.string(),
      fields: z.record(z.string(), z.string()),
    }),
  ),
  end: z.object({ has_next: z.boolean(), next_cursor: z.string().optional() }),
});
export async function loadWorkspaces(key: string, signal: AbortSignal) {
  return workspaceList.parse(await peerClient(key, signal).listWorkspaces());
}
export async function loadHistory(
  key: string,
  id: string,
  query: string,
  cursor: string | undefined,
  signal: AbortSignal,
) {
  return historyPage.parse(
    await peerClient(key, signal).listWorkspaceHistory(id, {
      query,
      cursor,
      limit: 100,
    }),
  );
}
export async function loadLogs(
  key: string,
  query: string,
  level: "DEBUG" | "INFO" | "WARN" | "ERROR" | undefined,
  start: number,
  end: number,
  cursor: string | undefined,
  signal: AbortSignal,
) {
  return persistentLogs.parse(
    await peerClient(key, signal).searchLogs({
      query,
      level,
      start_time_ms: start,
      end_time_ms: end,
      cursor,
      limit: 200,
    }),
  );
}
export async function loadHistoryAudio(
  key: string,
  workspaceId: string,
  historyId: string,
  signal: AbortSignal,
): Promise<Blob> {
  return peerClient(key, signal).downloadHistoryAudio(workspaceId, historyId);
}
