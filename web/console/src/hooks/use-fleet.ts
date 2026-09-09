import { useEffect, useRef, useState } from "react";
import type { ConsoleServer } from "@/lib/config";
import { loadNode, nodeErrorMessage, type NodeSnapshot } from "@/lib/api";
import { rates, type Sample } from "@/lib/format";

const MAX_SAMPLES = 600;
export type ServerState = {
  status: "loading" | "online" | "error";
  snapshot?: NodeSnapshot;
  error?: string;
  updatedAt?: number;
  samples: Sample[];
};
export type FleetState = Record<string, ServerState>;

const initial = (): ServerState => ({
  status: "loading",
  samples: [],
});

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
