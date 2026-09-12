import type { KnowledgePassage } from "./knowledge.ts";
import type { ConsoleRoute } from "./routes.ts";

/**
 * Every way the assistant reaches outside itself. The console implements
 * these with its existing clients and hash router; tests use FakeRuntime.
 * Each method is exposed to the model through exactly one tool (apis.ts).
 */
export type AssistantRuntime = {
  page: PageController;
  view: PageView;
  fleet: FleetSource;
  devices: DeviceSource;
  logs: LogSource;
  knowledge: KnowledgeSource;
};

/** The knowledge base: built-in troubleshooting notes plus imported documents. */
export type KnowledgeSource = {
  search(query: string, limit: number): Promise<KnowledgePassage[]>;
};

export type PageController = {
  current(): ConsoleRoute;
  navigate(route: ConsoleRoute): void;
  /** Asks the user to open an external link; resolves false when declined. */
  openLink(url: string): Promise<boolean>;
};

export type PageView = {
  /** A JSON-compatible description of what the current page shows. */
  snapshot(): unknown;
};

export type FleetSource = {
  nodes(): Promise<NodeStatus[]>;
  /** Measured traffic samples the console holds in memory for one node. */
  traffic(nodeId: string, windowSeconds: number): Promise<TrafficSample[]>;
};

export type TrafficSample = {
  time: string;
  rx_bytes_per_second: number;
  tx_bytes_per_second: number;
};

export type NodeStatus = {
  id: string;
  name: string;
  role: "server" | "edge" | "unknown";
  region?: string;
  status: "loading" | "online" | "error";
  error?: string;
  snapshot?: {
    version?: string;
    build_commit?: string;
    uptime_seconds: number;
    goroutines: number;
    heap_bytes: number;
    transport: {
      connections: number;
      services: number;
      inbound_service_channels: number;
      rx_bytes: number;
      tx_bytes: number;
    };
  };
  /** Latest measured byte rates, when at least two samples exist. */
  rates?: { rx_bytes_per_second: number; tx_bytes_per_second: number };
};

export type DeviceSource = {
  watched(): Promise<WatchedDevice[]>;
  find(kind: "sn" | "imei", value: string, serial?: string): Promise<string[]>;
  status(publicKey: string): Promise<DeviceStatus>;
  telemetry(publicKey: string, fields?: string[]): Promise<TelemetryValue[]>;
  telemetryRange(request: TelemetryRangeRequest): Promise<TelemetryRange>;
  /** Current Wi-Fi link and saved networks; the device must be online. */
  wifi(publicKey: string): Promise<DeviceWifi>;
  workspaces(publicKey: string): Promise<DeviceWorkspace[]>;
  history(request: HistoryRequest): Promise<HistoryPage>;
};

export type TelemetryRangeRequest = {
  publicKey: string;
  field: string;
  startMs: number;
  endMs: number;
};

export type TelemetryRange = {
  field: string;
  step_ms: number;
  points: { observed_at_unix_ms: number; value: number }[];
};

export type DeviceWifi = {
  status: {
    connected: boolean;
    ssid?: string;
    rssi_dbm?: number;
    ip?: string;
    bssid?: string;
  };
  saved: string[];
};

export type DeviceWorkspace = {
  id: string;
  name: string;
  collection?: string;
  workflow_name?: string;
  available: boolean;
  system: boolean;
  last_active_at: string;
};

export type HistoryRequest = {
  publicKey: string;
  workspaceId: string;
  query?: string;
  order: "asc" | "desc";
  startMs?: number;
  endMs?: number;
  cursor?: string;
  limit: number;
};

export type HistoryPage = {
  items: HistoryEntry[];
  nextCursor?: string;
};

/** One conversation entry: who said what, and when. */
export type HistoryEntry = {
  name: string;
  actor_name: string;
  type: string;
  text: string;
  created_at: string;
};

export type WatchedDevice = {
  publicKey: string;
  label: string;
  online?: boolean;
};

export type DeviceStatus = {
  info: Record<string, unknown>;
  runtime: {
    online: boolean;
    debug_mode?: string;
    last_addr?: string;
    last_seen_at?: string;
    rx_bytes?: number;
    tx_bytes?: number;
  };
  status: Record<string, unknown>;
};

export type TelemetryValue = {
  field: string;
  value: unknown;
  observed_at_unix_ms: number;
};

export type LogSource = {
  search(request: LogSearchRequest): Promise<LogSearchPage>;
};

export type LogSearchRequest = {
  publicKey: string;
  query?: string;
  level?: "debug" | "info" | "warn" | "error";
  sinceMs?: number;
  untilMs?: number;
  cursor?: string;
};

export type LogSearchPage = {
  items: LogEntry[];
  nextCursor?: string;
};

export type LogEntry = {
  time_ms: number;
  level: string;
  message: string;
  source?: string;
  path?: string;
  fields?: Record<string, unknown>;
};

type AnyRuntimeMethod = {
  [
    Port in keyof AssistantRuntime
  ]: `${Port}.${Extract<keyof AssistantRuntime[Port], string>}`;
}[keyof AssistantRuntime];

/** Every AssistantRuntime method, as "port.method". */
export const RUNTIME_METHODS = [
  "page.current",
  "page.navigate",
  "page.openLink",
  "view.snapshot",
  "fleet.nodes",
  "fleet.traffic",
  "devices.watched",
  "devices.find",
  "devices.status",
  "devices.telemetry",
  "devices.telemetryRange",
  "devices.wifi",
  "devices.workspaces",
  "devices.history",
  "logs.search",
  "knowledge.search",
] as const satisfies readonly AnyRuntimeMethod[];

export type RuntimeMethod = (typeof RUNTIME_METHODS)[number];

// Fails to compile when a runtime method is missing from RUNTIME_METHODS.
type AssertNever<T extends never> = T;
export type RuntimeMethodsComplete = AssertNever<
  Exclude<AnyRuntimeMethod, RuntimeMethod>
>;

/**
 * A failure reported by a source. Tools hand the code and message to the
 * model as data instead of ending the conversation.
 */
export class SourceError extends Error {
  readonly code: string;
  readonly httpStatus?: number;

  constructor(code: string, message: string, httpStatus?: number) {
    super(message);
    this.name = "SourceError";
    this.code = code;
    this.httpStatus = httpStatus;
  }
}
