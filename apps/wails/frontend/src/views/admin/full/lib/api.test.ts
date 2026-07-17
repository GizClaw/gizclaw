import assert from "node:assert/strict";
import test from "node:test";

import { AdminPeerSessionManager } from "../../../../lib/gizclaw/admin.ts";
import { createRecoveringServiceFetch } from "./api.ts";

test("transport failure replaces the Admin session once and retries once", async () => {
  const first = new FakePeerConnection();
  const second = new FakePeerConnection();
  const connections = [first, second];
  let connectCalls = 0;
  const session = new AdminPeerSessionManager({
    connect: async () => connections[connectCalls++] as unknown as RTCPeerConnection,
  });
  await session.start();

  let firstCalls = 0;
  let secondCalls = 0;
  const recoveringFetch = createRecoveringServiceFetch(session, (connection) => {
    if (connection === (first as unknown as RTCPeerConnection)) {
      return async () => {
        firstCalls += 1;
        throw new Error("service data channel timed out");
      };
    }
    return async () => {
      secondCalls += 1;
      return new Response("ok");
    };
  });

  const responses = await Promise.all([recoveringFetch("http://gizclaw/a"), recoveringFetch("http://gizclaw/b")]);
  assert.deepEqual(await Promise.all(responses.map((response) => response.text())), ["ok", "ok"]);
  assert.equal(connectCalls, 2);
  assert.equal(firstCalls, 2);
  assert.equal(secondCalls, 2);
  assert.equal(first.closeCalls, 1);
});

test("HTTP responses and request cancellation do not recover the session", async () => {
  const connection = new FakePeerConnection();
  let connectCalls = 0;
  const session = new AdminPeerSessionManager({
    connect: async () => {
      connectCalls += 1;
      return connection as unknown as RTCPeerConnection;
    },
  });
  await session.start();

  const httpFetch = createRecoveringServiceFetch(session, () => async () => new Response("failed", { status: 503 }));
  assert.equal((await httpFetch("http://gizclaw/http-error")).status, 503);

  const aborted = new AbortController();
  aborted.abort();
  const abortFetch = createRecoveringServiceFetch(session, () => async () => {
    throw Object.assign(new Error("cancelled"), { name: "AbortError" });
  });
  await assert.rejects(abortFetch("http://gizclaw/aborted", { signal: aborted.signal }), { name: "AbortError" });
  assert.equal(connectCalls, 1);
});

test("late events and cleanup from an obsolete session cannot replace the active session", async () => {
  const first = new FakePeerConnection();
  const second = new FakePeerConnection();
  const connections = [first, second];
  let connectCalls = 0;
  const session = new AdminPeerSessionManager({
    connect: async () => connections[connectCalls++] as unknown as RTCPeerConnection,
    disconnectedGraceMS: 0,
  });
  await session.start();
  await session.recover(first as unknown as RTCPeerConnection);

  first.setState("failed");
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(await session.connection(), second as unknown as RTCPeerConnection);
  assert.equal(connectCalls, 2);
  assert.equal(second.closeCalls, 0);

  session.close();
  assert.equal(second.closeCalls, 1);
});

test("a failed retry becomes terminal without a third request", async () => {
  const first = new FakePeerConnection();
  const second = new FakePeerConnection();
  const connections = [first, second];
  const states: string[] = [];
  let connectCalls = 0;
  let fetchCalls = 0;
  const session = new AdminPeerSessionManager({
    connect: async () => connections[connectCalls++] as unknown as RTCPeerConnection,
    onState: (state) => states.push(state.status),
  });
  await session.start();
  const recoveringFetch = createRecoveringServiceFetch(session, () => async () => {
    fetchCalls += 1;
    throw new Error(`transport failure ${fetchCalls}`);
  });

  await assert.rejects(recoveringFetch("http://gizclaw/workflows"), /transport failure 2/);
  await assert.rejects(session.connection(), /transport failure 2/);
  assert.equal(connectCalls, 2);
  assert.equal(fetchCalls, 2);
  assert.equal(second.closeCalls, 1);
  assert.equal(states.at(-1), "failed");
});

test("closing the session aborts an in-flight replacement", async () => {
  const first = new FakePeerConnection();
  let connectCalls = 0;
  let replacementAborted = false;
  const session = new AdminPeerSessionManager({
    connect: async (signal) => {
      connectCalls += 1;
      if (connectCalls === 1) {
        return first as unknown as RTCPeerConnection;
      }
      return new Promise<RTCPeerConnection>((_resolve, reject) => {
        signal.addEventListener("abort", () => {
          replacementAborted = true;
          reject(Object.assign(new Error("aborted"), { name: "AbortError" }));
        });
      });
    },
  });
  await session.start();
  const recovery = session.recover(first as unknown as RTCPeerConnection);
  session.close();

  await assert.rejects(recovery, { name: "AbortError" });
  assert.equal(replacementAborted, true);
  assert.equal(first.closeCalls, 1);
});

class FakePeerConnection {
  connectionState: RTCPeerConnectionState = "connected";
  closeCalls = 0;
  readonly #listeners: Array<() => void> = [];

  addEventListener(type: string, listener: EventListenerOrEventListenerObject): void {
    if (type === "connectionstatechange") {
      this.#listeners.push(typeof listener === "function" ? () => listener(new Event(type)) : () => listener.handleEvent(new Event(type)));
    }
  }

  close(): void {
    this.closeCalls += 1;
    this.connectionState = "closed";
  }

  setState(state: RTCPeerConnectionState): void {
    this.connectionState = state;
    for (const listener of this.#listeners) {
      listener();
    }
  }
}
