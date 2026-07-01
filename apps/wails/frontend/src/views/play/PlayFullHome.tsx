import { useEffect, useState } from "react";

import { createPeerRPCClient } from "@gizclaw/gizclaw/rpc";
import { connectPlayPeerConnection } from "../../lib/gizclaw/play";
import type { RuntimeContext } from "../../lib/runtime/types";
import { clearPlayDataClient, clearPlayRPCClient, clearPlayRuntime, configurePlayDataClient, configurePlayRPCClient, configurePlayRuntime } from "./full/peer-rpc-adapter";
import { PlayFullApp } from "./full/PlayFullApp";
import "./full/styles.css";

export function PlayFullHome({ runtime }: { runtime: RuntimeContext }) {
  const [error, setError] = useState("");
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let cancelled = false;
    let pc: RTCPeerConnection | undefined;
    const rpcClients: ReturnType<typeof createPeerRPCClient>[] = [];
    setError("");
    setReady(false);
    configurePlayRuntime(runtime);
    const testClient = window.__GIZCLAW_DESKTOP_TEST_PLAY_CLIENT__;
    if (testClient != null) {
      configurePlayDataClient(testClient);
      setReady(true);
      return () => {
        clearPlayDataClient(testClient);
        clearPlayRuntime(runtime);
      };
    }
    connectPlayPeerConnection(runtime)
      .then((next) => {
        if (cancelled) {
          next.close();
          return;
        }
        pc = next;
        const rpc = createPeerRPCClient(next);
        rpcClients.push(rpc);
        configurePlayRPCClient(rpc);
        setReady(true);
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err));
        }
      });
    return () => {
      cancelled = true;
      for (const rpc of rpcClients) {
        clearPlayRPCClient(rpc);
      }
      clearPlayRuntime(runtime);
      pc?.close();
    };
  }, [runtime]);

  if (error !== "") {
    return <div className="rounded-md border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">{error}</div>;
  }
  if (!ready) {
    return <div className="rounded-md border bg-card p-4 text-sm text-muted-foreground">Connecting Play RPC over WebRTC...</div>;
  }
  return <PlayFullApp />;
}
