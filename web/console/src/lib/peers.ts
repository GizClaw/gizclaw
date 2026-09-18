import {
  GizClawControlError,
  createGizClawDiscoveryClient,
  createGizClawPeerMonitorClient,
  type PeerTelemetryField,
  type PeerTelemetryLatestResponse,
} from "@gizclaw/gizclaw-control";
import { z } from "zod";
import { isLocal } from "@/lib/config";
import { nodeErrorMessage } from "@/lib/api";

/**
 * A device the operator watches. The endpoint records which access point the
 * device was read through; it is this page's own origin unless the
 * configuration overrides it, because the console is served from the access
 * point itself.
 */
export type WatchedPeer = {
  publicKey: string;
  label: string;
  endpoint: string;
  addedAt: number;
};

const watchedSchema = z.array(
  z.object({
    publicKey: z.string().min(1),
    label: z.string().default(""),
    endpoint: z.string().min(1),
    addedAt: z.number().default(0),
  }),
);

export function parseWatchedPeers(text: string): WatchedPeer[] {
  if (text.trim() === "") return [];
  try {
    const parsed = watchedSchema.safeParse(JSON.parse(text));
    return parsed.success ? parsed.data : [];
  } catch {
    return [];
  }
}

export function peerId(peer: Pick<WatchedPeer, "endpoint" | "publicKey">) {
  return `${peer.endpoint}/${peer.publicKey}`;
}

const runtimeSchema = z.looseObject({
  online: z.boolean(),
  rx_bytes: z.number().optional(),
  tx_bytes: z.number().optional(),
  debug_mode: z.string().optional(),
  last_addr: z.string().optional(),
  last_seen_at: z.string(),
  // Recorded by the Server, so both answer while the device is offline.
  active_workspace_name: z.string().optional(),
  pending_workspace_name: z.string().optional(),
});
const record = z.record(z.string(), z.unknown());

export type PeerSnapshot = {
  info: Record<string, unknown>;
  runtime: z.infer<typeof runtimeSchema>;
  status: Record<string, unknown>;
};

/** Accepts a bare host and requires HTTPS outside localhost, like node URLs. */
export function normalizeEndpoint(raw: string): string {
  const value = /^https?:\/\//i.test(raw.trim())
    ? raw.trim()
    : `https://${raw.trim()}`;
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    throw new Error(`${raw} 不是合法的接入点地址`);
  }
  if (url.username || url.password || url.search || url.hash) {
    throw new Error(`${raw} 不能包含账号密码、查询参数或片段`);
  }
  if (
    url.protocol !== "https:" &&
    !(url.protocol === "http:" && isLocal(url.hostname))
  ) {
    throw new Error(`${raw} 必须使用 HTTPS（localhost 可用 HTTP）`);
  }
  return url.origin;
}

function connection(endpoint: string, signal: AbortSignal) {
  const url = new URL(endpoint);
  return {
    baseUrl: endpoint,
    allowInsecureTransport: url.protocol === "http:" && isLocal(url.hostname),
    signal,
  };
}

export function normalizePublicKey(value: string): string {
  return value.trim().replace(/^gizclaw_pk_/, "");
}

/** Public identifier lookup; a duplicate identifier returns every match. */
export async function findPeers(
  endpoint: string,
  kind: "sn" | "imei" | "key",
  value: string,
  serial: string,
  signal: AbortSignal,
): Promise<string[]> {
  if (kind === "key") {
    const key = normalizePublicKey(value);
    return key === "" ? [] : [key];
  }
  const discovery = createGizClawDiscoveryClient(connection(endpoint, signal));
  return kind === "sn"
    ? discovery.findBySn(value.trim())
    : discovery.findByImei(value.trim(), serial.trim());
}

export async function loadPeer(
  endpoint: string,
  publicKey: string,
  signal: AbortSignal,
): Promise<PeerSnapshot> {
  const peer = createGizClawPeerMonitorClient({
    ...connection(endpoint, signal),
    publicKey,
  });
  const [info, runtime, status] = await Promise.all([
    peer.get(),
    peer.getRuntime(),
    peer.getStatus(),
  ]);
  return {
    info: record.parse(info),
    runtime: runtimeSchema.parse(runtime),
    status: record.parse(status),
  };
}

/** Best-effort display name from whatever the device record carries. */
export function peerLabel(
  peer: WatchedPeer,
  snapshot: PeerSnapshot | undefined,
): string {
  if (peer.label !== "") return peer.label;
  const info = snapshot?.info ?? {};
  for (const key of ["name", "sn", "serial_number", "imei"]) {
    const value = info[key];
    if (typeof value === "string" && value !== "") return value;
  }
  return `${peer.publicKey.slice(0, 10)}…`;
}

function client(endpoint: string, publicKey: string, signal: AbortSignal) {
  return createGizClawPeerMonitorClient({
    ...connection(endpoint, signal),
    publicKey,
  });
}

