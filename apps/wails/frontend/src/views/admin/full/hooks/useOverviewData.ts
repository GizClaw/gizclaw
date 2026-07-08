import { useEffect, useState } from "react";

import { expectData, toMessage } from "@/dashboard";
import { listPeers } from "@gizclaw/gizclaw/admin";
import { getServerInfo, type ServerInfo } from "@gizclaw/gizclaw/peerhttp";

import type { Registration } from "@gizclaw/gizclaw/admin";

import { PEER_PAGE_LIMIT } from "./usePeersPage";

export interface OverviewData {
  error: string;
  peers: Registration[];
  loading: boolean;
  serverInfo: ServerInfo | null;
}

export function useOverviewData(): OverviewData {
  const [data, setData] = useState<OverviewData>({
    error: "",
    peers: [],
    loading: true,
    serverInfo: null,
  });

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const [serverInfo, registrations] = await Promise.all([
          expectData(getServerInfo()),
          expectData(
            listPeers({
              query: { limit: PEER_PAGE_LIMIT },
            }),
          ),
        ]);
        if (cancelled) {
          return;
        }
        setData({
          error: "",
          peers: registrations.items ?? [],
          loading: false,
          serverInfo,
        });
      } catch (error) {
        if (cancelled) {
          return;
        }
        setData((current) => ({
          ...current,
          error: toMessage(error),
          loading: false,
        }));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  return data;
}
