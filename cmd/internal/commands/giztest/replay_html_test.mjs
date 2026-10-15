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
const settle = () => new Promise((resolve) => setImmediate(resolve));
class Element {
  constructor(tag) {
    this.tag = tag;
    this.children = [];
    this.listeners = new Map();
    this.value = "";
    this.textContent = "";
    this.classList = { toggle() {} };
  }
  append(...nodes) {
    this.children.push(...nodes);
  }
  setAttribute() {}
  addEventListener(name, callback) {
    this.listeners.set(name, callback);
  }
  fire(name, event) {
    this.listeners.get(name)?.(event);
  }
  scrollIntoView() {}
}
function player(events = [], overrides = {}, runtime = {}) {
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
      started_at: "2026-10-16T00:00:00Z",
      duration_ms: 4000,
      tasks: [task],
    },
    events: events.map((event) => ({
      path: task.path,
      task_id: task.task_id,
      step: "turn",
      role: "assistant",
      client: "peer",
      direction: "received",
      stream: 1,
      connection: 1,
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
      "overview",
      "speed",
      "seek",
      "clock",
      "conversation",
      "frame-count",
      "step-count",
      "frame-index",
      "frame-jump",
      "frame-state",
      "empty-conversation",
    ].map((id) => [id, new Element(id)]),
  );
  elements.get("recording-data").textContent = JSON.stringify(data);
  let now = 0,
    timerID = 0;
  const timers = new Map(),
    sounds = [],
    listeners = new Map(),
    contexts = [];
  class AudioContext {
    constructor() {
      this.destination = {};
      this.state = "running";
      contexts.push(this);
    }
    get currentTime() {
      return now / 1000;
    }
    async resume() {
      this.state = "running";
    }
    async close() {
      this.state = "closed";
    }
    decodeAudioData(bytes) {
      return runtime.decode
        ? runtime.decode(bytes)
        : Promise.resolve({ duration: 10 });
    }
    createBufferSource() {
      const sound = {
        playbackRate: { value: 1 },
        stopped: false,
        ended: false,
        connect() {},
        disconnect() {},
        start(when, offset, duration) {
          Object.assign(this, { when, offset, duration });
          sounds.push(this);
        },
        stop() {
          this.stopped = true;
          this.onended?.();
        },
      };
      return sound;
    }
  }
  const context = vm.createContext({
    document: {
      getElementById: (id) => elements.get(id),
      createElement: (tag) => new Element(tag),
    },
    window: {
      AudioContext,
      addEventListener: (name, callback) => listeners.set(name, callback),
    },
    atob: (text) => Buffer.from(text, "base64").toString("binary"),
    performance: { now: () => now },
    requestAnimationFrame: () => 1,
    cancelAnimationFrame() {},
    setTimeout: (callback) => {
      timers.set(++timerID, callback);
      return timerID;
    },
    clearTimeout: (id) => timers.delete(id),
  });
  const evaluate = (code) => vm.runInContext(code, context, { timeout: 5000 });
  vm.runInContext(source, context, { timeout: 5000 });
  return {
    data,
    elements,
    sounds,
    contexts,
    timers,
    evaluate,
    async click(id = "play") {
      elements.get(id).fire("click");
      await settle();
    },
    async seek(at) {
      elements.get("seek").value = String(at);
      elements.get("seek").fire("input");
      await settle();
    },
    async speed(value) {
      elements.get("speed").value = String(value);
      elements.get("speed").fire("change");
      await settle();
    },
    audible: () =>
      sounds.filter(
        (sound) =>
          !sound.stopped &&
          sound.when <= now / 1000 &&
          now / 1000 < sound.when + sound.duration / sound.playbackRate.value,
      ),
    advance(ms, render = true) {
      now += ms;
      sounds.forEach((sound) => {
        if (
          !sound.ended &&
          now / 1000 >= sound.when + sound.duration / sound.playbackRate.value
        ) {
          sound.ended = true;
          sound.onended?.();
        }
      });
      const callbacks = [...timers.values()];
      timers.clear();
      callbacks.forEach((callback) => callback());
      if (render) evaluate("tick()");
    },
    pagehide() {
      listeners.get("pagehide")();
    },
  };
}
function audio(overrides = {}) {
  return {
    kind: "audio",
    audio_data: wav(32000, 11),
    offset_ms: 500,
    duration_ms: 2000,
    audio_timing: [{ offset_ms: 500, duration_ms: 2000 }],
    ...overrides,
  };
}
const json = (ui, expression) =>
  JSON.parse(ui.evaluate(`JSON.stringify(${expression})`));

