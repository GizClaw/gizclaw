"use strict";
const data = JSON.parse(document.getElementById("recording-data").textContent);
const $ = (id) => document.getElementById(id);
const el = (tag, cls, text) => {
  const node = document.createElement(tag);
  if (cls) node.className = cls;
  if (text !== undefined) node.textContent = text;
  return node;
};
const fmt = (ms) => {
  const ticks = Math.max(0, Math.round(ms));
  return `${Math.floor(ticks / 60000)}:${Math.floor((ticks % 60000) / 1000)
    .toString()
    .padStart(2, "0")}.${(ticks % 1000).toString().padStart(3, "0")}`;
};
const tasks = data.report.tasks || [];
// Event, step and packet offsets all use report.started_at. Preserve startup
// delays and parallel-task staggering instead of rebasing to the first task.
const rowKey = (path, task, cleanup, parent, step) =>
  JSON.stringify([path, task, !!cleanup, parent || "", step]);
const timingByRow = new Map();
(data.events || []).forEach((event) => {
  const key = rowKey(
    event.path,
    event.task_id,
    event.cleanup,
    event.parent,
    event.step,
  );
  if (!timingByRow.has(key)) timingByRow.set(key, { transcriptEnds: [] });
  const timing = timingByRow.get(key);
  if (event.direction !== "received") return;
  if (event.role === "transcript" && event.eos && !event.error)
    timing.transcriptEnds.push(event.offset_ms);
  if (event.role !== "assistant") return;
  if (event.kind === "text" && event.text && !event.interim)
    timing.text = Math.min(timing.text ?? Infinity, event.offset_ms);
  if (event.kind === "audio")
    timing.audio = Math.min(timing.audio ?? Infinity, event.offset_ms);
});
function stepTiming(task, step, cleanup, parent, offset) {
  const cell = el("td", "timing");
  cell.append(
    el("div", "", `开始 ${fmt(offset)}`),
    el("div", "meta", `持续 ${((step.duration_ms || 0) / 1000).toFixed(3)} s`),
  );
  const timing = timingByRow.get(
    rowKey(task.path, task.task_id, cleanup, parent, step.id),
  );
  if (!timing) return cell;
  if (timing.text !== undefined)
    cell.append(el("div", "meta", `首字 ${fmt(timing.text)}`));
  if (timing.audio !== undefined)
    cell.append(el("div", "meta", `首音 ${fmt(timing.audio)}`));
  const response = Math.min(timing.text ?? Infinity, timing.audio ?? Infinity);
  if (response !== Infinity) {
    const ends = timing.transcriptEnds.filter((at) => at <= response);
    if (ends.length) {
      const recognized = ends.reduce((latest, at) => Math.max(latest, at));
      cell.append(el("div", "meta", `识别结束 ${fmt(recognized)}`));
      for (const [name, at] of [
        ["首字", timing.text],
        ["首音", timing.audio],
      ]) {
        if (at !== undefined)
          cell.append(
            el(
              "div",
              "meta",
              `识别结束→${name} ${((at - recognized) / 1000).toFixed(3)} s`,
            ),
          );
      }
    }
  }
  return cell;
}
const rows = [],
  clips = [],
  messages = [],
  timelineEvents = [],
  rowMap = new Map(),
  conversationFrames = [],
  speechFrames = new Map(),
  speakers = new Map();
let number = 0,
  cursor = 0,
  duration = Math.max(1, data.report.duration_ms || 0),
  playing = false,
  loading = false,
  overview = false,
  origin = 0,
  started = 0,
  rate = 1,
  frame = 0,
  timer = 0,
  audioGeneration = 0,
  audioContext = null,
  segmentClip = null,
  lastFocus = null;
