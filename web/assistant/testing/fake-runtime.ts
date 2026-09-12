import type { KnowledgePassage } from "../src/knowledge.ts";
import { guidesIndex } from "./guides.ts";
import type { ConsoleRoute } from "../src/routes.ts";
import {
  SourceError,
  type AssistantRuntime,
  type DeviceStatus,
  type DeviceWifi,
  type DeviceWorkspace,
  type HistoryEntry,
  type HistoryPage,
  type HistoryRequest,
  type LogEntry,
  type LogSearchPage,
  type LogSearchRequest,
  type NodeStatus,
  type TelemetryRange,
  type TelemetryRangeRequest,
  type TelemetryValue,
  type TrafficSample,
  type WatchedDevice,
} from "../src/runtime.ts";

/** A source answer that fails the way the console client would. */
export type Failure = {
  error: { code: string; message: string; httpStatus?: number };
};

export type WorldDevice = {
  publicKey: string;
  label: string;
  sn?: string;
  imei?: { tac: string; serial: string };
  /** Whether the device is on the console watch list; defaults to true. */
  watched?: boolean;
  status: DeviceStatus | Failure;
  telemetry?: TelemetryValue[] | Failure;
  /** Recorded points per telemetry field. */
  telemetryHistory?: Record<string, TelemetryRange["points"]> | Failure;
  wifi?: DeviceWifi | Failure;
  workspaces?: DeviceWorkspace[] | Failure;
  /** Conversation entries per workspace id, in any order. */
  history?: Record<string, HistoryEntry[]> | Failure;
  logs?: LogEntry[] | Failure;
};

/** Everything the assistant can observe in one scenario. */
export type World = {
  route: ConsoleRoute;
  /** What the current page shows; defaults to the route alone. */
  view?: unknown;
  nodes: NodeStatus[];
  /** In-memory traffic samples per node id, oldest first. */
  traffic?: Record<string, TrafficSample[]>;
  devices: WorldDevice[];
  /** The user's answer to an open_link confirmation; defaults to true. */
  acceptLinks?: boolean;
};

export type SourceCall = { method: string; args: unknown[] };

/**
 * An AssistantRuntime backed by a World fixture. It answers every source
 * from the fixture and records each call, navigation and link request.
 */
export class FakeRuntime implements AssistantRuntime {
  readonly calls: SourceCall[] = [];
  readonly navigations: ConsoleRoute[] = [];
  readonly links: string[] = [];
  private route: ConsoleRoute;
  private readonly world: World;

  constructor(world: World) {
    this.world = world;
    this.route = world.route;
  }

  get currentRoute(): ConsoleRoute {
    return this.route;
  }

  readonly page = {
    current: (): ConsoleRoute => {
      this.calls.push({ method: "page.current", args: [] });
      return this.route;
    },
    navigate: (route: ConsoleRoute): void => {
      this.calls.push({ method: "page.navigate", args: [route] });
      this.navigations.push(route);
      this.route = route;
    },
    openLink: async (url: string): Promise<boolean> => {
      this.calls.push({ method: "page.openLink", args: [url] });
      this.links.push(url);
      return this.world.acceptLinks ?? true;
    },
  };

  readonly view = {
    snapshot: (): unknown => {
      this.calls.push({ method: "view.snapshot", args: [] });
      return this.world.view ?? { route: this.route };
    },
  };

  readonly fleet = {
    nodes: async (): Promise<NodeStatus[]> => {
      this.calls.push({ method: "fleet.nodes", args: [] });
      return structuredClone(this.world.nodes);
    },
    traffic: async (
      nodeId: string,
      windowSeconds: number,
    ): Promise<TrafficSample[]> => {
      this.calls.push({
        method: "fleet.traffic",
        args: [nodeId, windowSeconds],
      });
      if (!this.world.nodes.some((node) => node.id === nodeId)) {
        throw new SourceError(
          "NOT_FOUND",
          `node ${nodeId} is not configured`,
          404,
        );
      }
      const samples = this.world.traffic?.[nodeId] ?? [];
      const last = Date.parse(samples.at(-1)?.time ?? "");
      return structuredClone(
        samples.filter(
          (sample) => Date.parse(sample.time) >= last - windowSeconds * 1000,
        ),
      );
    },
  };

