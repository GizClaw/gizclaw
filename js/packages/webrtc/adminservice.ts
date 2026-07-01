import { createClient as createAdminServiceClient } from "@gizclaw/adminservice/client";
import type { Client as AdminServiceClient } from "@gizclaw/adminservice/client";
import { createAdminAPIFetch } from "./index";
import type { WebRTCRPCDataChannelFactory, WebRTCServiceFetchOptions } from "./index";

export function createAdminAPIClient(pc: WebRTCRPCDataChannelFactory, options: Omit<WebRTCServiceFetchOptions, "service"> = {}): AdminServiceClient {
  return createAdminServiceClient({
    baseUrl: "http://gizclaw",
    fetch: createAdminAPIFetch(pc, options),
  });
}