const warnings = [];
if (data.truncated) warnings.push("录制达到容量上限，以下为已保存的部分记录。");
if (data.errors?.length) warnings.push(data.errors.join("\n"));
const notice = (message) => {
  $("notice").textContent = [...warnings, message].filter(Boolean).join("\n");
};
notice("");
function appendEvent(row, node, at, parent = row.content) {
  parent.append(node);
  timelineEvents.push({ node, at });
}
function speechFrame(row, event, at) {
  const key = JSON.stringify([
    rowKey(event.path, event.task_id, event.cleanup, event.parent, event.step),
    event.client,
    event.connection,
    event.stream,
    event.role,
    event.direction,
    event.attempt,
    event.interim,
    event.segment,
  ]);
  let entry = speechFrames.get(key);
  if (entry) {
    entry.at = Math.min(entry.at, at);
    return entry;
  }
  const speaker =
    event.client || (event.role === "assistant" ? "Agent" : "听众");
  if (!speakers.has(speaker)) speakers.set(speaker, speakers.size);
  const node = el("article", `frame speaker-${speakers.get(speaker) % 4}`);
  const header = el("div", "frame-header");
  const badge = el("button", "frame-number");
  badge.type = "button";
  header.append(
    badge,
    el("span", "speaker", speaker),
    el(
      "span",
      "frame-direction",
      event.role === "event"
        ? "事件"
        : event.role === "assistant"
          ? "发言"
          : event.direction === "prepared"
            ? "准备的语音"
            : event.role === "transcript"
              ? "听到的内容"
              : "输入",
    ),
    el("span", "frame-time", fmt(at)),
  );
  const content = el("div", "frame-content");
  node.append(header, content);
  entry = { node, content, badge, at, end: at, row, speaker };
  speechFrames.set(key, entry);
  entry.preview =
    event.direction === "prepared" || event.direction === "output";
  if (entry.preview) {
    badge.hidden = true;
    row.content.append(node);
  } else conversationFrames.push(entry);
  badge.addEventListener("click", () => jumpToFrame(entry));
  return entry;
}
function jumpToFrame(entry) {
  pause();
  segmentClip = null;
  cursor = entry.at;
  update(true);
  entry.node.scrollIntoView({ block: "center", behavior: "smooth" });
}
$("summary").textContent =
  `${data.report.status === "passed" ? "验收通过" : "验收失败"} · ${tasks.length} 次执行 · ${fmt(data.report.duration_ms)} · ${new Date(data.report.started_at).toLocaleString()}`;

function appendSteps(task, steps, cleanup = false, parent = "", prefix = "") {
  (steps || []).forEach((step, index) => {
    const sequence = prefix ? `${prefix}.${index + 1}` : String(++number);
    const offset = Math.max(
      0,
      step.start_offset_ms ?? task.actual_start_offset_ms ?? 0,
    );
    const row = {
      task,
      step,
      cleanup,
      parent,
      offset,
      duration: step.duration_ms || 0,
      messages: new Map(),
    };
    const tr = el("tr");
    const name = el("td");
    name.append(
      el("div", "step-name", step.id),
      el(
        "div",
        "meta",
        `${cleanup ? "清理 · " : ""}${step.operation}${step.client ? " · " + step.client : ""}`,
      ),
    );
    if (tasks.length > 1)
      name.append(el("div", "meta", `第 ${task.repeat_index + 1} 次执行`));
    const content = el("td");
    if (step.error) content.append(el("div", "body error", step.error));
    if (step.evidence || step.attempts?.length) {
      const details = el("details", "raw-data");
      details.append(
        el("summary", "", "原始数据"),
        el(
          "pre",
          "evidence",
          JSON.stringify(
            { evidence: step.evidence, attempts: step.attempts },
            null,
            2,
          ),
        ),
      );
      content.append(details);
    }
    tr.append(
      el("td", "", sequence),
      name,
      stepTiming(task, step, cleanup, parent, offset),
      el("td", step.status, step.status === "passed" ? "通过" : "失败"),
      content,
    );
    row.node = tr;
    row.content = content;
    rows.push(row);
    rowMap.set(rowKey(task.path, task.task_id, cleanup, parent, step.id), row);
    $("steps").append(tr);
    appendSteps(task, step.children, cleanup, step.id, sequence);
  });
}
tasks.forEach((task) => {
  appendSteps(task, task.steps);
  appendSteps(task, task.cleanup, true);
  if (task.error && !task.steps?.some((step) => step.error)) {
    appendSteps(task, [
      {
        id: "连接或任务错误",
        operation: "setup",
        status: "failed",
        error: task.error,
        duration_ms: 0,
      },
    ]);
  }
});

