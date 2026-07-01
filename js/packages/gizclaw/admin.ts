import { createClient as createAdminServiceClient } from "./generated/adminservice/client/index.ts";
import type { Client as AdminServiceClient } from "./generated/adminservice/client/index.ts";
import { createAdminAPIFetch } from "./index.ts";
import type { WebRTCRPCDataChannelFactory, WebRTCServiceFetchOptions } from "./index.ts";

export { client as adminServiceClient } from "./generated/adminservice/client.gen.ts";
export * from "./generated/adminservice/index.ts";
export type { AdminServiceClient };

export function createAdminAPIClient(pc: WebRTCRPCDataChannelFactory, options: Omit<WebRTCServiceFetchOptions, "service"> = {}): AdminServiceClient {
  return createAdminServiceClient({
    baseUrl: "http://gizclaw",
    fetch: createAdminAPIFetch(pc, options),
  });
}
