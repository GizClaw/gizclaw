import { adminServiceClient } from "@gizclaw/gizclaw/admin";
import { createAdminAPIFetch, createWebRTCServiceFetch, GIZCLAW_SERVICE_SERVER_PUBLIC } from "@gizclaw/gizclaw";
import type { WebRTCRPCDataChannelFactory } from "@gizclaw/gizclaw";
import { serverPublicClient } from "@gizclaw/gizclaw/serverpublic";

export function configureAdminClients(pc: WebRTCRPCDataChannelFactory): void {
  configureAdminClientsWithFetch(createAdminAPIFetch(pc), createWebRTCServiceFetch(pc, { service: GIZCLAW_SERVICE_SERVER_PUBLIC }));
}

export function configureAdminClientsWithFetch(adminFetch: typeof fetch, publicFetch: typeof fetch = adminFetch): void {
  adminServiceClient.setConfig({
    baseUrl: "http://gizclaw",
    fetch: adminFetch,
    responseStyle: "fields",
    throwOnError: false,
  });
  serverPublicClient.setConfig({
    baseUrl: "http://gizclaw",
    fetch: publicFetch,
    responseStyle: "fields",
    throwOnError: false,
  });
}
