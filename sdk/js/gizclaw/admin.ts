import { createClient as createAdminHTTPClient } from "./generated/adminhttp/client/index.ts";
import type { Client as AdminHTTPClient } from "./generated/adminhttp/client/index.ts";
import { createAdminAPIFetch } from "./index.ts";
import type { WebRTCRPCDataChannelFactory, WebRTCServiceFetchOptions } from "./index.ts";

export { client as adminHTTPClient } from "./generated/adminhttp/client.gen.ts";
export * from "./generated/adminhttp/index.ts";
export type { AdminHTTPClient };

export function createAdminAPIClient(pc: WebRTCRPCDataChannelFactory, options: Omit<WebRTCServiceFetchOptions, "service"> = {}): AdminHTTPClient {
  return createAdminHTTPClient({
    baseUrl: "http://gizclaw",
    fetch: createAdminAPIFetch(pc, options),
  });
}
