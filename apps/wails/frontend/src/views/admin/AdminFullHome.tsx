import { useEffect, useState } from "react";
import { MemoryRouter } from "react-router-dom";

import { connectAdminPeerConnection } from "../../lib/gizclaw/admin";
import type { RuntimeContext } from "../../lib/runtime/types";
import { configureAdminClients, configureAdminClientsWithFetch } from "./full/lib/api";
import { AppRoutes } from "./full/router";
import "./full/styles.css";

export function AdminFullHome({ runtime }: { runtime: RuntimeContext }) {
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
    return <div className="rounded-md border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">{error}</div>;
  }
  if (!ready) {
    return <div className="rounded-md border bg-card p-4 text-sm text-muted-foreground">Connecting Admin API over WebRTC...</div>;
  }
  return (
    <MemoryRouter initialEntries={["/overview"]}>
      <AppRoutes />
    </MemoryRouter>
  );
}

declare global {
  interface Window {
    __GIZCLAW_DESKTOP_TEST_ADMIN_FETCH__?: typeof fetch;
  }
}