// Recording keeps both ends of a transfer. Playback uses the original reply
// once; a new reply with identical bytes is still a separate utterance.
function firstAudioPackets(uri) {
  const match = /^data:audio\/(ogg|wav);base64,(.*)$/.exec(uri);
  if (!match) return null;
  let bytes;
  try {
    bytes = Uint8Array.from(atob(match[2]), (char) => char.charCodeAt(0));
  } catch {
    return null;
  }
  const signature = (offset, text) =>
    [...text].every(
      (char, index) => bytes[offset + index] === char.charCodeAt(0),
    );
  if (match[1] === "wav") {
    // The native exporter writes a fixed mono L16/16 kHz WAV header.
    if (
      bytes.length <= 44 ||
      !signature(0, "RIFF") ||
      !signature(8, "WAVEfmt ") ||
      !signature(36, "data")
    )
      return null;
    return {
      format: Array.from(bytes.slice(8, 36)).join(","),
      packets: [bytes.slice(44, 3244)],
    };
  }
  const packets = [],
    pending = [];
  let offset = 0;
  while (offset < bytes.length && packets.length < 5) {
    if (!signature(offset, "OggS") || offset + 27 > bytes.length) return null;
    const segments = bytes[offset + 26];
    const lengths = bytes.slice(offset + 27, offset + 27 + segments);
    let position = offset + 27 + segments;
    if (lengths.length !== segments) return null;
    for (const length of lengths) {
      if (position + length > bytes.length) return null;
      pending.push(...bytes.slice(position, position + length));
      position += length;
      if (length < 255) {
        const packet = Uint8Array.from(pending);
        pending.length = 0;
        const header = String.fromCharCode(...packet.slice(0, 8));
        if (header !== "OpusHead" && header !== "OpusTags")
          packets.push(packet);
        if (packets.length === 5) break;
      }
    }
    offset = position;
  }
  return { format: "opus", packets };
}
function audioPrefixMatches(short, full, count, cache) {
  for (const uri of [short, full]) {
    if (!cache.has(uri)) cache.set(uri, firstAudioPackets(uri));
  }
  const prefix = cache.get(short),
    complete = cache.get(full);
  if (
    !prefix ||
    !complete ||
    prefix.format !== complete.format ||
    prefix.packets.length === 0
  )
    return false;
  if (prefix.format === "opus" && prefix.packets.length !== count) return false;
  return prefix.packets.every((packet, index) => {
    const other = complete.packets[index];
    return (
      other &&
      (prefix.format !== "opus" || packet.length === other.length) &&
      packet.length <= other.length &&
      packet.every((byte, position) => byte === other[position])
    );
  });
}
function audioReferences(events) {
  const references = new Map(),
    replies = new Map(),
    listeners = new Map();
  const prefixes = new Map();
  const scope = (event, extra) =>
    JSON.stringify([event.path, event.task_id, ...extra]);
  const rowFor = (event) =>
    rowMap.get(
      rowKey(
        event.path,
        event.task_id,
        event.cleanup,
        event.parent,
        event.step,
      ),
    );
  const listenKey = (event, time) =>
    scope(event, [event.parent, event.client, event.role, time]);
  events.forEach((event, index) => {
    if (event.kind !== "audio" || !event.audio_data) return;
    const key = scope(event, [event.audio_data]);
    const original = replies.get(key);
    if (
      event.direction === "sent" &&
      original !== undefined &&
      event.client &&
      event.client !== events[original].client
    ) {
      references.set(index, original);
    }
    if (event.direction === "received" && event.role === "assistant") {
      replies.set(key, index);
      if (event.parent && rowFor(event)?.step.evidence?.mode === "listen") {
        const bucket = listenKey(event, Math.floor(event.offset_ms));
        const previous = listeners.get(bucket);
        if (
          previous === undefined ||
          event.duration_ms > events[previous].duration_ms
        )
          listeners.set(bucket, index);
      }
    }
  });
  events.forEach((event, index) => {
    const integrity = rowFor(event)?.step.evidence?.audio_integrity;
    // A first-response probe only records a few startup packets. The sibling
    // listener on that same connection records the actual continuous reply.
    if (
      !event.parent ||
      event.direction !== "received" ||
      event.role !== "assistant" ||
      !(event.packets > 0 && event.packets <= 5) ||
      !(event.duration_ms > 0 && event.duration_ms <= 100) ||
      integrity?.open !== 1 ||
      integrity.max_active !== 1 ||
      integrity.violations !== 0
    )
      return;
    for (const time of [
      Math.floor(event.offset_ms) - 1,
      Math.floor(event.offset_ms),
      Math.floor(event.offset_ms) + 1,
    ]) {
      const original = listeners.get(listenKey(event, time));
      if (
        original !== undefined &&
        Math.abs(events[original].offset_ms - event.offset_ms) < 1 &&
        events[original].duration_ms > event.duration_ms &&
        audioPrefixMatches(
          event.audio_data,
          events[original].audio_data,
          event.packets,
          prefixes,
        )
      ) {
        references.set(index, original);
        break;
      }
    }
  });
  return references;
}
const recordedEvents = (data.events || [])
  .map((event) => ({ ...event }))
  .sort((a, b) => a.offset_ms - b.offset_ms);
const routeSegments = new Map();
recordedEvents.forEach((event) => {
  const key = JSON.stringify([
    rowKey(event.path, event.task_id, event.cleanup, event.parent, event.step),
    event.client,
    event.connection,
    event.stream,
    event.role,
    event.direction,
    event.attempt,
  ]);
  let state = routeSegments.get(key) || { number: 0 };
  const text = event.mime === "text/plain" || event.kind === "text";
  const audio = event.mime?.startsWith("audio/") || event.kind === "audio";
  if (
    (text &&
      state.textEnded &&
      (event.bos || (event.kind === "text" && event.text))) ||
    (audio && state.audioEnded && (event.bos || event.kind === "audio"))
  )
    state = { number: state.number + 1 };
  event.segment = state.number;
  if (event.eos) {
    if (text || !audio) state.textEnded = true;
    if (audio || !text) state.audioEnded = true;
  }
  routeSegments.set(key, state);
});
const audioCopies = audioReferences(recordedEvents);
// Copies are only inferred from a proven transferred recording or an exact
// complete text transfer. Independent received replies are never collapsed.
const textCopies = new Map(),
  textReplies = new Map(),
  textGroups = new Map(),
  forwardedInputs = new Map(),
  transcriptOwners = new Map(),
  transcriptCopies = new Map(),
  frameByEvent = new Map(),
  referenceNotes = [];
