// Test-only HTTP boundary for the real Monitor loaders and control SDK.
import { vi } from "vitest";

export function monitorHTTPFixture() {
  const peer = {
    endpoint: "https://monitor.example",
    publicKey: "signed-device",
    label: "Signed device",
    addedAt: 1,
    signedBearer: signedBearer("first"),
  };
  const requests: {
    path: string;
    method: string;
    authorization: string | null;
  }[] = [];
  let accepted = peer.signedBearer;
  const time = Date.now();
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = new Request(input, init);
    const path = new URL(request.url).pathname.replace(
      "/gizclaw/v1/device",
      "",
    );
    requests.push({
      path,
      method: request.method,
      authorization: request.headers.get("Authorization"),
    });
    // Debug is off: only the currently accepted signed credential can read.
    if (request.headers.get("Authorization") !== `Bearer ${accepted}`) {
      return Response.json(
        { error: { code: "DEBUG_ACCESS_FORBIDDEN", message: "debug is off" } },
        { status: 403 },
      );
    }
    if (path === "") return Response.json({ name: peer.label });
    if (path === "/runtime")
      return Response.json({
        online: true,
        debug_mode: "off",
        last_seen_at: new Date(time).toISOString(),
      });
    if (path === "/status") return Response.json({ activity: "idle" });
    if (path.endsWith("/latest")) {
      const field = path.split("/")[2];
      return Response.json({
        values: [{ field, value: 50, observed_at_unix_ms: time }],
      });
    }
    if (path === "/telemetry") {
      const field = new URL(request.url).searchParams.get("field");
      return Response.json({
        field,
        step_ms: 1000,
        points: [
          {
            observed_at_unix_ms: time,
            value: field === "gnss.longitude" ? 12 : 51,
          },
        ],
      });
    }
    if (path === "/workspaces")
      return Response.json([
        {
          id: "signed-workspace",
          name: "Signed workspace",
          available: true,
          system: false,
          last_active_at: new Date(time).toISOString(),
        },
      ]);
    if (path.endsWith("/audio.ogg"))
      return new Response("OggS signed audio", {
        headers: { "Content-Type": "audio/ogg" },
      });
    if (path.endsWith("/history"))
      return Response.json({
        available: true,
        items: [
          {
            name: "signed-entry",
            actor_name: "assistant",
            type: "agent",
            text: "Signed history",
            created_at: new Date(time).toISOString(),
            replay_available: true,
            audio_available: true,
          },
        ],
        has_next: false,
      });
    if (path === "/logs/search")
      return Response.json({
        items: [
          {
            time_ms: time,
            level: "INFO",
            message: "Signed log",
            source: "fixture",
            path: "fixture",
            fields: {},
          },
        ],
        end: { has_next: false },
      });
    if (path === "/mhs/v0/manifest")
      return Response.json({
        devices: [
          { id: "wifi.main", hwd: "wifi" },
          { id: "modem.main", hwd: "modem" },
          { id: "nfc.left", hwd: "nfc" },
        ],
      });
    if (path === "/mhs/v0/read") {
      const body = (await request.json()) as { id: string; hwd: string };
      const value =
        body.hwd === "wifi"
          ? { connected: true, ssid: "Signed Wi-Fi" }
          : { enabled: false };
      return Response.json({ ...body, value });
    }
    if (path === "/tool/v0/tools")
      return Response.json({
        tools: ["usage.time.get", "management.password.get"],
      });
    if (path === "/tool/v0/invoke") {
      const body = (await request.json()) as { tool: string };
      const results: Record<string, unknown> = {
        "wifi.saved.list": { networks: [{ ssid: "Signed Wi-Fi" }] },
        "usage.time.get": {
          state: { limit_seconds: 0, used_seconds: 0, exhausted: false },
        },
        "management.password.get": { is_set: false },
      };
      if (!Object.hasOwn(results, body.tool))
        throw new Error(`unexpected tool: ${body.tool}`);
      return Response.json({ result: results[body.tool] });
    }
    throw new Error(`unexpected Monitor request: ${request.method} ${path}`);
  });
  return {
    peer,
    requests,
    fetch,
    rotate() {
      accepted = signedBearer("replacement");
      return { ...peer, signedBearer: accepted };
    },
  };
}

function signedBearer(signature: string): string {
  // SDK format fixture. Cryptographic verification is covered by the real
  // Server/Edge Giztest, which mints tokens with NewSignedBearerToken.
  return `gizclaw_sig_v1_${btoa(JSON.stringify({ target: "signed-device" })).replaceAll("=", "")}.${signature}`;
}
