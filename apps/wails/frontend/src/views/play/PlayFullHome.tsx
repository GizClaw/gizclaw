import { useEffect, useState } from "react";

import { createWebRTCServiceFetch, GIZCLAW_SERVICE_PEER_OPENAI } from "@gizclaw/gizclaw";
import { createPeerRPCClient, RPC_METHODS } from "@gizclaw/gizclaw/rpc";
import { connectPlayPeerConnection } from "../../lib/gizclaw/play";
import { clearPlayOpenAIClient, configurePlayOpenAIClient } from "../../lib/gizclaw/openai";
import type { RuntimeContext } from "../../lib/runtime/types";
import { clearPlayDataClient, clearPlayRPCClient, clearPlayRuntime, configurePlayDataClient, configurePlayRPCClient, configurePlayRuntime } from "./full/peer-rpc-adapter";
import { PlayFullApp } from "./full/PlayFullApp";
import "./full/styles.css";

export function PlayFullHome({ onSignOut, runtime }: { onSignOut(): Promise<void>; runtime: RuntimeContext }) {
  const [error, setError] = useState("");
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let cancelled = false;
    let pc: RTCPeerConnection | undefined;
    let openAIFetch: typeof fetch | undefined;
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
      .then(async (next) => {
        if (cancelled) {
          next.close();
          return;
        }
        pc = next;
        const rpc = createPeerRPCClient(next);
        rpcClients.push(rpc);
        if (runtime.registration_token != null && runtime.registration_token !== "") {
          await rpc.call(RPC_METHODS["server.register"], { token: runtime.registration_token });
        }
        if (cancelled) {
          next.close();
          return;
        }
        configurePlayRPCClient(rpc);
        openAIFetch = createWebRTCServiceFetch(next, {
          requestTimeoutMs: 120_000,
          service: GIZCLAW_SERVICE_PEER_OPENAI,
        });
        configurePlayOpenAIClient(openAIFetch);
        setReady(true);
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          pc?.close();
          pc = undefined;
          setError(err instanceof Error ? err.message : String(err));
        }
      });
    return () => {
      cancelled = true;
      for (const rpc of rpcClients) {
        clearPlayRPCClient(rpc);
      }
      if (openAIFetch != null) {
        clearPlayOpenAIClient(openAIFetch);
      }
      clearPlayRuntime(runtime);
      pc?.close();
    };
  }, [runtime]);

  if (error !== "") {
    return <ViewConnectionState error={error} title="Play connection failed" />;
  }
  if (!ready) {
    return <ViewConnectionState title="Connecting Play RPC" />;
  }
  return <PlayFullApp contextName={runtime.context?.name} onSignOut={onSignOut} />;
}

function ViewConnectionState({ error = "", title }: { error?: string; title: string }): JSX.Element {
  return (
    <div className="flex h-screen items-center justify-center bg-slate-50 px-6">
      <div className="grid max-w-md gap-4 text-center">
        {error === "" ? <div className="mx-auto size-8 animate-spin rounded-full border-2 border-primary border-t-transparent" /> : null}
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {error === "" ? "Preparing the Play UI over WebRTC..." : error}
          </p>
        </div>
      </div>
    </div>
  );
}
