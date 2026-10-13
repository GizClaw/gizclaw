import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

const source = readFileSync(
  new URL("./recording_player.js", import.meta.url),
  "utf8",
);
const template = readFileSync(
  new URL("./recording.gohtml", import.meta.url),
  "utf8",
);
class Element {
  constructor(tag) {
    this.tag = tag;
    this.children = [];
    this.listeners = new Map();
    this.value = "";
    this.textContent = "";
    this.paused = true;
    this.classList = { toggle() {} };
  }
  append(...nodes) {
    this.children.push(...nodes);
  }
  setAttribute() {}
  addEventListener(name, callback) {
    this.listeners.set(name, callback);
  }
  fire(name) {
    this.listeners.get(name)?.();
  }
  scrollIntoView() {}
  pause() {
    this.paused = true;
  }
  play() {
    this.paused = false;
    return Promise.resolve();
  }
}
function player(events = [], overrides = {}) {
  const task = {
    path: "scene.yaml",
    task_id: "scene-0000",
    name: "scene",
    repeat_index: 0,
    status: "passed",
    duration_ms: 4000,
    steps: [
      {
        id: "turn",
        operation: "peer_stream",
        status: "passed",
        start_offset_ms: 0,
        duration_ms: 3000,
      },
    ],
  };
  const data = {
    report: {
      status: "passed",
      started_at: "2026-10-13T00:00:00Z",
      duration_ms: 4000,
      tasks: [task],
    },
    events: events.map((event) => ({
      path: task.path,
      task_id: task.task_id,
      step: "turn",
      role: "assistant",
      direction: "received",
      stream: 1,
      ...event,
    })),
    ...overrides,
  };
  const elements = new Map(
    [
      "recording-data",
      "summary",
      "steps",
      "notice",
      "play",
      "reset",
      "speed",
      "seek",
      "clock",
    ].map((id) => [id, new Element(id)]),
  );
  elements.get("recording-data").textContent = JSON.stringify(data);
  const context = vm.createContext({
    atob,
    document: {
      getElementById: (id) => elements.get(id),
      createElement: (tag) => new Element(tag),
    },
    window: { addEventListener() {} },
    performance: { now: () => 0 },
    requestAnimationFrame: () => 1,
    cancelAnimationFrame() {},
  });
  vm.runInContext(source, context, { timeout: 5000 });
  return {
    elements,
    evaluate: (code) => vm.runInContext(code, context, { timeout: 5000 }),
  };
}

