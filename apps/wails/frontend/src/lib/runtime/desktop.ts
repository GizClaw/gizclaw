import type { DesktopAPI } from "./types";

declare global {
  interface Window {
    go?: {
      main?: {
        App?: DesktopAPI;
      };
    };
    __GIZCLAW_DESKTOP_TEST_API__?: DesktopAPI;
  }
}

export function getDesktopAPI(): DesktopAPI {
  const api = window.go?.main?.App ?? window.__GIZCLAW_DESKTOP_TEST_API__;
  if (!api) {
    throw new Error("GizClaw desktop bridge is not available");
  }
  return api;
}
