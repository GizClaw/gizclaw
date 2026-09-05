import { getNodeMonitor } from "./generated/monitor";
import { createClient, createConfig } from "./generated/monitor/client";
import { z } from "zod";
import {
  createPeerHTTPClient,
  createPeerHTTPConfig,
  getDevice,
  getDeviceRuntime,
  getDeviceStatus,
  getDeviceTelemetryLatest,
  getDeviceLogs,
  findPublicKeysBySn,
  findPublicKeysByImei,
  rebootDevice,
  setDeviceVolume,
} from "../../../sdk/js/gizclaw/peerhttp";
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
const client = createPeerHTTPClient(
  createPeerHTTPConfig({ baseUrl: window.location.origin }),
);
const monitorClient = createClient(
  createConfig({ baseUrl: window.location.origin }),
);
function unwrap<T>(result: {
  data?: T;
  error?: unknown;
  response?: Response;
}): T {
  if (!result.response?.ok || result.data === undefined)
    throw new Error(
      `${result.response?.status ?? "NETWORK_ERROR"} · ${JSON.stringify(result.error ?? "Empty response")}`,
    );
  return result.data;
}
const options = (token: string, signal: AbortSignal) => ({
  client,
  signal,
  headers: { Authorization: `Bearer ${token}` },
});
export async function loadNode(
  token: string,
  signal: AbortSignal,
): Promise<NodeSnapshot> {
  const response = await getNodeMonitor({
    client: monitorClient,
    signal,
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  });
  return nodeSchema.parse(unwrap(response));
}
export async function loadPeer(
  key: string,
  signal: AbortSignal,
): Promise<PeerSnapshot> {
  const opts = options(`gizclaw_pk_${key}`, signal);
  const [info, runtime, status, telemetry, logs] = await Promise.all([
    getDevice(opts),
    getDeviceRuntime(opts),
    getDeviceStatus(opts),
    getDeviceTelemetryLatest(opts),
    getDeviceLogs(opts),
  ]);
  return {
    info: record.parse(unwrap(info)),
    runtime: runtimeSchema.parse(unwrap(runtime)),
    status: record.parse(unwrap(status)),
    telemetry: record.parse(unwrap(telemetry)),
    logs: z.array(logSchema).parse(unwrap(logs)),
  };
}
export async function findPeers(
  kind: string,
  value: string,
  serial: string,
  signal: AbortSignal,
): Promise<string[]> {
  if (kind === "key") return [value.replace(/^gizclaw_pk_/, "")];
  const response =
    kind === "sn"
      ? await findPublicKeysBySn({ client, signal, path: { sn: value } })
      : await findPublicKeysByImei({
          client,
          signal,
          path: { tac: value, serial },
        });
  return z.object({ public_keys: z.array(z.string()) }).parse(unwrap(response))
    .public_keys;
}
export async function controlPeer(
  key: string,
  action: "reboot" | "volume",
  volume: number,
  signal: AbortSignal,
) {
  const opts = options(`gizclaw_pk_${key}`, signal);
  const result =
    action === "reboot"
      ? await rebootDevice(opts)
      : await setDeviceVolume({
          ...opts,
          body: { level: volume, muted: false },
        });
  if (!result.response?.ok)
    throw new Error(
      `${result.response?.status ?? "NETWORK_ERROR"} · ${JSON.stringify(result.error)}`,
    );
}
export type Sample = { time: string; rx: number; tx: number };
export function rates(
  previous: { time: number; rx: number; tx: number } | undefined,
  current: { time: number; rx: number; tx: number },
): Sample {
  const seconds = previous ? (current.time - previous.time) / 1000 : 0;
  return {
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