const eventRouteKey = (event) =>
  JSON.stringify([
    rowKey(event.path, event.task_id, event.cleanup, event.parent, event.step),
    event.client,
    event.connection,
    event.stream,
    event.role,
    event.direction,
    event.attempt,
    event.segment,
  ]);
// Sibling probes/listeners on a client observe the same forwarded input. A
// later independent input resets the association instead of hiding its ASR.
const inputScope = (event) =>
  JSON.stringify([
    event.path,
    event.task_id,
    !!event.cleanup,
    event.client,
    event.parent || event.step,
    event.attempt,
  ]);
recordedEvents.forEach((event, index) => {
  if (event.kind !== "text" || event.interim || !event.text) return;
  const key = eventRouteKey(event);
  if (!textGroups.has(key))
    textGroups.set(key, {
      event,
      index,
      indices: [],
      text: "",
      start: event.offset_ms,
      end: event.offset_ms,
    });
  const group = textGroups.get(key);
  group.indices.push(index);
  group.text += event.text;
  group.end = Math.max(group.end, event.offset_ms);
});
for (const [key, group] of textGroups) {
  if (group.event.direction === "received" && group.event.role === "assistant")
    textReplies.set(key, group);
}
const repliesByText = new Map();
for (const reply of textReplies.values()) {
  const key = JSON.stringify([
    reply.event.path,
    reply.event.task_id,
    reply.text.trim(),
  ]);
  if (!repliesByText.has(key)) repliesByText.set(key, new Map());
  const owners = repliesByText.get(key);
  if (!owners.has(reply.event.client)) owners.set(reply.event.client, []);
  owners.get(reply.event.client).push(reply);
}
for (const owners of repliesByText.values())
  for (const replies of owners.values()) replies.sort((a, b) => a.end - b.end);
for (const group of textGroups.values()) {
  const event = group.event;
  if (event.direction !== "sent" || !event.client) continue;
  let match;
  const owners = repliesByText.get(
    JSON.stringify([event.path, event.task_id, group.text.trim()]),
  );
  for (const [owner, replies] of owners || []) {
    if (!owner || owner === event.client) continue;
    let lo = 0,
      hi = replies.length;
    while (lo < hi) {
      const mid = (lo + hi) >>> 1;
      if (replies[mid].end <= group.end) lo = mid + 1;
      else hi = mid;
    }
    for (let i = lo - 1; i >= 0; i--) {
      const reply = replies[i];
      if (reply.start <= group.start) {
        if (!match || match.end < reply.end) match = reply;
        break;
      }
    }
  }
  if (match)
    for (const index of group.indices) textCopies.set(index, match.index);
}
// A startup audio probe also duplicates the text deltas captured by its full
// sibling listener. Reuse the byte-verified audio relation, not text similarity.
for (const [copyIndex, originalIndex] of audioCopies) {
  const copy = recordedEvents[copyIndex],
    original = recordedEvents[originalIndex];
  if (
    copy.direction !== "received" ||
    original.direction !== "received" ||
    copy.client !== original.client
  )
    continue;
  const short = textReplies.get(eventRouteKey(copy)),
    full = textReplies.get(eventRouteKey(original));
  if (
    short &&
    full &&
    Math.abs(short.start - full.start) < 1 &&
    full.text.startsWith(short.text)
  )
    for (const index of short.indices) textCopies.set(index, full.index);
}
recordedEvents
  .map((event, index) => ({ event, index }))
  .sort((a, b) => a.event.offset_ms - b.event.offset_ms || a.index - b.index)
  .forEach(({ event, index }) => {
    const scope = inputScope(event);
    if (
      event.direction === "sent" &&
      (event.kind === "audio" || event.kind === "text")
    ) {
      const original = audioCopies.get(index) ?? textCopies.get(index);
      forwardedInputs.set(
        scope,
        original === undefined
          ? { source: index, forwarded: false }
          : { source: original, forwarded: true },
      );
    } else if (event.role === "transcript" && event.direction === "received") {
      const route = eventRouteKey(event);
      if (!transcriptOwners.has(route))
        transcriptOwners.set(route, forwardedInputs.get(scope) || null);
      const owner = transcriptOwners.get(route);
      if (owner) transcriptCopies.set(index, owner);
    }
  });