// The device endpoint answers one telemetry field per request; the console
// picks the fields it displays and limits how many run at once.
export const telemetryFields = [
  "battery.percent",
  "battery.charging",
  "battery.voltage_mv",
  "network.rssi_dbm",
  "network.signal_level",
  "network.connected",
  "system.uptime_seconds",
  "system.free_memory_bytes",
  "system.temperature_c",
  "gnss.latitude",
  "gnss.longitude",
  "gnss.altitude_m",
  "gnss.accuracy_m",
] as const satisfies readonly PeerTelemetryField[];

export type TelemetryValue = {
  field: string;
  value: number;
  observed_at_unix_ms: number;
};

export async function loadTelemetry(
  endpoint: string,
  publicKey: string,
  signal: AbortSignal,
): Promise<TelemetryValue[]> {
  const peer = client(endpoint, publicKey, signal);
  const values: PeerTelemetryLatestResponse["values"] = [];
  for (let offset = 0; offset < telemetryFields.length; offset += 4) {
    signal.throwIfAborted();
    const page = await Promise.all(
      telemetryFields
        .slice(offset, offset + 4)
        .map((field) => peer.getTelemetryLatest(field)),
    );
    for (const result of page) values.push(...result.values);
  }
  return telemetrySchema.parse({ values }).values;
}

