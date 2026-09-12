import {
  formatRoute,
  parseRoute,
  SourceError,
  type AssistantRuntime,
  type ConsoleRoute,
  type KnowledgeIndex,
  type NodeStatus,
} from "@gizclaw/assistant";
import { GizClawControlError } from "@gizclaw/gizclaw-control";

import type { FleetState } from "@/hooks/use-fleet";
import type { PeerState } from "@/hooks/use-peers";
import type { ConsoleConfig } from "@/lib/config";
import {
  findPeers,
  loadDeviceLogs,
  loadHistory,
  loadPeer,
  loadTelemetry,
  loadTelemetryRange,
  loadWifi,
  loadWorkspaces,
  normalizePublicKey,
  peerId,
  telemetryFields,
  type WatchedPeer,
} from "@/lib/peers";

import type { PageViewHolder } from "./page-view";

/** The latest console state, read at call time through refs. */
export type ConsoleRuntimeDeps = {
  config(): ConsoleConfig;
  fleet(): FleetState;
  peers(): WatchedPeer[];
  peerStates(): Record<string, PeerState>;
  /** Adds a device to the watch list so its pages can show it. */
  watch(peer: WatchedPeer): void;
  view: PageViewHolder;
  /** Cancels source requests when the panel closes or a new turn starts. */
  signal(): AbortSignal;
  window: Pick<Window, "location" | "confirm" | "open">;
  now(): number;
  /** The project guides index, loaded on the first search. */
  knowledge(): Promise<KnowledgeIndex>;
};

/** What the console hands the panel; the panel adds the rest per session. */
export type ConsoleStateDeps = Omit<ConsoleRuntimeDeps, "signal" | "knowledge">;

/**
 * Implements every assistant runtime method with the clients and state the
 * console already uses; nothing here talks to a new service.
 */
