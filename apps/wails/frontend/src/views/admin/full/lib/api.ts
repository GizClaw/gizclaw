import { adminHTTPClient } from "@gizclaw/gizclaw/admin";
import { createAdminAPIFetch, createWebRTCServiceFetch, GIZCLAW_SERVICE_PEER_HTTP } from "@gizclaw/gizclaw";
import type { WebRTCRPCDataChannelFactory } from "@gizclaw/gizclaw";
import { peerHTTPClient } from "@gizclaw/gizclaw/peerhttp";

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
