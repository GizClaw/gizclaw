import { useEffect, useState } from "react";
import { MemoryRouter } from "react-router-dom";

import { connectAdminPeerConnection } from "../../lib/gizclaw/admin";
import type { RuntimeContext } from "../../lib/runtime/types";
import { configureAdminClients, configureAdminClientsWithFetch } from "./full/lib/api";
import { AppRoutes } from "./full/router";
import "./full/styles.css";

export function AdminFullHome({ onSignOut, runtime }: { onSignOut(): Promise<void>; runtime: RuntimeContext }) {
  const [error, setError] = useState("");
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let cancelled = false;
    let pc: RTCPeerConnection | undefined;
    setError("");
    setReady(false);
    const testFetch = window.__GIZCLAW_DESKTOP_TEST_ADMIN_FETCH__;
    if (testFetch != null) {
      configureAdminClientsWithFetch(testFetch);
      setReady(true);
      return () => {
        cancelled = true;
      };
    }
    connectAdminPeerConnection(runtime)
      .then((next) => {
        if (cancelled) {
          next.close();
          return;
        }
        pc = next;
        configureAdminClients(next);
        setReady(true);
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err));
        }
      });
    return () => {
      cancelled = true;
      pc?.close();
    };
  }, [runtime]);

  if (error !== "") {
    return <ViewConnectionState error={error} title="Admin connection failed" />;
  }
  if (!ready) {
    return <ViewConnectionState title="Connecting Admin API" />;
  }
  return (
    <MemoryRouter initialEntries={["/overview"]}>
      <AppRoutes contextName={runtime.context?.name} onSignOut={onSignOut} />
    </MemoryRouter>
  );
}

function ViewConnectionState({ error = "", title }: { error?: string; title: string }): JSX.Element {
  return (
    <div className="flex h-screen items-center justify-center bg-muted/30 px-6">
      <div className="grid max-w-md gap-4 text-center">
        {error === "" ? <div className="mx-auto size-8 animate-spin rounded-full border-2 border-primary border-t-transparent" /> : null}
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {error === "" ? "Preparing the Admin UI over WebRTC..." : error}
          </p>
        </div>
      </div>
    </div>
  );
}

declare global {
  interface Window {
    __GIZCLAW_DESKTOP_TEST_ADMIN_FETCH__?: typeof fetch;
  }
}