export function createConsoleRuntime(
  deps: ConsoleRuntimeDeps,
): AssistantRuntime {
  const deviceEndpoint = () =>
    deps.config().deviceEndpoint ?? deps.window.location.origin;
  const endpointFor = (publicKey: string) =>
    deps.peers().find((peer) => peer.publicKey === publicKey)?.endpoint ??
    deviceEndpoint();
  const key = (value: string) => normalizePublicKey(value);

  const ensureWatched = (publicKey: string) => {
    if (deps.peers().some((peer) => peer.publicKey === publicKey)) return;
    deps.watch({
      publicKey,
      label: "",
      endpoint: deviceEndpoint(),
      addedAt: deps.now(),
    });
  };

  return {
    page: {
      current: () => parseRoute(deps.window.location.hash),
      navigate: (route: ConsoleRoute) => {
        // Device pages and device logs only render watched devices.
        if (route.page === "peer") ensureWatched(key(route.publicKey));
        if (route.page === "logs" && route.query) {
          const named = /(?:^|\s)peer(?:_public_key)?:(\S+)/.exec(route.query);
          if (named) ensureWatched(key(named[1]));
        }
        deps.window.location.hash = formatRoute(route);
      },
      openLink: async (url: string) => {
        if (!deps.window.confirm(`诊断助手请求在新窗口打开：\n${url}`)) {
          return false;
        }
        deps.window.open(url, "_blank", "noopener,noreferrer");
        return true;
      },
    },
    view: {
      snapshot: () =>
        deps.view.current ?? { route: parseRoute(deps.window.location.hash) },
    },
    fleet: {
      nodes: async () =>
        deps.config().servers.map((server): NodeStatus => {
          const state = deps.fleet()[server.id];
          const last = state?.samples.at(-1);
          return {
            id: server.id,
            name: server.name,
            role: server.role,
            region: server.region,
            status: state?.status ?? "loading",
            error: state?.error,
            snapshot: state?.snapshot,
            rates: last
              ? { rx_bytes_per_second: last.rx, tx_bytes_per_second: last.tx }
              : undefined,
          };
        }),
      traffic: async (nodeId: string, windowSeconds: number) => {
        if (!deps.config().servers.some((server) => server.id === nodeId)) {
          throw new SourceError(
            "NOT_FOUND",
            `控制台配置中没有节点 ${nodeId}`,
            404,
          );
        }
        const samples = deps.fleet()[nodeId]?.samples ?? [];
        const since = deps.now() - windowSeconds * 1000;
        return samples
          .filter((sample) => sample.timestamp >= since)
          .map((sample) => ({
            time: new Date(sample.timestamp).toISOString(),
            rx_bytes_per_second: sample.rx,
            tx_bytes_per_second: sample.tx,
          }));
      },
    },
    devices: {
      watched: async () => {
        const states = deps.peerStates();
        return deps.peers().map((peer) => ({
          publicKey: peer.publicKey,
          label: peer.label,
          online: states[peerId(peer)]?.snapshot?.runtime.online,
        }));
      },
      find: (kind, value, serial) =>
        source(() =>
          findPeers(deviceEndpoint(), kind, value, serial ?? "", deps.signal()),
        ),
      status: (publicKey) =>
        source(() =>
          loadPeer(endpointFor(key(publicKey)), key(publicKey), deps.signal()),
        ),
      telemetry: async (publicKey, fields) => {
        const values = await source(() =>
          loadTelemetry(
            endpointFor(key(publicKey)),
            key(publicKey),
            deps.signal(),
          ),
        );
        return fields
          ? values.filter((value) => fields.includes(value.field))
          : values;
      },
      telemetryRange: async (request) => {
        const field = telemetryFields.find((item) => item === request.field);
        if (!field) {
          throw new SourceError(
            "UNSUPPORTED_FIELD",
            `控制台支持的 telemetry 字段：${telemetryFields.join(", ")}`,
          );
        }
        const publicKey = key(request.publicKey);
        return source(() =>
          loadTelemetryRange(
            endpointFor(publicKey),
            publicKey,
            field,
            request.startMs,
            request.endMs,
            deps.signal(),
          ),
        );
      },
      wifi: (publicKey) =>
        source(() =>
          loadWifi(endpointFor(key(publicKey)), key(publicKey), deps.signal()),
        ),
      workspaces: (publicKey) =>
        source(() =>
          loadWorkspaces(
            endpointFor(key(publicKey)),
            key(publicKey),
            deps.signal(),
          ),
        ),
      history: async (request) => {
        const publicKey = key(request.publicKey);
        const page = await source(() =>
          loadHistory(
            endpointFor(publicKey),
            publicKey,
            request.workspaceId,
            {
              query: request.query ?? "",
              cursor: request.cursor,
              order: request.order,
              startTimeMs: request.startMs,
              endTimeMs: request.endMs,
              limit: request.limit,
            },
            deps.signal(),
          ),
        );
        return { items: page.items, nextCursor: page.next_cursor };
      },
    },
    logs: {
      search: async (request) => {
        const publicKey = key(request.publicKey);
        const page = await source(() =>
          loadDeviceLogs(
            endpointFor(publicKey),
            publicKey,
            request.query ?? "",
            request.level
              ? (request.level.toUpperCase() as
                  "DEBUG" | "INFO" | "WARN" | "ERROR")
              : undefined,
            request.sinceMs ?? 0,
            request.untilMs ?? deps.now(),
            request.cursor,
            deps.signal(),
          ),
        );
        return { items: page.items, nextCursor: page.end.next_cursor };
      },
    },
    knowledge: {
      search: async (query, limit) =>
        (await deps.knowledge()).search(query, limit),
    },
  };
}

/** Turns console client failures into codes the assistant can explain. */
async function source<T>(run: () => Promise<T>): Promise<T> {
  try {
    return await run();
  } catch (cause) {
    if (cause instanceof GizClawControlError) {
      throw new SourceError(
        cause.code ??
          (cause.status ? `HTTP_${cause.status}` : cause.kind.toUpperCase()),
        cause.message,
        cause.status,
      );
    }
    if (cause instanceof TypeError) {
      throw new SourceError(
        "NETWORK_ERROR",
        `${cause.message}（可能是地址不可达、证书不受信任或节点未开放跨域）`,
      );
    }
    throw cause;
  }
}
