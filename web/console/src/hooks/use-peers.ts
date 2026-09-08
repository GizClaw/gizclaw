import { useCallback, useEffect, useRef, useState } from "react";
import { nodeErrorMessage } from "@/lib/api";
import {
  loadPeer,
  parseWatchedPeers,
  peerId,
  type PeerSnapshot,
  type WatchedPeer,
} from "@/lib/peers";
import { readPeers, savePeers } from "@/lib/store";
import type { ConsoleConfigPeer } from "@/lib/config";
import { rates, type Sample } from "@/lib/format";

const MAX_SAMPLES = 600;

export type PeerState = {
  status: "loading" | "online" | "error";
  snapshot?: PeerSnapshot;
  error?: string;
  updatedAt?: number;
  samples: Sample[];
};

/**
 * The watch list is personal: it lives in this browser only, and each device is
 * polled through its own access point. Nothing here is shared with other
 * operators or written back to the cluster.
 */
export function useWatchedPeers(intervalMs = 5000, paused = false) {
  const [peers, setPeers] = useState<WatchedPeer[]>([]);
  const [states, setStates] = useState<Record<string, PeerState>>({});
  const counters = useRef<
    Map<string, { time: number; rx: number; tx: number }>
  >(new Map());
  const [storageError, setStorageError] = useState("");
  const list = useRef<WatchedPeer[]>([]);
  list.current = peers;

  useEffect(() => {
    let cancelled = false;
    readPeers()
      .then((text) => {
        if (!cancelled) setPeers(parseWatchedPeers(text));
      })
      .catch((error: unknown) => {
        if (!cancelled) setStorageError(nodeErrorMessage(error));
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const persist = useCallback((next: WatchedPeer[]) => {
    setPeers(next);
    savePeers(JSON.stringify(next)).catch((error: unknown) =>
      setStorageError(nodeErrorMessage(error)),
    );
  }, []);

  const add = useCallback(
    (peer: WatchedPeer) => {
      const kept = list.current.filter((item) => peerId(item) !== peerId(peer));
      persist([...kept, peer]);
    },
    [persist],
  );

  const remove = useCallback(
    (peer: WatchedPeer) => {
      persist(list.current.filter((item) => peerId(item) !== peerId(peer)));
      counters.current.delete(peerId(peer));
      setStates((current) => {
        const next = { ...current };
        delete next[peerId(peer)];
        return next;
      });
    },
    [persist],
  );

  /** Devices carried by an imported configuration join the personal list. */
  const importPeers = useCallback(
    (imported: ConsoleConfigPeer[], defaultEndpoint: string) => {
      if (imported.length === 0) return;
      const merged = [...list.current];
      for (const peer of imported) {
        const entry: WatchedPeer = {
          publicKey: peer.publicKey,
          label: peer.label,
          endpoint: peer.endpoint ?? defaultEndpoint,
          addedAt: Date.now(),
        };
        const index = merged.findIndex(
          (item) => peerId(item) === peerId(entry),
        );
        if (index >= 0)
          merged[index] = { ...merged[index], label: entry.label };
        else merged.push(entry);
      }
      persist(merged);
    },
    [persist],
  );

  const keys = peers.map(peerId).join(" ");
  useEffect(() => {
    const watched = list.current;
    if (paused || watched.length === 0) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const poll = async () => {
      await Promise.all(
        watched.map(async (peer) => {
          const id = peerId(peer);
          try {
            const snapshot = await loadPeer(
              peer.endpoint,
              peer.publicKey,
              controller.signal,
            );
            if (controller.signal.aborted) return;
            const now = Date.now();
            const sample =
              snapshot.runtime.rx_bytes === undefined ||
              snapshot.runtime.tx_bytes === undefined
                ? undefined
                : rates(counters.current.get(id), {
                    time: now,
                    rx: snapshot.runtime.rx_bytes,
                    tx: snapshot.runtime.tx_bytes,
                  });
            if (sample) {
              counters.current.set(id, {
                time: now,
                rx: snapshot.runtime.rx_bytes ?? 0,
                tx: snapshot.runtime.tx_bytes ?? 0,
              });
            }
            setStates((current) => ({
              ...current,
              [id]: {
                status: "online",
                snapshot,
                updatedAt: now,
                samples: sample
                  ? [...(current[id]?.samples ?? []), sample].slice(
                      -MAX_SAMPLES,
                    )
                  : (current[id]?.samples ?? []),
              },
            }));
          } catch (error) {
            if (controller.signal.aborted) return;
            setStates((current) => ({
              ...current,
              [id]: {
                snapshot: current[id]?.snapshot,
                samples: current[id]?.samples ?? [],
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
  }, [keys, intervalMs, paused]);

  return { peers, states, add, remove, importPeers, storageError };
}
