import { useEffect, useState } from "react";
import { MemoryRouter } from "react-router-dom";

import {
  AdminPeerSessionManager,
  connectAdminPeerConnection,
} from "../../lib/gizclaw/admin";
import type { RuntimeContext } from "../../lib/runtime/types";
import {
  configureAdminClientsWithFetch,
  configureRecoveringAdminClients,
} from "./full/lib/api";
import { AppRoutes } from "./full/router";
import "./full/styles.css";

export function AdminFullHome({
  onSignOut,
  runtime,
}: {
  onSignOut(): Promise<void>;
  runtime: RuntimeContext;
}) {
  const [error, setError] = useState("");
  const [ready, setReady] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    let session: AdminPeerSessionManager | undefined;
    setError("");
    setReady(false);
    setReconnecting(false);
    const testFetch = window.__GIZCLAW_DESKTOP_TEST_ADMIN_FETCH__;
    if (testFetch != null) {
      configureAdminClientsWithFetch(testFetch);
      setReady(true);
      return () => {
        cancelled = true;
      };
    }
    session = new AdminPeerSessionManager({
      connect: (signal) => connectAdminPeerConnection(runtime, signal),
      onState: (state) => {
        if (cancelled) {
          return;
        }
        if (state.status === "connecting") {
          setError("");
          setReady(false);
          setReconnecting(false);
        } else if (state.status === "reconnecting") {
          setError("");
          setReconnecting(true);
        } else if (state.status === "ready") {
          setError("");
          setReady(true);
          setReconnecting(false);
        } else if (state.status === "failed") {
          setError(state.error.message);
          setReady(false);
          setReconnecting(false);
        }
      },
    });
    configureRecoveringAdminClients(session);
    session
      .start()
      .then(() => {
        if (cancelled) {
          session?.close();
          return;
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err));
        }
      });
    return () => {
      cancelled = true;
      session?.close();
    };
  }, [runtime]);

  if (error !== "") {
    return (
      <ViewConnectionState error={error} title="Admin connection failed" />
    );
  }
  if (!ready) {
    return <ViewConnectionState title="Connecting Admin API" />;
  }
  return (
    <>
      <MemoryRouter initialEntries={["/overview"]}>
        <AppRoutes contextName={runtime.context?.name} onSignOut={onSignOut} />
      </MemoryRouter>
      {reconnecting ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm">
          <ViewConnectionState title="Reconnecting Admin API" />
        </div>
      ) : null}
    </>
  );
}

function ViewConnectionState({
  error = "",
  title,
}: {
  error?: string;
  title: string;
}): JSX.Element {
  return (
    <div className="flex h-screen items-center justify-center bg-muted/30 px-6">
      <div className="grid max-w-md gap-4 text-center">
        {error === "" ? (
          <div className="mx-auto size-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        ) : null}
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