const telemetrySchema = z.object({
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
    collection: z.string().optional(),
    workflow_name: z.string().optional(),
    available: z.boolean(),
    system: z.boolean(),
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

export const deviceLogs = z.object({
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

export async function loadWorkspaces(
  endpoint: string,
  publicKey: string,
  signal: AbortSignal,
) {
  return workspaceList.parse(
    await client(endpoint, publicKey, signal).listWorkspaces(),
  );
}

/** One server-side History read; the server owns ordering and time bounds. */
export type HistoryRequest = {
  query: string;
  /** Exclusive entry-ID boundary: a next_cursor or any loaded item name. */
  cursor?: string;
  order: "asc" | "desc";
  /** Inclusive lower bound on created_at, Unix milliseconds. */
  startTimeMs?: number;
  /** Exclusive upper bound on created_at, Unix milliseconds. */
  endTimeMs?: number;
  limit?: number;
};

export async function loadHistory(
  endpoint: string,
  publicKey: string,
  workspaceId: string,
  request: HistoryRequest,
  signal: AbortSignal,
) {
  return historyPage.parse(
    await client(endpoint, publicKey, signal).listWorkspaceHistory(
      workspaceId,
      {
        query: request.query,
        cursor: request.cursor,
        order: request.order,
        start_time_ms: request.startTimeMs,
        end_time_ms: request.endTimeMs,
        limit: request.limit ?? 100,
      },
    ),
  );
}

export function loadHistoryAudio(
  endpoint: string,
  publicKey: string,
  workspaceId: string,
  historyId: string,
  signal: AbortSignal,
): Promise<Blob> {
  return client(endpoint, publicKey, signal).downloadHistoryAudio(
    workspaceId,
    historyId,
  );
}

export async function loadDeviceLogs(
  endpoint: string,
  publicKey: string,
  query: string,
  level: "DEBUG" | "INFO" | "WARN" | "ERROR" | undefined,
  start: number,
  end: number,
  cursor: string | undefined,
  signal: AbortSignal,
) {
  return deviceLogs.parse(
    await client(endpoint, publicKey, signal).searchLogs({
      query,
      level,
      start_time_ms: start,
      end_time_ms: end,
      cursor,
      limit: 200,
    }),
  );
}

const rangeSchema = z.object({
  field: z.string(),
  step_ms: z.number(),
  points: z.array(
    z.object({ observed_at_unix_ms: z.number(), value: z.number().finite() }),
  ),
});

/** The device's current Wi-Fi link and saved networks; it must be online. */
export async function loadWifi(
  endpoint: string,
  publicKey: string,
  signal: AbortSignal,
) {
  const peer = client(endpoint, publicKey, signal);
  const [status, saved] = await Promise.all([
    peer.getWifi(),
    peer.listSavedWifi(),
  ]);
  return { status, saved: saved.networks.map((network) => network.ssid) };
}

/** Server-side telemetry history: real stored samples, not a session buffer. */
export async function loadTelemetryRange(
  endpoint: string,
  publicKey: string,
  field: (typeof telemetryFields)[number],
  startMs: number,
  endMs: number,
  signal: AbortSignal,
) {
  const result = await client(endpoint, publicKey, signal).queryTelemetry({
    field,
    start_time_ms: startMs,
    end_time_ms: endMs,
    limit: 500,
    order: "asc",
  });
  // The Server downsamples to a derived step, so a long window over a short
  // recording legitimately collapses to a single point.
  return rangeSchema.parse(result);
}

/** Device-reported status members the detail page summarizes at a glance. */
export type PeerGlance = {
  activity?: string;
  activityDetail?: string;
  activityObservedAt?: string;
  firmwareVersion?: string;
  firmwareSha256?: string;
  /** The more recently observed network route; Wi-Fi wins a tie. */
  signal?:
    | { kind: "wifi"; rssiDbm: number; observedAt?: string }
    | {
        kind: "cellular";
        rssiDbm?: number;
        level?: number;
        observedAt?: string;
      };
};

const glanceSchema = z.looseObject({
  activity: z.string().optional().catch(undefined),
  activity_detail: z.string().optional().catch(undefined),
  firmware_version: z.string().optional().catch(undefined),
  firmware_sha256: z.string().optional().catch(undefined),
  wifi_rssi_dbm: z.number().finite().optional().catch(undefined),
  cellular_rssi_dbm: z.number().finite().optional().catch(undefined),
  cellular_signal_level: z.number().finite().optional().catch(undefined),
  telemetry_observed_at: z
    .record(z.string(), z.unknown())
    .optional()
    .catch(undefined),
});

/** Reads the at-a-glance members; malformed members read as absent. */
export function peerGlance(status: Record<string, unknown>): PeerGlance {
  const parsed = glanceSchema.safeParse(status);
  if (!parsed.success) return {};
  const value = parsed.data;
  const observed = (field: string) => {
    const at = value.telemetry_observed_at?.[field];
    return typeof at === "string" && at !== "" ? at : undefined;
  };
  const glance: PeerGlance = {
    activity: value.activity || undefined,
    activityDetail: value.activity_detail || undefined,
    activityObservedAt: observed("activity"),
    firmwareVersion: value.firmware_version || undefined,
    firmwareSha256: value.firmware_sha256 || undefined,
  };
  const wifi =
    value.wifi_rssi_dbm === undefined
      ? undefined
      : {
          kind: "wifi" as const,
          rssiDbm: value.wifi_rssi_dbm,
          observedAt: observed("wifi_rssi_dbm"),
        };
  const cellular =
    value.cellular_rssi_dbm === undefined &&
    value.cellular_signal_level === undefined
      ? undefined
      : {
          kind: "cellular" as const,
          rssiDbm: value.cellular_rssi_dbm,
          level: value.cellular_signal_level,
          observedAt: latest(
            observed("cellular_rssi_dbm"),
            observed("cellular_signal_level"),
          ),
        };
  if (wifi && cellular) {
    glance.signal =
      latest(wifi.observedAt, cellular.observedAt) === wifi.observedAt
        ? wifi
        : cellular;
  } else {
    glance.signal = wifi ?? cellular;
  }
  return glance;
}

/** The later of two observation times; a missing or invalid time loses. */
function latest(a: string | undefined, b: string | undefined) {
  const time = (value: string | undefined) => {
    const parsed = value === undefined ? NaN : Date.parse(value);
    return Number.isFinite(parsed) ? parsed : -Infinity;
  };
  return time(b) > time(a) ? b : (a ?? b);
}

/**
 * One independently loaded device section. Offline and unsupported are
 * expected states of a monitored device, not failures.
 */
export type DeviceSection<T> =
  | { state: "ok"; data: T }
  | { state: "offline" }
  | { state: "unsupported" }
  | { state: "error"; message: string };

export function deviceSectionError(error: unknown): DeviceSection<never> {
  if (error instanceof GizClawControlError) {
    if (error.kind === "deviceOffline") return { state: "offline" };
    if (error.kind === "deviceUnsupported") return { state: "unsupported" };
    if (error.kind === "deviceTimeout") {
      return {
        state: "error",
        message: `设备未在超时时间内响应 · ${nodeErrorMessage(error)}`,
      };
    }
  }
  return { state: "error", message: nodeErrorMessage(error) };
}

async function section<T>(
  read: () => Promise<T>,
  signal: AbortSignal,
): Promise<DeviceSection<T>> {
  try {
    return { state: "ok", data: await read() };
  } catch (error) {
    // An aborted read belongs to a view that is gone; never report it.
    if (signal.aborted) throw error;
    return deviceSectionError(error);
  }
}

const deviceToolList = z.object({
  items: z.array(
    z.looseObject({
      name: z.string(),
      control_access: z.string(),
      i18n: z
        .record(
          z.string(),
          z.looseObject({
            display_name: z.string(),
            description: z.string().optional(),
          }),
        )
        .default({}),
      input_schema: z.record(z.string(), z.unknown()).default({}),
    }),
  ),
});
export type DeviceTool = z.infer<typeof deviceToolList>["items"][number];

export type DeviceConfig = {
  settings: DeviceSection<Record<string, unknown>>;
  rpcMethods: DeviceSection<string[]>;
  tools: DeviceSection<DeviceTool[]>;
};

/**
 * Device settings, the RPC methods its firmware implements and the Tools its
 * RuntimeProfile exposes to the control app. The three reads run in parallel
 * and fail independently, so an offline device still lists its Tools.
 */
export async function loadDeviceConfig(
  endpoint: string,
  publicKey: string,
  signal: AbortSignal,
): Promise<DeviceConfig> {
  const peer = client(endpoint, publicKey, signal);
  const [settings, rpcMethods, tools] = await Promise.all([
    section(async () => record.parse(await peer.getSettings()), signal),
    section(
      async () =>
        z
          .object({ methods: z.array(z.string()) })
          .parse(await peer.listRpcMethods())
          .methods.toSorted(),
      signal,
    ),
    section(
      async () => deviceToolList.parse(await peer.listTools()).items,
      signal,
    ),
  ]);
  return { settings, rpcMethods, tools };
}
