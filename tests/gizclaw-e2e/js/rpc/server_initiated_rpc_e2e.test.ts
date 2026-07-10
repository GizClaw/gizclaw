import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { randomBytes } from "node:crypto";
import type { Readable } from "node:stream";

import { connectGiznetWebRTCFromEndpoint } from "@gizclaw/gizclaw";
import wrtc from "@roamhq/wrtc";
import { closePeerConnection, repoRoot } from "../common/webrtc.ts";

async function main(): Promise<void> {
  const probe = spawn("go", ["run", "./tests/gizclaw-e2e/cmd/serverrpcprobe"], {
    cwd: repoRoot,
    stdio: ["ignore", "pipe", "pipe"],
  });
  let stderr = "";
  probe.stderr.setEncoding("utf8");
  probe.stderr.on("data", (chunk: string) => {
    stderr += chunk;
  });
  const lines = new LineReader(probe.stdout);
  const exit = new Promise<{ code: number | null; signal: NodeJS.Signals | null }>((resolve) => {
    probe.once("exit", (code, signal) => resolve({ code, signal }));
  });
  let pc: wrtc.RTCPeerConnection | undefined;
  try {
    const ready = JSON.parse(await nextProbeLine(lines, exit, () => stderr)) as { endpoint?: string; public_key?: string };
    assert.equal(typeof ready.endpoint, "string");
    assert.equal(typeof ready.public_key, "string");

    for (const name of ["ping", "zero", "upload-only", "download-only", "full-duplex"]) {
      const started = JSON.parse(await nextProbeLine(lines, exit, () => stderr)) as { case?: string };
      assert.equal(started.case, name);
      pc = new wrtc.RTCPeerConnection();
      await connectGiznetWebRTCFromEndpoint({
        clientPrivateKey: new Uint8Array(randomBytes(32)),
        endpoint: ready.endpoint,
        pc: pc as unknown as RTCPeerConnection,
      });
      const completed = JSON.parse(await nextProbeLine(lines, exit, () => stderr)) as { case?: string; ok?: boolean };
      assert.deepEqual(completed, { case: name, ok: true });
      closePeerConnection(pc);
      pc = undefined;
    }

    const result = JSON.parse(await nextProbeLine(lines, exit, () => stderr)) as { ok?: boolean };
    assert.equal(result.ok, true);
    const status = await exit;
    assert.equal(status.code, 0, `server RPC probe failed (${status.signal ?? status.code}): ${stderr}`);
  } finally {
    if (pc != null) {
      closePeerConnection(pc);
    }
    if (probe.exitCode == null && probe.signalCode == null) {
      probe.kill("SIGTERM");
      await exit;
    }
  }
}

async function nextProbeLine(
  lines: LineReader,
  exit: Promise<{ code: number | null; signal: NodeJS.Signals | null }>,
  stderr: () => string,
): Promise<string> {
  return Promise.race([
    lines.next(30_000),
    exit.then((status) => {
      throw new Error(`server RPC probe exited before completing (${status.signal ?? status.code}): ${stderr()}`);
    }),
  ]);
}

class LineReader {
  private buffer = "";
  private readonly lines: string[] = [];
  private readonly waiters: Array<(line: string) => void> = [];

  constructor(stream: Readable) {
    stream.setEncoding("utf8");
    stream.on("data", (chunk: string) => {
      this.buffer += chunk;
      for (;;) {
        const newline = this.buffer.indexOf("\n");
        if (newline < 0) {
          return;
        }
        const line = this.buffer.slice(0, newline);
        this.buffer = this.buffer.slice(newline + 1);
        const waiter = this.waiters.shift();
        if (waiter == null) {
          this.lines.push(line);
        } else {
          waiter(line);
        }
      }
    });
  }

  async next(timeoutMs: number): Promise<string> {
    const buffered = this.lines.shift();
    if (buffered != null) {
      return buffered;
    }
    let waiter: ((line: string) => void) | undefined;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const line = new Promise<string>((resolve) => {
      waiter = resolve;
      this.waiters.push(resolve);
    });
    const timeout = new Promise<never>((_, reject) => {
      timer = setTimeout(() => reject(new Error(`server RPC probe did not produce output within ${timeoutMs}ms`)), timeoutMs);
    });
    try {
      return await Promise.race([line, timeout]);
    } finally {
      if (timer != null) {
        clearTimeout(timer);
      }
      if (waiter != null) {
        const index = this.waiters.indexOf(waiter);
        if (index >= 0) {
          this.waiters.splice(index, 1);
        }
      }
    }
  }
}

main().then(
  () => {
    console.log("ok - Node WebRTC SDK serves server-initiated protobuf ping and speed-test RPC");
    process.exit(0);
  },
  (err: unknown) => {
    console.error(err);
    process.exit(1);
  },
);