  readonly devices = {
    watched: async (): Promise<WatchedDevice[]> => {
      this.calls.push({ method: "devices.watched", args: [] });
      return this.world.devices
        .filter((device) => device.watched ?? true)
        .map((device) => ({
          publicKey: device.publicKey,
          label: device.label,
          online:
            "error" in device.status ? undefined : device.status.runtime.online,
        }));
    },
    find: async (
      kind: "sn" | "imei",
      value: string,
      serial?: string,
    ): Promise<string[]> => {
      this.calls.push({ method: "devices.find", args: [kind, value, serial] });
      return this.world.devices
        .filter((device) =>
          kind === "sn"
            ? device.sn === value
            : device.imei?.tac === value && device.imei.serial === serial,
        )
        .map((device) => device.publicKey);
    },
    status: async (publicKey: string): Promise<DeviceStatus> => {
      this.calls.push({ method: "devices.status", args: [publicKey] });
      return answer(this.device(publicKey).status);
    },
    telemetry: async (
      publicKey: string,
      fields?: string[],
    ): Promise<TelemetryValue[]> => {
      this.calls.push({
        method: "devices.telemetry",
        args: [publicKey, fields],
      });
      const values = answer(this.device(publicKey).telemetry ?? []);
      return fields
        ? values.filter((value) => fields.includes(value.field))
        : values;
    },
    telemetryRange: async (
      request: TelemetryRangeRequest,
    ): Promise<TelemetryRange> => {
      this.calls.push({ method: "devices.telemetryRange", args: [request] });
      const history = answer(
        this.device(request.publicKey).telemetryHistory ?? {},
      );
      const points = (history[request.field] ?? []).filter(
        (point) =>
          point.observed_at_unix_ms >= request.startMs &&
          point.observed_at_unix_ms < request.endMs,
      );
      return { field: request.field, step_ms: 60_000, points };
    },
    wifi: async (publicKey: string): Promise<DeviceWifi> => {
      this.calls.push({ method: "devices.wifi", args: [publicKey] });
      const device = this.device(publicKey);
      if (!device.wifi) {
        throw new SourceError(
          "DEVICE_UNSUPPORTED",
          "the device does not report Wi-Fi",
          501,
        );
      }
      return answer(device.wifi);
    },
    workspaces: async (publicKey: string): Promise<DeviceWorkspace[]> => {
      this.calls.push({ method: "devices.workspaces", args: [publicKey] });
      return answer(this.device(publicKey).workspaces ?? []);
    },
    history: async (request: HistoryRequest): Promise<HistoryPage> => {
      this.calls.push({ method: "devices.history", args: [request] });
      const history = answer(this.device(request.publicKey).history ?? {});
      const entries = history[request.workspaceId];
      if (!entries) {
        throw new SourceError(
          "WORKSPACE_NOT_FOUND",
          `workspace ${request.workspaceId} not found`,
          404,
        );
      }
      const query = request.query?.toLowerCase();
      const items = entries
        .filter((entry) => !query || entry.text.toLowerCase().includes(query))
        .filter(
          (entry) =>
            request.startMs === undefined ||
            Date.parse(entry.created_at) >= request.startMs,
        )
        .filter(
          (entry) =>
            request.endMs === undefined ||
            Date.parse(entry.created_at) < request.endMs,
        )
        .sort((left, right) =>
          request.order === "desc"
            ? right.created_at.localeCompare(left.created_at)
            : left.created_at.localeCompare(right.created_at),
        );
      return { items: items.slice(0, request.limit) };
    },
  };

  readonly logs = {
    search: async (request: LogSearchRequest): Promise<LogSearchPage> => {
      this.calls.push({ method: "logs.search", args: [request] });
      const entries = answer(this.device(request.publicKey).logs ?? []);
      const query = request.query?.toLowerCase();
      const items = entries
        .filter(
          (entry) =>
            !request.level || entry.level.toLowerCase() === request.level,
        )
        .filter(
          (entry) =>
            request.sinceMs === undefined || entry.time_ms >= request.sinceMs,
        )
        .filter(
          (entry) =>
            request.untilMs === undefined || entry.time_ms <= request.untilMs,
        )
        .filter(
          (entry) =>
            !query ||
            `${entry.message} ${JSON.stringify(entry.fields ?? {})}`
              .toLowerCase()
              .includes(query),
        )
        .sort((left, right) => right.time_ms - left.time_ms);
      return { items: structuredClone(items) };
    },
  };

  readonly knowledge = {
    search: async (
      query: string,
      limit: number,
    ): Promise<KnowledgePassage[]> => {
      this.calls.push({ method: "knowledge.search", args: [query, limit] });
      return guidesIndex().search(query, limit);
    },
  };

  /** The source calls made so far, by method name. */
  called(method: string): SourceCall[] {
    return this.calls.filter((call) => call.method === method);
  }

  private device(publicKey: string): WorldDevice {
    const device = this.world.devices.find(
      (item) => item.publicKey === publicKey,
    );
    if (!device) {
      throw new SourceError(
        "NOT_FOUND",
        `device ${publicKey} is not known to this node`,
        404,
      );
    }
    return device;
  }
}

function answer<T>(value: T | Failure): T {
  if (typeof value === "object" && value !== null && "error" in value) {
    const { code, message, httpStatus } = (value as Failure).error;
    throw new SourceError(code, message, httpStatus);
  }
  return structuredClone(value as T);
}
