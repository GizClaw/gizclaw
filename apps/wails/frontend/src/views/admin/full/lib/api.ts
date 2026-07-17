import { adminHTTPClient } from "@gizclaw/gizclaw/admin";
import { createAdminAPIFetch, createWebRTCServiceFetch, GIZCLAW_SERVICE_PEER_HTTP } from "@gizclaw/gizclaw";
import type { WebRTCRPCDataChannelFactory } from "@gizclaw/gizclaw";
import { peerHTTPClient } from "@gizclaw/gizclaw/peerhttp";

import type { AdminPeerSessionManager } from "../../../../lib/gizclaw/admin";

export function configureAdminClients(pc: WebRTCRPCDataChannelFactory): void {
  configureAdminClientsWithFetch(createAdminAPIFetch(pc), createWebRTCServiceFetch(pc, { service: GIZCLAW_SERVICE_PEER_HTTP }));
}

export function configureAdminClientsWithFetch(adminFetch: typeof fetch, publicFetch: typeof fetch = adminFetch): void {
  adminHTTPClient.setConfig({
    baseUrl: "http://gizclaw",
    fetch: adminFetch,
    responseStyle: "fields",
    throwOnError: false,
  });
  peerHTTPClient.setConfig({
    baseUrl: "http://gizclaw",
    fetch: publicFetch,
    responseStyle: "fields",
    throwOnError: false,
  });
}

export function configureRecoveringAdminClients(session: AdminPeerSessionManager): void {
  configureAdminClientsWithFetch(
    createRecoveringServiceFetch(session, (connection) => createAdminAPIFetch(connection)),
    createRecoveringServiceFetch(session, (connection) => createWebRTCServiceFetch(connection, { service: GIZCLAW_SERVICE_PEER_HTTP })),
  );
}

export function createRecoveringServiceFetch(
  session: AdminPeerSessionManager,
  createFetch: (connection: RTCPeerConnection) => typeof fetch,
): typeof fetch {
  return async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const connection = await session.connection();
    try {
      return await createFetch(connection)(input, init);
    } catch (error: unknown) {
      if (requestWasAborted(input, init, error)) {
        throw error;
      }
      const replacement = await session.recover(connection);
      try {
        return await createFetch(replacement)(input, init);
      } catch (retryError: unknown) {
        throw session.fail(retryError);
      }
    }
  };
}

function requestWasAborted(input: RequestInfo | URL, init: RequestInit | undefined, error: unknown): boolean {
  const signal = init?.signal ?? (input instanceof Request ? input.signal : undefined);
  return signal?.aborted === true || (error instanceof Error && error.name === "AbortError");
}