const connections = new Map(),
  terminals = new Map(),
  beginnings = new Map(),
  streamEnds = new Map();
const connectionKey = (event) =>
  JSON.stringify([event.path, event.task_id, event.client, event.connection]);
const streamKey = (event) =>
  JSON.stringify([connectionKey(event), event.direction, event.stream]);
recordedEvents.forEach((event) => {
  if (event.bos) {
    const key = streamKey(event);
    if (!beginnings.has(key)) beginnings.set(key, []);
    beginnings.get(key).push(event.offset_ms);
  }
  if (event.action === "close" && event.connection) {
    const key = connectionKey(event);
    connections.set(
      key,
      Math.min(connections.get(key) ?? Infinity, event.offset_ms),
    );
  } else if (event.error && (event.kind !== "audio" || event.eos)) {
    const key = streamKey(event);
    if (!terminals.has(key)) terminals.set(key, []);
    terminals.get(key).push(event.offset_ms);
  }
});
terminals.forEach((times) => times.sort((a, b) => a - b));
beginnings.forEach((times) => times.sort((a, b) => a - b));

function lowerBound(times, at) {
  let lo = 0,
    hi = times.length;
  while (lo < hi) {
    const mid = (lo + hi) >>> 1;
    if (times[mid] < at) lo = mid + 1;
    else hi = mid;
  }
  return lo;
}