test("one numbered table shows steps, evidence and replies without links", () => {
  const ui = player([{ kind: "text", text: "你好 </script>", offset_ms: 100 }]);
  assert.equal(ui.elements.get("steps").children.length, 1);
  assert.equal(
    ui.elements.get("steps").children[0].children[0].textContent,
    "1",
  );
  ui.elements.get("overview").fire("click");
  assert.equal(ui.evaluate("messages[0].body.textContent"), "你好 </script>");
  assert.doesNotMatch(template, /<a\s|href=|src=/);
  assert.doesNotMatch(source, /fetch\s*\(|XMLHttpRequest|innerHTML/);
});

test("text, controls and audio use recorded event offsets on one clock", async () => {
  const ui = player([
    { kind: "text", text: "first", offset_ms: 100 },
    { kind: "text", text: " reply", offset_ms: 900 },
    audio(),
    { kind: "control", mime: "audio/opus", eos: true, offset_ms: 800 },
  ]);
  await ui.click();
  assert.equal(ui.evaluate("messages[0].body.textContent"), "");
  assert.equal(ui.audible().length, 0);
  ui.advance(600);
  assert.equal(ui.evaluate("messages[0].body.textContent"), "first");
  assert.equal(
    ui.evaluate("timelineEvents.some(e => e.at > cursor && !e.node.hidden)"),
    false,
  );
  assert.equal(ui.audible().length, 1);
  ui.advance(400);
  assert.equal(ui.evaluate("messages[0].body.textContent"), "first reply");
  assert.equal(ui.elements.get("clock").textContent, "0:01.000 / 0:04.000");
  assert.equal(
    ui.audible().length,
    1,
    "ordinary EOS must not discard queued samples",
  );
  await ui.click();
  assert.equal(ui.audible().length, 0);
});

test("an interruption stops queued sound at the event time without a rendered frame", async () => {
  const ui = player([
    audio(),
    { kind: "control", eos: true, error: "interrupted", offset_ms: 1200 },
  ]);
  await ui.click();
  assert.equal(ui.sounds[0].when, 0.5);
  assert.equal(ui.sounds[0].duration, 0.7);
  ui.advance(1100, false);
  assert.equal(ui.audible().length, 1);
  ui.advance(200, false);
  assert.equal(ui.audible().length, 0);
  ui.evaluate("tick()");
  assert.equal(ui.evaluate("cursor"), 1300);
  assert.equal(ui.evaluate("timelineEvents.at(-1).node.hidden"), false);
});

test("closing a transport cuts its streams and late packets while another connection continues", async () => {
  const ui = player([
    audio(),
    audio({ stream: 2 }),
    audio({ connection: 2, stream: 3 }),
    { kind: "control", action: "close", connection: 1, offset_ms: 900 },
    audio({
      offset_ms: 1000,
      audio_timing: [{ offset_ms: 1000, duration_ms: 2000 }],
    }),
    {
      kind: "control",
      action: "close",
      connection: 2,
      task_id: "another-task",
      offset_ms: 700,
    },
  ]);
  await ui.click();
  assert.equal(ui.sounds.length, 3);
  ui.advance(1000);
  assert.equal(ui.audible().length, 1);
  assert.equal(ui.evaluate("clips[3].button.disabled"), true);
});

test("a terminal event rebound to another step still stops its original stream only", async () => {
  const task = {
    path: "scene.yaml",
    task_id: "scene-0000",
    steps: [
      { id: "turn", operation: "peer_stream" },
      { id: "interrupt", operation: "peer_stream" },
    ],
  };
  const ui = player(
    [
      audio(),
      audio({ stream: 2 }),
      {
        kind: "control",
        step: "interrupt",
        eos: true,
        error: "interrupted",
        offset_ms: 900,
      },
    ],
    { report: { duration_ms: 4000, tasks: [task] } },
  );
  await ui.click();
  ui.advance(1000);
  assert.equal(ui.audible().length, 1);
});

test("packet arrival gaps remain silent and burst packets queue in media order", async () => {
  const ui = player([
    audio({
      audio_timing: [
        { offset_ms: 500, duration_ms: 500 },
        { offset_ms: 500, duration_ms: 500 },
        { offset_ms: 2000, duration_ms: 1000 },
      ],
    }),
    { kind: "text", text: "during gap", offset_ms: 1600 },
  ]);
  await ui.click();
  assert.deepEqual(json(ui, "clips[0].windows"), [
    { start: 500, end: 1500, media: 0 },
    { start: 2000, end: 3000, media: 1000 },
  ]);
  ui.advance(1750);
  assert.equal(ui.audible().length, 0);
  assert.equal(ui.evaluate("messages[0].body.textContent"), "during gap");
  ui.advance(500);
  assert.equal(ui.audible().length, 1);
  assert.equal(ui.sounds[1].when, 2);
  assert.equal(ui.sounds[1].offset, 1);
  assert.equal(ui.evaluate("cursor"), 2250);
});

test("seeking inside a packet gap waits for the next recorded arrival", async () => {
  const ui = player([
    audio({
      audio_timing: [
        { offset_ms: 500, duration_ms: 500 },
        { offset_ms: 2000, duration_ms: 1500 },
      ],
    }),
  ]);
  await ui.seek(1500);
  await ui.click();
  assert.equal(ui.sounds[0].when, 0.5);
  assert.equal(ui.sounds[0].offset, 0.5);
  assert.equal(ui.audible().length, 0);
  await ui.seek(2200);
  assert.equal(ui.sounds.at(-1).when, 0);
  assert.equal(ui.sounds.at(-1).offset, 0.7);
});

test("pause samples the current clock even between frames and resumes from the same sample", async () => {
  const ui = player([audio()]);
  await ui.click();
  ui.advance(1400, false);
  await ui.click();
  assert.equal(ui.evaluate("cursor"), 1400);
  assert.equal(ui.evaluate("playing"), false);
  ui.advance(1000);
  await ui.click();
  assert.equal(ui.sounds.at(-1).offset, 0.9);
  assert.equal(ui.sounds.at(-1).when, 2.4);
  assert.equal(ui.evaluate("cursor"), 1400);
  ui.advance(300);
  assert.equal(ui.evaluate("cursor"), 1700);
});

test("speed changes preserve elapsed event time and reschedule both sound and interruption", async () => {
  const ui = player([
    audio(),
    { kind: "control", eos: true, error: "interrupted", offset_ms: 1600 },
  ]);
  await ui.click();
  ui.advance(600, false);
  await ui.speed(2);
  assert.equal(ui.evaluate("cursor"), 600);
  const sound = ui.sounds.at(-1);
  assert.equal(sound.playbackRate.value, 2);
  assert.equal(sound.offset, 0.1);
  assert.equal(sound.duration, 1);
  ui.advance(400);
  assert.equal(ui.evaluate("cursor"), 1400);
  assert.equal(ui.audible().length, 1);
  ui.advance(100, false);
  assert.equal(ui.audible().length, 0);
});

test("segment playback seeks the event timeline and honors its interruption cutoff", async () => {
  const ui = player([
    audio(),
    { kind: "text", text: "opening", offset_ms: 100 },
    { kind: "control", eos: true, error: "interrupted", offset_ms: 1400 },
  ]);
  ui.evaluate("clips[0].button.fire('click')");
  await settle();
  assert.equal(ui.evaluate("cursor"), 500);
  assert.equal(ui.evaluate("messages[0].body.textContent"), "opening");
  assert.equal(ui.sounds[0].when, 0);
  ui.advance(900);
  assert.equal(ui.evaluate("cursor"), 1400);
  assert.equal(ui.evaluate("playing"), false);
  assert.equal(ui.audible().length, 0);
});

test("preparing media freezes the event clock and cancellation cannot restart an old run", async () => {
  let resolve;
  const deferred = new Promise((done) => {
    resolve = done;
  });
  const ui = player(
    [audio(), { kind: "text", text: "later", offset_ms: 100 }],
    {},
    { decode: () => deferred },
  );
  ui.elements.get("play").fire("click");
  await settle();
  ui.advance(3000);
  assert.equal(ui.evaluate("cursor"), 0);
  assert.equal(ui.sounds.length, 0);
  assert.equal(ui.evaluate("messages[0].body.textContent"), "");
  await ui.click("reset");
  resolve({ duration: 2 });
  await settle();
  assert.equal(ui.evaluate("playing"), false);
  assert.equal(ui.sounds.length, 0);
  await ui.click();
  assert.equal(ui.sounds[0].when, 3.5);
  ui.advance(100);
  assert.equal(ui.evaluate("cursor"), 100);
});

test("decode failure preserves the paused timeline and reports the affected step", async () => {
  const ui = player(
    [audio()],
    {},
    { decode: () => Promise.reject(new Error("bad media")) },
  );
  await ui.click();
  assert.equal(ui.evaluate("playing || loading"), false);
  assert.equal(ui.evaluate("cursor"), 0);
  assert.match(ui.elements.get("notice").textContent, /turn.*bad media/);
  assert.equal(ui.sounds.length, 0);
});

test("prepared speech is available as a segment and is not scheduled in the full replay", async () => {
  const ui = player([audio({ direction: "prepared" })]);
  await ui.click();
  assert.equal(ui.sounds.length, 0);
  assert.equal(ui.contexts.length, 0);
  ui.evaluate("clips[0].button.fire('click')");
  await settle();
  assert.equal(ui.sounds.length, 1);
});

test("text-only replay needs no audio context and retains startup offsets", async () => {
  const task = {
    path: "scene.yaml",
    task_id: "scene-0000",
    actual_start_offset_ms: 1000,
    duration_ms: 2000,
    steps: [{ id: "turn", operation: "peer_stream", start_offset_ms: 1100 }],
  };
  const ui = player([{ kind: "text", text: "delayed", offset_ms: 1200 }], {
    report: { duration_ms: 4000, tasks: [task] },
  });
  await ui.click();
  ui.advance(1190);
  assert.equal(ui.evaluate("rows[0].offset"), 1100);
  assert.equal(ui.evaluate("messages[0].body.textContent"), "");
  ui.advance(10);
  assert.equal(ui.evaluate("messages[0].body.textContent"), "delayed");
  assert.equal(ui.contexts.length, 0);
  assert.equal(ui.elements.get("clock").textContent, "0:01.200 / 0:04.000");
});

test("pagehide stops scheduled audio, closes its context and cancels pumping", async () => {
  const ui = player([audio()]);
  await ui.click();
  ui.advance(600);
  ui.pagehide();
  await settle();
  assert.equal(ui.audible().length, 0);
  assert.equal(ui.contexts[0].state, "closed");
  assert.equal(ui.timers.size, 0);
});

test("old recordings use known event times and explicitly disclose missing timing evidence", async () => {
  const ui = player([
    audio({ audio_timing: undefined, connection: undefined }),
  ]);
  assert.match(ui.elements.get("notice").textContent, /旧记录.*未保存/);
  await ui.click();
  assert.equal(ui.sounds[0].when, 0.5);
});

test("timestamps round across minute boundaries", () => {
  const ui = player();
  assert.equal(ui.evaluate("fmt(59999.99)"), "1:00.000");
  assert.equal(ui.evaluate("fmt(60000)"), "1:00.000");
});
test("forwarded reply audio is referenced and does not play twice", async () => {
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
  await ui.click();
  ui.advance(1600);
  assert.equal(ui.audible().length, 0);
  assert.equal(ui.sounds.length, 1);
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

test("audio split by a capture rebind queues on the same stream instead of overlapping", async () => {
  const ui = player([
    audio({
      duration_ms: 1000,
      audio_timing: [{ offset_ms: 500, duration_ms: 1000 }],
    }),
    audio({
      offset_ms: 800,
      duration_ms: 1000,
      audio_timing: [{ offset_ms: 800, duration_ms: 1000 }],
    }),
  ]);
  await ui.click();
  ui.advance(1000);
  assert.equal(ui.audible().length, 1);
  ui.advance(600);
  assert.equal(ui.audible().length, 1);
  assert.equal(ui.sounds[1].when, 1.5);
});

test("late data on an interrupted route stays silent until a new BOS", async () => {
  const ui = player([
    audio({
      duration_ms: 500,
      audio_timing: [{ offset_ms: 500, duration_ms: 500 }],
    }),
    { kind: "control", eos: true, error: "interrupted", offset_ms: 700 },
    audio({
      offset_ms: 800,
      duration_ms: 300,
      audio_timing: [{ offset_ms: 800, duration_ms: 300 }],
    }),
    { kind: "control", bos: true, offset_ms: 1200 },
    audio({
      offset_ms: 1300,
      duration_ms: 300,
      audio_timing: [{ offset_ms: 1300, duration_ms: 300 }],
    }),
  ]);
  await ui.click();
  ui.advance(900);
  assert.equal(ui.audible().length, 0);
  ui.advance(500);
  assert.equal(ui.audible().length, 1);
  assert.equal(ui.sounds.length, 2);
});

test("a terminal error attached to text is shown and stops the same route", async () => {
  const ui = player([
    audio(),
    {
      kind: "text",
      text: "partial reply",
      error: "interrupted",
      offset_ms: 800,
    },
  ]);
  await ui.click();
  ui.advance(900);
  assert.equal(ui.audible().length, 0);
  assert.equal(ui.evaluate("messages[0].body.textContent"), "partial reply");
  assert.match(
    ui.evaluate("timelineEvents.at(-1).node.textContent"),
    /发言被打断/,
  );
});

test("auditioning prepared media past report end keeps the displayed clock and slider consistent", async () => {
  const ui = player([
    audio({
      direction: "prepared",
      offset_ms: 3500,
      audio_timing: [{ offset_ms: 3500, duration_ms: 2000 }],
    }),
  ]);
  ui.evaluate("clips[0].button.fire('click')");
  await settle();
  ui.advance(2000);
  assert.equal(ui.evaluate("cursor"), 5500);
  assert.equal(ui.elements.get("clock").textContent, "0:05.500 / 0:05.500");
  assert.equal(ui.elements.get("seek").max, "5500");
});

function delayedReplyPlayer() {
  return player(
    [
      {
        kind: "control",
        direction: "sent",
        role: "user",
        eos: true,
        offset_ms: 22776.675333,
      },
      {
        kind: "control",
        role: "transcript",
        mime: "text/plain",
        eos: true,
        offset_ms: 27072.004833,
      },
      {
        kind: "control",
        mime: "text/plain",
        bos: true,
        offset_ms: 27072.010458,
      },
      { kind: "text", text: "first words", offset_ms: 39152.81 },
      audio({
        offset_ms: 39589.192,
        audio_timing: [{ offset_ms: 39589.192, duration_ms: 2000 }],
      }),
      { kind: "text", text: " later words", offset_ms: 40000 },
    ],
    {
      report: {
        status: "passed",
        duration_ms: 78000,
        tasks: [
          {
            path: "scene.yaml",
            task_id: "scene-0000",
            duration_ms: 78000,
            steps: [
              {
                id: "turn",
                operation: "peer_stream",
                start_offset_ms: 22010.239583,
                duration_ms: 55000,
              },
            ],
          },
        ],
      },
    },
  );
}

test("step start, first reply and recognition waits are explicitly labeled on the same clock", () => {
  const ui = delayedReplyPlayer();
  assert.deepEqual(
    json(ui, "rows[0].node.children[2].children.map(node => node.textContent)"),
    [
      "开始 0:22.010",
      "持续 55.000 s",
      "首字 0:39.153",
      "首音 0:39.589",
      "识别结束 0:27.072",
      "识别结束→首字 12.081 s",
      "识别结束→首音 12.517 s",
    ],
  );
  assert.equal(
    ui.evaluate(
      "timelineEvents.some(e => e.node.textContent.includes('文字流建立（不代表首字）'))",
    ),
    true,
  );
  assert.equal(
    ui.evaluate(
      "timelineEvents.some(e => e.node.textContent.includes('识别结束'))",
    ),
    true,
  );
});

test("at 38 seconds a 39.153-second reply is hidden, then reveals only arrived deltas", async () => {
  const ui = delayedReplyPlayer();
  await ui.seek(38000);
  assert.equal(ui.elements.get("clock").textContent, "0:38.000 / 1:18.000");
  assert.equal(ui.evaluate("messages[0].body.textContent"), "");
  assert.equal(
    ui.evaluate("timelineEvents.some(e => e.at > cursor && !e.node.hidden)"),
    false,
  );
  await ui.seek(39153);
  assert.equal(ui.evaluate("messages[0].body.textContent"), "first words");
  assert.equal(
    ui.evaluate(
      "timelineEvents.filter(e => e.node.className === 'audio')[0].node.hidden",
    ),
    true,
  );
  await ui.seek(40000);
  assert.equal(
    ui.evaluate("messages[0].body.textContent"),
    "first words later words",
  );
  assert.match(template, /\[hidden\]\{display:none!important\}/);
});

test("complete-record view is explicit and reset hides future content again", async () => {
  const ui = delayedReplyPlayer();
  assert.equal(ui.evaluate("messages[0].body.textContent"), "");
  await ui.seek(38000);
  await ui.click("overview");
  assert.equal(
    ui.evaluate("messages[0].body.textContent"),
    "first words later words",
  );
  assert.match(ui.elements.get("clock").textContent, /^完整记录 · 0:38\.000/);
  await ui.click("overview");
  assert.equal(ui.evaluate("messages[0].body.textContent"), "");
  assert.equal(ui.elements.get("clock").textContent, "0:38.000 / 1:18.000");
  await ui.click("overview");
  await ui.click("reset");
  assert.equal(ui.evaluate("messages[0].body.textContent"), "");
  assert.equal(ui.elements.get("clock").textContent, "0:00.000 / 1:18.000");
});

const frameText = (ui) =>
  json(
    ui,
    "conversationFrames.map(entry => messages.filter(message => message.entry === entry).map(message => message.body.textContent).join(''))",
  );

test("dialogue frames have stable numbers and locating a frame seeks its real time", async () => {
  const ui = player([
    { kind: "text", text: "first", offset_ms: 100 },
    { kind: "text", stream: 2, text: "second", offset_ms: 2500 },
  ]);
  const ids = json(ui, "conversationFrames.map(entry => entry.id)");
  assert.deepEqual(ids, ["#0001", "#0002"]);
  ui.elements.get("frame-index").value = "2";
  await ui.click("frame-jump");
  assert.equal(ui.evaluate("cursor"), 2500);
  assert.deepEqual(frameText(ui), ["first", "second"]);
  await ui.seek(50);
  await ui.click("overview");
  await ui.click("reset");
  assert.deepEqual(json(ui, "conversationFrames.map(entry => entry.id)"), ids);
  ui.elements.get("frame-index").value = "90";
  await ui.click("frame-jump");
  assert.equal(ui.evaluate("cursor"), 0);
  assert.match(ui.elements.get("notice").textContent, /1 到 2/);
});

test("forwarded audio and its ASR are references while the receiver's own reply remains a frame", async () => {
  const a = wav(16000, 11),
    b = wav(16000, 22);
  const ui = player([
    { kind: "text", client: "host", text: "这是一条新闻。", offset_ms: 100 },
    audio({
      client: "host",
      offset_ms: 200,
      duration_ms: 1000,
      audio_data: a,
      audio_timing: [{ offset_ms: 200, duration_ms: 1000 }],
    }),
    audio({
      client: "listener",
      direction: "sent",
      role: "user",
      stream: 2,
      offset_ms: 2000,
      duration_ms: 1000,
      audio_data: a,
      audio_timing: [{ offset_ms: 2000, duration_ms: 1000 }],
    }),
    {
      kind: "text",
      client: "listener",
      role: "transcript",
      stream: 3,
      text: "这是一条新闻。",
      offset_ms: 2400,
    },
    {
      kind: "text",
      client: "listener",
      stream: 4,
      text: "来源是哪家媒体？",
      offset_ms: 3500,
    },
    audio({
      client: "listener",
      stream: 4,
      offset_ms: 3600,
      duration_ms: 1000,
      audio_data: b,
      audio_timing: [{ offset_ms: 3600, duration_ms: 1000 }],
    }),
  ]);
  await ui.click("overview");
  assert.deepEqual(frameText(ui), ["这是一条新闻。", "来源是哪家媒体？"]);
  assert.equal(ui.evaluate("clips.length"), 2);
  assert.equal(ui.evaluate("conversationFrames[1].at"), 3500);
  assert.equal(ui.evaluate("conversationFrames[0].end"), 1200);
  assert.equal(
    ui.evaluate(
      "referenceNotes.some(note => note.node.textContent.includes('#0001'))",
    ),
    true,
  );
  await ui.seek(3000);
  assert.equal(
    ui.evaluate("conversationFrames[1].node.hidden"),
    true,
    "the real wait remains",
  );
});

test("streamed forwarded text is one utterance and two independent identical answers remain separate", async () => {
  const ui = player([
    { kind: "text", client: "host", text: "你好", offset_ms: 100 },
    {
      kind: "text",
      client: "listener",
      direction: "sent",
      role: "user",
      stream: 2,
      text: "你好",
      offset_ms: 110,
    },
    { kind: "text", client: "host", text: "世界", offset_ms: 200 },
    {
      kind: "text",
      client: "listener",
      direction: "sent",
      role: "user",
      stream: 2,
      text: "世界",
      offset_ms: 210,
    },
    {
      kind: "text",
      client: "listener",
      role: "transcript",
      stream: 3,
      text: "你好世界",
      offset_ms: 300,
    },
    {
      kind: "text",
      client: "listener",
      stream: 4,
      text: "明白",
      offset_ms: 1000,
    },
    {
      kind: "text",
      client: "listener",
      stream: 5,
      text: "明白",
      offset_ms: 2000,
    },
  ]);
  await ui.click("overview");
  assert.deepEqual(frameText(ui), ["你好世界", "明白", "明白"]);
});

test("an ordinary user's audio and ASR share one input frame without hiding the recognized text", async () => {
  const ui = player([
    audio({ direction: "sent", role: "user" }),
    {
      kind: "text",
      role: "transcript",
      stream: 2,
      text: "请讲一下新闻",
      offset_ms: 1400,
    },
  ]);
  await ui.click("overview");
  assert.equal(ui.evaluate("conversationFrames.length"), 1);
  assert.deepEqual(frameText(ui), ["请讲一下新闻"]);
});

test("startup probe text uses the verified audio-copy relation, including a sibling ASR", async () => {
  const d = structuredClone(parallelAudio(wav(320, 11), wav(16000, 11)).data);
  const [probe, full] = d.events;
  probe.stream = 9;
  full.stream = 10;
  d.events.push(
    {
      ...probe,
      kind: "text",
      audio_data: undefined,
      text: "今天",
      offset_ms: 450,
    },
    {
      ...full,
      kind: "text",
      audio_data: undefined,
      text: "今天新闻",
      offset_ms: 450.001,
    },
  );
  const ui = player([], d);
  await ui.click("overview");
  assert.equal(ui.evaluate("conversationFrames.length"), 1);
  assert.deepEqual(frameText(ui), ["今天新闻"]);
});

test("raw JSON and technical steps are collapsed and prepared media is outside the dialogue", () => {
  const ui = player([audio({ direction: "prepared" })]);
  assert.equal(ui.evaluate("conversationFrames.length"), 0);
  assert.match(template, /<details id="technical" class="technical">/);
  assert.doesNotMatch(template, /<details[^>]*\sopen(?:\s|>)/);
  assert.match(source, /el\("details", "raw-data"\)/);
});

test("reusing a completed stream ID keeps two independent replies as separate numbered frames", async () => {
  const ui = player([
    { kind: "text", text: "第一句", offset_ms: 100 },
    { kind: "control", mime: "text/plain", eos: true, offset_ms: 200 },
    { kind: "control", mime: "text/plain", bos: true, offset_ms: 2000 },
    { kind: "text", text: "第二句", offset_ms: 2200 },
  ]);
  await ui.click("overview");
  assert.deepEqual(frameText(ui), ["第一句", "第二句"]);
  assert.deepEqual(json(ui, "conversationFrames.map(entry => entry.id)"), [
    "#0001",
    "#0002",
  ]);
});

test("an ASR route keeps its original input owner when another forwarded input arrives", async () => {
  const sound = wav(16000, 11);
  const ui = player([
    { kind: "text", client: "host", text: "新闻。", offset_ms: 50 },
    audio({
      client: "host",
      audio_data: sound,
      offset_ms: 100,
      audio_timing: [{ offset_ms: 100, duration_ms: 1000 }],
      duration_ms: 1000,
    }),
    audio({
      client: "listener",
      role: "user",
      direction: "sent",
      stream: 2,
      offset_ms: 300,
      audio_data: wav(16000, 22),
      audio_timing: [{ offset_ms: 300, duration_ms: 1000 }],
      duration_ms: 1000,
    }),
    {
      kind: "control",
      client: "listener",
      role: "transcript",
      stream: 3,
      mime: "text/plain",
      bos: true,
      offset_ms: 400,
    },
    audio({
      client: "listener",
      role: "user",
      direction: "sent",
      stream: 4,
      offset_ms: 2000,
      audio_data: sound,
      audio_timing: [{ offset_ms: 2000, duration_ms: 1000 }],
      duration_ms: 1000,
    }),
    {
      kind: "text",
      client: "listener",
      role: "transcript",
      stream: 3,
      text: "独立的用户输入",
      offset_ms: 2500,
    },
  ]);
  await ui.click("overview");
  assert.deepEqual(frameText(ui), ["新闻。", "独立的用户输入"]);
});
