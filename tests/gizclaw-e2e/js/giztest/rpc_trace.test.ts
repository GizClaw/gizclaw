import assert from "node:assert/strict";
import test from "node:test";
import {
  GIZCLAW_SERVICE_PEER_RPC,
  giznetServiceDataChannelLabel,
} from "@gizclaw/gizclaw";

import { tracePeerRPCChannels } from "./rpc_trace.ts";

test("RPC diagnostics stay bounded and exclude message contents", () => {
  const pc = new EventTarget();
  const events = tracePeerRPCChannels(pc as unknown as RTCPeerConnection);
  const channel = Object.assign(new EventTarget(), {
    id: 3,
    label: giznetServiceDataChannelLabel(GIZCLAW_SERVICE_PEER_RPC),
    readyState: "open",
  });
  pc.dispatchEvent(Object.assign(new Event("datachannel"), { channel }));
  for (let i = 0; i < 100; i++) {
    channel.dispatchEvent(
      new MessageEvent("message", {
        data: new TextEncoder().encode("password must stay private"),
      }),
    );
  }
  assert.equal(events.length, 64);
  assert.equal(events.at(-1)?.bytes, 26);
  assert.ok(events.every((event) => event.channel_id === 3));
  assert.ok(!JSON.stringify(events).includes("password"));
});

test("RPC diagnostics ignore unrelated service channels", () => {
  const pc = new EventTarget();
  const events = tracePeerRPCChannels(pc as unknown as RTCPeerConnection);
  const channel = Object.assign(new EventTarget(), { label: "unrelated" });
  pc.dispatchEvent(Object.assign(new Event("datachannel"), { channel }));
  channel.dispatchEvent(new MessageEvent("message", { data: "private" }));
  assert.deepEqual(events, []);
});