function audioWindows(event) {
  let cutoff = connections.get(connectionKey(event)) ?? Infinity;
  // Normal EOS finishes delivery; queued samples may still be audible after
  // it. Only terminal errors and explicit transport closure discard audio.
  const times = terminals.get(streamKey(event)) || [];
  const lo = lowerBound(times, event.offset_ms);
  cutoff = Math.min(cutoff, times[lo] ?? Infinity);
  if (lo > 0) {
    const starts = beginnings.get(streamKey(event)) || [];
    const nextBegin = starts[lowerBound(starts, times[lo - 1])];
    if (nextBegin === undefined || nextBegin > event.offset_ms)
      cutoff = Math.min(cutoff, times[lo - 1]);
  }
  const timing = event.audio_timing || [
    { offset_ms: event.offset_ms, duration_ms: event.duration_ms },
  ];
  const windows = [];
  let media = 0,
    end = Math.max(event.offset_ms, streamEnds.get(streamKey(event)) ?? 0);
  for (const chunk of timing) {
    if (
      !Number.isFinite(chunk.offset_ms) ||
      !Number.isFinite(chunk.duration_ms) ||
      chunk.duration_ms < 0
    )
      throw new Error("录制中的音频时间数据无效。");
    const start = Math.max(end, chunk.offset_ms);
    end = start + chunk.duration_ms;
    const stop = Math.min(end, cutoff);
    if (stop > start) {
      const previous = windows[windows.length - 1];
      if (previous && previous.end === start) previous.end = stop;
      else windows.push({ start, end: stop, media });
    }
    media += chunk.duration_ms;
  }
  streamEnds.set(streamKey(event), Math.min(end, cutoff));
  return windows;
}
if (
  recordedEvents.some(
    (event) =>
      event.kind === "audio" &&
      event.direction !== "prepared" &&
      !event.audio_timing,
  )
) {
  warnings.push(
    "旧记录未保存音频包时间及连接关闭事件，包间停顿和未记录的打断无法完整还原。",
  );
  notice("");
}
recordedEvents.forEach((event, eventIndex) => {
  const row = rowMap.get(
    rowKey(event.path, event.task_id, event.cleanup, event.parent, event.step),
  );
  if (!row) return;
  const at = Math.max(0, event.offset_ms);
  duration = Math.max(duration, at);
  const direction =
    event.role === "transcript" && event.direction === "received"
      ? "识别"
      : { sent: "发送", received: "回复", prepared: "准备", output: "输出" }[
          event.direction
        ] ||
        event.direction ||
        "";
  const label = `${event.client || ""} ${event.role || ""} · ${direction}${event.attempt > 1 ? " · 第 " + event.attempt + " 次尝试" : ""}`;
  if (event.kind === "text") {
    const key = JSON.stringify([
      event.stream,
      event.role,
      event.direction,
      event.connection,
      event.attempt,
      event.interim,
      event.segment,
    ]);
    let message = row.messages.get(key);
    if (!message) {
      const box = el("div", "message");
      const body = el("div", "body");
      const start =
        event.role === "assistant" && event.direction === "received"
          ? "首字 "
          : "文字 ";
      const transcript = transcriptCopies.get(eventIndex);
      const original =
        textCopies.get(eventIndex) ??
        (transcript?.forwarded ? transcript.source : undefined);
      let frameEntry;
      if (original !== undefined || event.interim) {
        const note = el("div", "meta", `${start}${fmt(at)} · ${label}`);
        box.append(note, body);
        appendEvent(row, box, at);
        if (original !== undefined)
          referenceNotes.push({ node: note, original, label });
      } else {
        frameEntry =
          (transcript && !transcript.forwarded
            ? frameByEvent.get(transcript.source)
            : null) || speechFrame(row, event, at);
        frameEntry.end = Math.max(frameEntry.end, at);
        frameByEvent.set(eventIndex, frameEntry);
        box.append(body);
        appendEvent(row, box, at, frameEntry.content);
      }
      message = { body, parts: [], count: -1, entry: frameEntry };
      row.messages.set(key, message);
      messages.push(message);
    }
    message.parts.push({ at, text: event.text });
    if (message.entry) {
      message.entry.end = Math.max(message.entry.end, at);
      frameByEvent.set(eventIndex, message.entry);
    }
  } else if (event.kind === "audio" && event.audio_data) {
    let original = audioCopies.get(eventIndex);
    if (original !== undefined) {
      while (audioCopies.has(original)) original = audioCopies.get(original);
      const source = recordedEvents[original];
      const note = el(
        "div",
        "control audio-reference",
        `${label} · 同一录音已在 ${source.step} 中展示，回放只播放一次。`,
      );
      appendEvent(row, note, at);
      referenceNotes.push({ node: note, original, label });
      return;
    }
    const box = el("div", "audio");
    const audioTiming = el("div", "meta");
    box.append(audioTiming);
    const entry = speechFrame(row, event, at);
    frameByEvent.set(eventIndex, entry);
    appendEvent(row, box, at, entry.content);
    const button = el("button", "segment-play", "播放此段");
    button.type = "button";
    button.setAttribute(
      "aria-label",
      `从 ${fmt(at)} 播放 ${row.step.id} 的音频`,
    );
    box.append(button);
    let windows = [];
    try {
      windows = audioWindows(event);
    } catch (error) {
      warnings.push(`${row.step.id}：${error.message}`);
      notice("");
    }
    const clip = {
      event,
      button,
      offset: at,
      duration: event.duration_ms || 0,
      windows,
      end: windows[windows.length - 1]?.end ?? at,
      auto: event.direction !== "prepared",
      buffer: null,
    };
    clips.push(clip);
    const audibleMS = windows.reduce(
      (total, window) => total + window.end - window.start,
      0,
    );
    audioTiming.textContent = windows.length
      ? `${fmt(windows[0].start)} — ${fmt(clip.end)} · 音频 ${(audibleMS / 1000).toFixed(2)} s`
      : `${fmt(at)} · 这段音频已停止`;
    entry.end = Math.max(entry.end, clip.end);
    button.disabled = !windows.length;
    button.addEventListener("click", () => {
      if (
        (playing &&
          clip.windows.some(
            (window) => position() >= window.start && position() < window.end,
          )) ||
        (loading && segmentClip === clip)
      ) {
        pause();
        return;
      }
      pause();
      cursor = clip.offset;
      segmentClip = clip;
      void startPlayback();
    });
    if (clip.auto) duration = Math.max(duration, clip.end);
  }
  if (event.kind === "control" || event.bos || event.eos || event.error) {
    const channel =
      event.role === "transcript"
        ? "识别"
        : event.mime?.startsWith("audio/")
          ? "音频流"
          : event.mime === "text/plain"
            ? "文字流"
            : "流";
    const text = [
      event.bos
        ? `${channel}${channel === "识别" ? "开始" : "建立"}${channel === "文字流" && !event.text ? "（不代表首字）" : ""}`
        : "",
      event.eos ? `${channel}结束` : "",
      event.action === "close" ? "连接关闭 · 停止播放" : "",
      event.error,
    ]
      .filter(Boolean)
      .join(" · ");
    if (text) {
      appendEvent(
        row,
        el("div", "control", `${fmt(at)} · ${label} · ${text}`),
        at,
      );
      if (event.error) {
        const entry = speechFrame(
          row,
          { ...event, stream: `terminal-${eventIndex}`, role: "event" },
          at,
        );
        appendEvent(
          row,
          el(
            "div",
            "body error",
            /\binterrupted\b/i.test(event.error) ? "发言被打断" : event.error,
          ),
          at,
          entry.content,
        );
      }
    }
  }
});
conversationFrames.sort((a, b) => a.at - b.at);
conversationFrames.forEach((entry, index) => {
  entry.number = index + 1;
  entry.id = `#${String(entry.number).padStart(4, "0")}`;
  entry.node.id = `frame-${entry.number}`;
  entry.node.setAttribute("data-frame", String(entry.number));
  entry.badge.textContent = entry.id;
  entry.badge.setAttribute(
    "aria-label",
    `跳到第 ${entry.number} 帧，${entry.speaker}，${fmt(entry.at)}`,
  );
  $("conversation").append(entry.node);
});
referenceNotes.forEach(({ node, original, label }) => {
  const entry = frameByEvent.get(original);
  if (entry)
    node.textContent = `${label} · 转发或识别自 ${entry.id}，对话中只展示、播放一次。`;
});
$("frame-count").textContent = `${conversationFrames.length} 帧`;
$("frame-index").max = String(conversationFrames.length);
$("frame-index").disabled = $("frame-jump").disabled =
  !conversationFrames.length;