test("one numbered table shows steps, evidence and replies without links", () => {
  const ui = player([{ kind: "text", text: "你好 </script>", offset_ms: 100 }]);
  assert.equal(ui.elements.get("steps").children.length, 1);
  assert.equal(
    ui.elements.get("steps").children[0].children[0].textContent,
    "1",
  );
  assert.equal(ui.evaluate("messages[0].body.textContent"), "你好 </script>");
  assert.doesNotMatch(template, /<a\s|href=|src=/);
  assert.doesNotMatch(source, /fetch\s*\(|XMLHttpRequest|innerHTML/);
});

test("forwarded reply audio is referenced and does not play twice", () => {
  const audio = "data:audio/wav;base64,UklGRg==";
  const ui = player([
    {
      kind: "audio",
      client: "host",
      audio_data: audio,
      offset_ms: 100,
      duration_ms: 1000,
    },
    {
      kind: "audio",
      client: "listener",
      direction: "sent",
      role: "user",
      audio_data: audio,
      offset_ms: 1500,
      duration_ms: 1000,
    },
  ]);
  assert.equal(ui.evaluate("clips.length"), 1);
  assert.equal(ui.evaluate("audioCopies.size"), 1);
  assert.equal(ui.evaluate("data.events.length"), 2);
  ui.elements.get("play").fire("click");
  ui.evaluate("tick(1600)");
  assert.equal(ui.evaluate("clips.some(clip=>!clip.node.paused)"), false);
});

test("independent replies and same-client inputs remain audible", () => {
  const audio = "data:audio/wav;base64,UklGRg==";
  const ui = player([
    {
      kind: "audio",
      client: "host",
      audio_data: audio,
      offset_ms: 100,
      duration_ms: 100,
    },
    {
      kind: "audio",
      client: "host",
      audio_data: audio,
      offset_ms: 500,
      duration_ms: 100,
    },
    {
      kind: "audio",
      client: "host",
      direction: "sent",
      audio_data: audio,
      offset_ms: 800,
      duration_ms: 100,
    },
    {
      kind: "audio",
      client: "listener",
      direction: "sent",
      audio_data: "different",
      offset_ms: 1000,
      duration_ms: 100,
    },
  ]);
  assert.equal(ui.evaluate("clips.length"), 4);
  assert.equal(ui.evaluate("audioCopies.size"), 0);
});

test("a separate task never references another task's reply", () => {
  const task = (id) => ({
    path: "scene.yaml",
    task_id: id,
    name: "scene",
    status: "passed",
    duration_ms: 4000,
    steps: [{ id: "turn", operation: "peer_stream", status: "passed" }],
  });
  const audio = "data:audio/wav;base64,UklGRg==";
  const ui = player([], {
    report: {
      status: "failed",
      started_at: "2026-10-13T00:00:00Z",
      duration_ms: 4000,
      tasks: [task("a"), task("b")],
    },
    events: [
      {
        path: "scene.yaml",
        task_id: "a",
        step: "turn",
        kind: "audio",
        direction: "received",
        role: "assistant",
        client: "host",
        audio_data: audio,
        offset_ms: 100,
        duration_ms: 100,
      },
      {
        path: "scene.yaml",
        task_id: "b",
        step: "turn",
        kind: "audio",
        direction: "sent",
        role: "user",
        client: "listener",
        audio_data: audio,
        offset_ms: 800,
        duration_ms: 100,
      },
    ],
  });
  assert.equal(ui.evaluate("clips.length"), 2);
  assert.equal(ui.evaluate("audioCopies.size"), 0);
  assert.match(ui.elements.get("summary").textContent, /验收失败/);
});

function wav(samples, value) {
  const bytes = Buffer.alloc(44 + samples * 2, value);
  bytes.write("RIFF", 0);
  bytes.writeUInt32LE(bytes.length - 8, 4);
  bytes.write("WAVEfmt ", 8);
  bytes.writeUInt32LE(16, 16);
  bytes.writeUInt16LE(1, 20);
  bytes.writeUInt16LE(1, 22);
  bytes.writeUInt32LE(16000, 24);
  bytes.writeUInt32LE(32000, 28);
  bytes.writeUInt16LE(2, 32);
  bytes.writeUInt16LE(16, 34);
  bytes.write("data", 36);
  bytes.writeUInt32LE(samples * 2, 40);
  return "data:audio/wav;base64," + bytes.toString("base64");
}
function parallelAudio(
  probe,
  full,
  integrity = { open: 1, max_active: 1, violations: 0 },
) {
  const task = {
    path: "scene.yaml",
    task_id: "scene-0000",
    name: "scene",
    status: "passed",
    duration_ms: 4000,
    steps: [
      {
        id: "both",
        operation: "parallel",
        status: "passed",
        children: [
          {
            id: "probe",
            client: "host",
            operation: "peer_stream",
            status: "passed",
            evidence: { audio_integrity: integrity },
          },
          {
            id: "listen",
            client: "host",
            operation: "peer_stream",
            status: "passed",
            evidence: { mode: "listen" },
          },
        ],
      },
    ],
  };
  return player([], {
    report: {
      status: "passed",
      started_at: "2026-10-13T00:00:00Z",
      duration_ms: 4000,
      tasks: [task],
    },
    events: [
      {
        path: task.path,
        task_id: task.task_id,
        parent: "both",
        step: "probe",
        kind: "audio",
        direction: "received",
        role: "assistant",
        client: "host",
        audio_data: probe,
        offset_ms: 500,
        duration_ms: 20,
        packets: 1,
      },
      {
        path: task.path,
        task_id: task.task_id,
        parent: "both",
        step: "listen",
        kind: "audio",
        direction: "received",
        role: "assistant",
        client: "host",
        audio_data: full,
        offset_ms: 500.004,
        duration_ms: 1000,
        packets: 50,
      },
    ],
  });
}
test("the startup probe references the full recording of the same audio", () => {
  const ui = parallelAudio(wav(320, 11), wav(16000, 11));
  assert.equal(ui.evaluate("clips.length"), 1);
  assert.equal(ui.evaluate("clips[0].duration"), 1000);
  assert.equal(ui.evaluate("audioCopies.size"), 1);
});
test("different or overlapping streams are not hidden as startup copies", () => {
  for (const ui of [
    parallelAudio(wav(320, 11), wav(16000, 22)),
    parallelAudio(wav(320, 11), wav(16000, 11), {
      open: 2,
      max_active: 2,
      violations: 1,
    }),
    parallelAudio("malformed", "also-malformed"),
  ]) {
    assert.equal(ui.evaluate("clips.length"), 2);
    assert.equal(ui.evaluate("audioCopies.size"), 0);
  }
});

test("replay reveals replies and plays each audio segment when reached", () => {
  const ui = player([
    { kind: "text", text: "first", offset_ms: 100 },
    { kind: "text", text: " reply", offset_ms: 900 },
    {
      kind: "audio",
      audio_data: "data:audio/wav;base64,UklGRg==",
      offset_ms: 500,
      duration_ms: 2000,
    },
  ]);
  ui.elements.get("play").fire("click");
  assert.equal(ui.evaluate("messages[0].body.textContent"), "");
  ui.evaluate("tick(600)");
  assert.equal(ui.evaluate("messages[0].body.textContent"), "first");
  assert.equal(ui.evaluate("clips[0].node.paused"), false);
  assert.equal(
    ui.evaluate("clips[0].node.src"),
    "data:audio/wav;base64,UklGRg==",
  );
  ui.evaluate("tick(1000)");
  assert.equal(ui.evaluate("messages[0].body.textContent"), "first reply");
  ui.elements.get("play").fire("click");
  assert.equal(ui.evaluate("clips[0].node.paused"), true);
});

test("seeking and playback speed apply to inline segment audio", () => {
  const ui = player([
    {
      kind: "audio",
      audio_data: "data:audio/wav;base64,UklGRg==",
      offset_ms: 500,
      duration_ms: 2000,
    },
  ]);
  ui.elements.get("play").fire("click");
  ui.evaluate("tick(600)");
  ui.elements.get("speed").value = "2";
  ui.elements.get("speed").fire("change");
  assert.equal(ui.evaluate("clips[0].node.playbackRate"), 2);
  ui.elements.get("seek").value = "1500";
  ui.elements.get("seek").fire("input");
  ui.evaluate("tick(0)");
  assert.equal(ui.evaluate("clips[0].node.currentTime"), 1);
});

test("parallel children have step sequence numbers and their own content", () => {
  const task = {
    path: "scene.yaml",
    task_id: "scene-0000",
    name: "scene",
    repeat_index: 0,
    status: "passed",
    steps: [
      {
        id: "both",
        operation: "parallel",
        status: "passed",
        children: [
          { id: "left", operation: "peer_stream", status: "passed" },
          { id: "right", operation: "peer_stream", status: "passed" },
        ],
      },
    ],
  };
  const ui = player([], {
    report: {
      status: "passed",
      started_at: "2026-10-13T00:00:00Z",
      duration_ms: 4000,
      tasks: [task],
    },
    events: [
      {
        path: task.path,
        task_id: task.task_id,
        step: "left",
        parent: "both",
        kind: "text",
        text: "left reply",
        offset_ms: 100,
      },
    ],
  });
  assert.deepEqual(
    Array.from(ui.evaluate("rows.map(row=>row.node.children[0].textContent)")),
    ["1", "1.1", "1.2"],
  );
  assert.equal(ui.evaluate("rows[1].messages.size"), 1);
  assert.equal(ui.evaluate("rows[2].messages.size"), 0);
});

test("a previous canceled audio promise cannot stop the new playback", async () => {
  const ui = player([
    {
      kind: "audio",
      audio_data: "data:audio/wav;base64,UklGRg==",
      offset_ms: 500,
      duration_ms: 2000,
    },
  ]);
  ui.evaluate("clips[0].node.play=()=>Promise.reject(new Error('canceled'))");
  ui.elements.get("play").fire("click");
  ui.evaluate("tick(600)");
  ui.elements.get("reset").fire("click");
  ui.evaluate("clips[0].node.play=()=>Promise.resolve()");
  ui.elements.get("play").fire("click");
  ui.evaluate("tick(600)");
  await Promise.resolve();
  assert.equal(ui.evaluate("playing"), true);
  assert.equal(ui.elements.get("notice").textContent, "");
});
