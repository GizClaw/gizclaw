import assert from "node:assert/strict";
import { execFile, spawn } from "node:child_process";
import { randomBytes } from "node:crypto";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { Readable } from "node:stream";
import { promisify } from "node:util";

import { connectGiznetWebRTCFromEndpoint } from "@gizclaw/gizclaw";
import { createPeerRPCClient } from "@gizclaw/gizclaw/rpc";
import wrtc from "@roamhq/wrtc";

import { closePeerConnection, repoRoot } from "../common/webrtc.ts";

async function main(): Promise<void> {
  const tempDirectory = await mkdtemp(
    join(tmpdir(), "gizclaw-serverrpcprobe-"),
  );
  const probePath = join(tempDirectory, "serverrpcprobe");
  try {
    await promisify(execFile)(
      "go",
      ["build", "-o", probePath, "./tests/gizclaw-e2e/cmd/serverrpcprobe"],
      { cwd: repoRoot },
    );
  } catch (err) {
    await rm(tempDirectory, { recursive: true, force: true });
    throw err;
  }
  const probe = spawn(probePath, ["--audio-downloads"], {
    cwd: repoRoot,
    stdio: ["ignore", "pipe", "pipe"],
  });
  let stderr = "";
  probe.stderr.setEncoding("utf8");
  probe.stderr.on("data", (chunk: string) => {
    stderr += chunk;
  });
  const lines = new LineReader(probe.stdout);
  const exit = new Promise<{
    code: number | null;
    signal: NodeJS.Signals | null;
  }>((resolve) => {
    probe.once("exit", (code, signal) => resolve({ code, signal }));
  });

  let pc: wrtc.RTCPeerConnection | undefined;
  try {
    const ready = JSON.parse(
      await nextProbeLine(lines, exit, () => stderr),
    ) as {
      endpoint?: string;
    };
    assert.equal(typeof ready.endpoint, "string");
    const endpoint = ready.endpoint;
    assert.ok(endpoint);

    pc = new wrtc.RTCPeerConnection();
    await connectGiznetWebRTCFromEndpoint({
      clientPrivateKey: new Uint8Array(randomBytes(32)),
      endpoint,
      pc: pc as unknown as RTCPeerConnection,
    });
    const rpc = createPeerRPCClient(pc as unknown as RTCPeerConnection);

    const workspace = await rpc.callBinary(
      "server.workspace.history.audio.download",
      { workspace_name: "workspace-a", history_name: "history-a" },
    );
    assert.deepEqual(workspace.result, {
      workspace_name: "workspace-a",
      history_name: "history-a",
      mime_type: "audio/ogg; codecs=opus",
      size_bytes: 23,
    });
    assert.equal(
      new TextDecoder().decode(workspace.body),
      "workspace-audio-payload",
    );

    assert.deepEqual(
      JSON.parse(await nextProbeLine(lines, exit, () => stderr)),
      { ok: true },
    );
    assert.deepEqual(await exit, { code: 0, signal: null });
  } finally {
    if (pc != null) closePeerConnection(pc);
    if (probe.exitCode == null && probe.signalCode == null) {
      probe.kill("SIGTERM");
      await exit;
    }
    await rm(tempDirectory, { recursive: true, force: true });
  }

  if (stderr !== "") {
    throw new Error(`audio download probe wrote stderr:\n${stderr}`);
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
      throw new Error(
        `audio download probe exited before completing (${status.signal ?? status.code}): ${stderr()}`,
      );
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
        if (newline < 0) return;
        const line = this.buffer.slice(0, newline);
        this.buffer = this.buffer.slice(newline + 1);
        const waiter = this.waiters.shift();
        if (waiter == null) this.lines.push(line);
        else waiter(line);
      }
    });
  }

  async next(timeoutMs: number): Promise<string> {
    const buffered = this.lines.shift();
    if (buffered != null) return buffered;

    let waiter: ((line: string) => void) | undefined;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const line = new Promise<string>((resolve) => {
      waiter = resolve;
      this.waiters.push(resolve);
    });
    const timeout = new Promise<never>((_, reject) => {
      timer = setTimeout(
        () =>
          reject(
            new Error(`audio download probe timed out after ${timeoutMs}ms`),
          ),
        timeoutMs,
      );
    });
    try {
      return await Promise.race([line, timeout]);
    } finally {
      if (timer != null) clearTimeout(timer);
      if (waiter != null) {
        const index = this.waiters.indexOf(waiter);
        if (index >= 0) this.waiters.splice(index, 1);
      }
    }
  }
}

main().then(
  () => {
    console.log("ok - JavaScript SDK downloads workspace History audio");
    process.exit(0);
  },
  (err: unknown) => {
    console.error(err);
    process.exit(1);
  },
);