function locateFrame() {
  const number = Number($("frame-index").value);
  if (
    !Number.isInteger(number) ||
    number < 1 ||
    number > conversationFrames.length
  ) {
    notice(`请输入 1 到 ${conversationFrames.length} 之间的帧号。`);
    return;
  }
  notice("");
  jumpToFrame(conversationFrames[number - 1]);
}
$("frame-jump").addEventListener("click", locateFrame);
$("frame-index").addEventListener("keydown", (event) => {
  if (event.key === "Enter") locateFrame();
});
$("step-count").textContent = `${rows.length} 个步骤`;
if (!rows.length) {
  const tr = el("tr"),
    td = el("td", "empty", "本次测试没有可回放步骤。");
  td.colSpan = 5;
  tr.append(td);
  $("steps").append(tr);
}
$("seek").max = String(duration);
$("play").disabled = !rows.length;

const sources = new Set();
let queue = [],
  nextWindow = 0;
const clockNow = () =>
  audioContext ? audioContext.currentTime * 1000 : performance.now();
const limit = () => (segmentClip ? segmentClip.end : duration);
const position = () =>
  Math.min(limit(), origin + (clockNow() - started) * rate);

function stopAudio() {
  audioGeneration++;
  clearTimeout(timer);
  sources.forEach((source) => {
    source.stop();
    source.disconnect();
  });
  sources.clear();
  clips.forEach((clip) => {
    clip.button.textContent = "播放此段";
  });
}
function pause() {
  if (playing) cursor = position();
  playing = loading = false;
  cancelAnimationFrame(frame);
  stopAudio();
  $("play").textContent = "继续回放";
  update(true);
}
async function decodeClip(clip) {
  if (!clip.buffer) {
    if (!clip.decoding) {
      clip.decoding = (async () => {
        const match = /^data:audio\/(?:wav|ogg);base64,(.*)$/.exec(
          clip.event.audio_data,
        );
        if (!match) throw new Error("无效的内嵌音频");
        const bytes = Uint8Array.from(atob(match[1]), (char) =>
          char.charCodeAt(0),
        );
        clip.buffer = await audioContext.decodeAudioData(bytes.buffer);
      })().catch((error) => {
        clip.decoding = null;
        throw new Error(
          `步骤 ${clip.event.step} 的音频无法解码：${error.message}`,
        );
      });
    }
    await clip.decoding;
  }
}
async function startPlayback() {
  stopAudio();
  if (cursor >= limit()) cursor = segmentClip ? segmentClip.offset : 0;
  const generation = audioGeneration;
  const selected = segmentClip
    ? [segmentClip]
    : clips.filter((clip) => clip.auto);
  loading = true;
  $("play").textContent = "取消准备";
  notice("正在准备音频…");
  update(true);
  try {
    if (selected.some((clip) => clip.windows.length)) {
      if (!audioContext) {
        const Audio = window.AudioContext || window.webkitAudioContext;
        if (!Audio) throw new Error("浏览器不支持音频回放。");
        audioContext = new Audio();
      }
      await audioContext.resume();
      // Decode before starting the clock. Browser loading cannot move a
      // recorded interruption or make two parallel streams drift apart.
      for (const clip of selected) {
        if (generation !== audioGeneration) return;
        if (clip.windows.length) await decodeClip(clip);
      }
    }
    if (generation !== audioGeneration) return;
    queue = selected
      .flatMap((clip) => clip.windows.map((window) => ({ ...window, clip })))
      .filter((window) => window.end > cursor)
      .sort((a, b) => a.start - b.start);
    nextWindow = 0;
    loading = false;
    playing = true;
    origin = cursor;
    started = clockNow();
    $("play").textContent = "暂停";
    notice("");
    pumpAudio();
    frame = requestAnimationFrame(tick);
  } catch (error) {
    if (generation !== audioGeneration) return;
    pause();
    notice(error.message);
  }
}
function pumpAudio() {
  if (!playing) return;
  const at = position();
  if (at >= limit()) {
    tick();
    return;
  }
  // Schedule on the audio clock ahead of rendering. Stops remain accurate
  // even when an animation frame is delayed or the tab is in the background.
  try {
    while (nextWindow < queue.length && queue[nextWindow].start < at + 2000) {
      const window = queue[nextWindow++];
      const start = Math.max(position(), window.start),
        end = Math.min(window.end, limit());
      if (end <= start) continue;
      const source = audioContext.createBufferSource();
      source.buffer = window.clip.buffer;
      source.playbackRate.value = rate;
      source.connect(audioContext.destination);
      source.onended = () => {
        sources.delete(source);
        source.disconnect();
      };
      source.start(
        (started + (start - origin) / rate) / 1000,
        (window.media + start - window.start) / 1000,
        (end - start) / 1000,
      );
      sources.add(source);
    }
  } catch (error) {
    pause();
    notice(`音频回放失败：${error.message}`);
    return;
  }
  timer = setTimeout(pumpAudio, 100);
}
function update(progress) {
  overview = !progress;
  $("overview").textContent = progress ? "查看全部记录" : "回到当前时间";
  const displayedDuration = Math.max(duration, segmentClip?.end || 0);
  $("seek").max = String(displayedDuration);
  $("seek").value = String(cursor);
  $("clock").textContent =
    `${progress ? "" : "完整记录 · "}${fmt(cursor)} / ${fmt(displayedDuration)}`;
  timelineEvents.forEach(({ node, at }) => {
    node.hidden = progress && at > cursor;
  });
  let focus = null;
  rows.forEach((row) => {
    const active =
      progress &&
      cursor >= row.offset &&
      cursor <= row.offset + Math.max(row.duration, 200);
    row.node.classList.toggle("active", active);
    row.node.classList.toggle("future", progress && cursor < row.offset);
  });
  messages.forEach((message) => {
    let count = message.parts.length;
    if (progress) {
      let lo = 0,
        hi = count;
      while (lo < hi) {
        const mid = (lo + hi) >>> 1;
        if (message.parts[mid].at <= cursor) lo = mid + 1;
        else hi = mid;
      }
      count = lo;
    }
    if (message.count !== count) {
      message.body.textContent = message.parts
        .slice(0, count)
        .map((part) => part.text)
        .join("");
      message.count = count;
    }
  });
  let visible = 0;
  conversationFrames.forEach((entry) => {
    entry.node.hidden = progress && entry.at > cursor;
    if (!entry.node.hidden) visible++;
    const active =
      progress &&
      cursor >= entry.at &&
      cursor < Math.max(entry.end, entry.at + 200);
    entry.node.classList.toggle("active", active);
    if (active) focus = entry.node;
  });
  $("empty-conversation").hidden = visible > 0;
  $("empty-conversation").textContent = conversationFrames.length
    ? "点击开始回放，或查看全部记录。每段对话都有固定帧号。"
    : "本次测试没有可回放的对话。";
  $("frame-state").textContent = overview
    ? "完整记录 · 后续内容全部展开"
    : loading
      ? "正在准备音频…"
      : playing
        ? focus
          ? "正在回放对话"
          : "等待下一段对话…"
        : "按事件时间回放 · 可用帧号定位";
  if (playing && focus && focus !== lastFocus) {
    focus.scrollIntoView({ block: "nearest", behavior: "smooth" });
    lastFocus = focus;
  }
}
function tick() {
  if (!playing) return;
  cursor = position();
  update(true);
  clips.forEach((clip) => {
    const active =
      (segmentClip ? clip === segmentClip : clip.auto) &&
      clip.windows.some(
        (window) => cursor >= window.start && cursor < window.end,
      );
    clip.button.textContent = active ? "暂停回放" : "播放此段";
  });
  if (cursor >= limit()) {
    pause();
    $("play").textContent = "重新回放";
    return;
  }
  frame = requestAnimationFrame(tick);
}
$("play").addEventListener("click", () => {
  if (playing || loading) {
    pause();
    return;
  }
  void startPlayback();
});
$("reset").addEventListener("click", () => {
  pause();
  segmentClip = null;
  cursor = 0;
  update(true);
  $("play").textContent = "开始回放";
});
$("overview").addEventListener("click", () => {
  const showAll = !overview;
  pause();
  update(!showAll);
});
$("seek").addEventListener("input", () => {
  const resume = playing || loading;
  const target = Number($("seek").value) || 0;
  pause();
  segmentClip = null;
  cursor = Math.max(0, Math.min(duration, target));
  update(true);
  if (resume) void startPlayback();
});
$("speed").addEventListener("change", () => {
  const resume = playing || loading;
  pause();
  rate = Math.max(0.5, Math.min(2, Number($("speed").value) || 1));
  if (resume) void startPlayback();
});
window.addEventListener("pagehide", () => {
  pause();
  if (audioContext) {
    void audioContext.close().catch(() => {});
    audioContext = null;
  }
});
messages.forEach((message) => message.parts.sort((a, b) => a.at - b.at));
update(true);
