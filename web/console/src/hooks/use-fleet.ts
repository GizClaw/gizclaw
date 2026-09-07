import { useEffect, useRef, useState } from "react";
import type { ConsoleServer } from "@/lib/config";
import { loadNode, nodeErrorMessage, type NodeSnapshot } from "@/lib/api";
import { rates, type Sample } from "@/lib/format";
import type { LogRecord } from "@/lib/log-query";

const MAX_SAMPLES = 600;
// Each snapshot carries the node's bounded ring, so the console keeps its own
// window of records that survives polls the ring has already rotated past.
const MAX_LOGS = 4000;

export type ServerState = {
  status: "loading" | "online" | "error";
  snapshot?: NodeSnapshot;
  error?: string;
  updatedAt?: number;
  samples: Sample[];
  logs: LogRecord[];
};
export type FleetState = Record<string, ServerState>;

const initial = (): ServerState => ({
  status: "loading",
  samples: [],
  logs: [],
});

// Record ids are per-process and monotonic; a lower id than the one already
// held means the node restarted, so the retained window starts over.
function mergeLogs(
  previous: LogRecord[],
  snapshot: NodeSnapshot,
  node: string,
  nodeName: string,
): LogRecord[] {
  const lastId = previous.at(-1)?.id ?? 0;
  const highest = snapshot.logs.at(-1)?.id ?? 0;
  const kept = highest < lastId ? [] : previous;
  const added = snapshot.logs
    .filter((entry) => entry.id > (kept.at(-1)?.id ?? 0))
    .map((entry) => ({ ...entry, node, nodeName }));
  if (added.length === 0) return kept;
  return [...kept, ...added].slice(-MAX_LOGS);
}

/** Polls every configured node in parallel; one failure never hides the rest. */
export function useFleet(
  servers: ConsoleServer[],
  intervalMs = 5000,
  paused = false,
) {
  const [fleet, setFleet] = useState<FleetState>({});
  const counters = useRef<
    Map<string, { time: number; rx: number; tx: number }>
  >(new Map());
  const targets = useRef(servers);
  targets.current = servers;
  const ids = servers.map((server) => server.id).join(" ");
  // An edited configuration can keep a node id while changing where or with
  // which token it is read, so polling restarts on the effective settings.
  const settings = servers
    .map(
      (server) => `${server.id}\u0000${server.url}\u0000${server.monitorToken}`,
    )
    .join("\u0001");

  useEffect(() => {
    const current = targets.current;
    setFleet((state) => {
      const next: FleetState = {};
      for (const server of current)
        next[server.id] = state[server.id] ?? initial();
      return next;
    });
    for (const id of counters.current.keys()) {
      if (!current.some((server) => server.id === id))
        counters.current.delete(id);
    }
  }, [ids]);

  useEffect(() => {
    if (paused || targets.current.length === 0) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const poll = async () => {
      await Promise.all(
        targets.current.map(async (server) => {
          try {
            const snapshot = await loadNode(server, controller.signal);
            const time = Date.parse(snapshot.time) || Date.now();
            const counter = {
              time,
              rx: snapshot.transport.rx_bytes,
              tx: snapshot.transport.tx_bytes,
            };
            const sample = rates(counters.current.get(server.id), counter);
            counters.current.set(server.id, counter);
            if (controller.signal.aborted) return;
            setFleet((state) => {
              const previous = state[server.id] ?? initial();
              return {
                ...state,
                [server.id]: {
                  status: "online",
                  snapshot,
                  updatedAt: Date.now(),
                  samples: [...previous.samples, sample].slice(-MAX_SAMPLES),
                  logs: mergeLogs(
                    previous.logs,
                    snapshot,
                    server.id,
                    server.name,
                  ),
                },
              };
            });
          } catch (error) {
            if (controller.signal.aborted) return;
            counters.current.delete(server.id);
            setFleet((state) => ({
              ...state,
              [server.id]: {
                ...(state[server.id] ?? initial()),
                status: "error",
                error: nodeErrorMessage(error),
                updatedAt: Date.now(),
              },
            }));
          }
        }),
      );
      if (!controller.signal.aborted) {
        timer = setTimeout(() => void poll(), intervalMs);
      }
    };
    void poll();
    return () => {
      controller.abort();
      if (timer) clearTimeout(timer);
    };
  }, [settings, intervalMs, paused]);

  return fleet;
}
