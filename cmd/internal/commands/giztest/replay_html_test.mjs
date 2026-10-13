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
