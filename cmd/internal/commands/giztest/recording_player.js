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
  const ticks = Math.max(0, Math.round(ms / 100));
  return `${Math.floor(ticks / 600)}:${((ticks % 600) / 10).toFixed(1).padStart(4, "0")}`;
};
const tasks = data.report.tasks || [];
const base = tasks.length
  ? Math.min(
      ...tasks.map(
        (task) =>
          task.actual_start_offset_ms ?? task.planned_start_offset_ms ?? 0,
      ),
    )
  : 0;
const rowKey = (path, task, cleanup, parent, step) =>
  JSON.stringify([path, task, !!cleanup, parent || "", step]);
const rows = [],
  clips = [],
  messages = [],
  timelineEvents = [],
  rowMap = new Map();
let number = 0,
  cursor = 0,
  duration = Math.max(
    1,
    tasks.length === 1
      ? (tasks[0].duration_ms ?? data.report.duration_ms)
      : data.report.duration_ms - base,
  ),
  playing = false,
  origin = 0,
  started = 0,
  rate = 1,
  frame = 0,
  audioGeneration = 0,
  segmentClip = null,
  lastFocus = null;
const warnings = [];
if (data.truncated) warnings.push("录制达到容量上限，以下为已保存的部分记录。");
if (data.errors?.length) warnings.push(data.errors.join("\n"));
const notice = (message) => {
  $("notice").textContent = [...warnings, message].filter(Boolean).join("\n");
};
notice("");
function appendEvent(row, node, at) {
  row.content.append(node);
  timelineEvents.push({ node, at });
}
$("summary").textContent =
  `${data.report.status === "passed" ? "验收通过" : "验收失败"} · ${tasks.length} 次执行 · ${fmt(data.report.duration_ms)} · ${new Date(data.report.started_at).toLocaleString()}`;

function appendSteps(task, steps, cleanup = false, parent = "", prefix = "") {
  (steps || []).forEach((step, index) => {
    const sequence = prefix ? `${prefix}.${index + 1}` : String(++number);
    const offset = Math.max(
      0,
      (step.start_offset_ms ?? task.actual_start_offset_ms ?? 0) - base,
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
      content.append(
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
    }
    tr.append(
      el("td", "", sequence),
      name,
      el("td", "", `${fmt(offset)}\n${step.duration_ms || 0} ms`),
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
const recordedEvents = data.events || [];
const audioCopies = audioReferences(recordedEvents);
const relaySent = new Set();
recordedEvents.forEach((event, eventIndex) => {
  const row = rowMap.get(
    rowKey(event.path, event.task_id, event.cleanup, event.parent, event.step),
  );
  if (!row) return;
  const at = Math.max(0, event.offset_ms - base);
  const label = `${event.client || ""} ${event.role || ""} · ${{ sent: "发送", received: "回复", prepared: "准备", output: "输出" }[event.direction] || event.direction || ""}${event.attempt > 1 ? " · 第 " + event.attempt + " 次尝试" : ""}`;
  if (event.kind === "text") {
    const key = JSON.stringify([
      event.stream,
      event.role,
      event.direction,
      event.attempt,
      event.interim,
    ]);
    let message = row.messages.get(key);
    if (!message) {
      const box = el("div", "message");
      const body = el("div", "body");
      box.append(el("div", "meta", `${fmt(at)} · ${label}`), body);
      appendEvent(row, box, at);
      message = { body, parts: [], count: -1 };
      row.messages.set(key, message);
      messages.push(message);
    }
    message.parts.push({ at, text: event.text });
  } else if (event.kind === "audio" && event.audio_data) {
    let original = audioCopies.get(eventIndex);
    if (original !== undefined) {
      while (audioCopies.has(original)) original = audioCopies.get(original);
      const source = recordedEvents[original];
      appendEvent(
        row,
        el(
          "div",
          "control audio-reference",
          `${label} · 同一录音已在 ${source.step} 中展示，回放只播放一次。`,
        ),
        at,
      );
      return;
    }
    const box = el("div", "audio");
    box.append(
      el(
        "div",
        "meta",
        `${fmt(at)} · ${label} · 时长 ${fmt(event.duration_ms || 0)}`,
      ),
    );
    const audio = el("audio");
    // A segment uses the same transport and absolute clock as the full replay.
    // Independent native controls would create a second, unsynchronized clock.
    audio.hidden = true;
    audio.preload = "none";
    audio.src = event.audio_data;
    audio.setAttribute("aria-label", `${row.step.id} ${event.role || ""} 音频`);
    box.append(audio);
    appendEvent(row, box, at);
    const relayKey = JSON.stringify([
      event.path,
      event.task_id,
      event.step,
      event.attempt,
    ]);
    const forwarded =
      row.step.operation === "workspace_relay" && event.direction === "sent";
    const auto =
      event.direction !== "prepared" &&
      (!forwarded || !relaySent.has(relayKey));
    if (forwarded) relaySent.add(relayKey);
    const button = el("button", "segment-play", "播放此段");
    button.type = "button";
    button.setAttribute(
      "aria-label",
      `从 ${fmt(at)} 播放 ${row.step.id} 的音频`,
    );
    box.append(button);
    const clip = {
      node: audio,
      button,
      offset: at,
      duration: event.duration_ms || 0,
      auto,
      active: false,
    };
    clips.push(clip);
    button.addEventListener("click", () => {
      if (playing && clip.active) {
        pause();
        return;
      }
      pause();
      cursor = clip.offset;
      segmentClip = clip;
      startPlayback();
    });
    duration = Math.max(duration, at + (event.duration_ms || 0));
    audio.addEventListener("error", () => {
      pause();
      notice(`步骤 ${row.step.id} 的音频无法播放。`);
    });
  } else if (event.kind === "control") {
    const text = [
      event.bos ? "流开始" : "",
      event.eos ? "流结束" : "",
      event.error,
    ]
      .filter(Boolean)
      .join(" · ");
    if (text)
      appendEvent(
        row,
        el("div", "control", `${fmt(at)} · ${label} · ${text}`),
        at,
      );
  }
});
if (!rows.length) {
  const tr = el("tr"),
    td = el("td", "empty", "本次测试没有可回放步骤。");
  td.colSpan = 5;
  tr.append(td);
  $("steps").append(tr);
}
$("seek").max = String(duration);
$("play").disabled = !rows.length;

function stopAudio() {
  audioGeneration++;
  clips.forEach((clip) => {
    clip.active = false;
    clip.node.pause();
    clip.button.textContent = "播放此段";
  });
}
function pause() {
  const master = clips.find((clip) => clip.active && !clip.node.ended);
  if (playing && master) cursor = clipPosition(master);
  playing = false;
  cancelAnimationFrame(frame);
  stopAudio();
  $("play").textContent = "继续回放";
  update(true);
}
function clipPosition(clip) {
  const local = Math.max(
    0,
    Math.min(clip.duration, clip.node.currentTime * 1000),
  );
  return Math.min(duration, clip.offset + (Number.isFinite(local) ? local : 0));
}
function startPlayback() {
  stopAudio();
  if (cursor >= duration) cursor = 0;
  playing = true;
  origin = cursor;
  started = performance.now();
  $("play").textContent = "暂停";
  notice("");
  update(true);
  frame = requestAnimationFrame(tick);
}
function update(progress) {
  $("seek").value = String(cursor);
  $("clock").textContent = `${fmt(cursor)} / ${fmt(duration)}`;
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
    if (active) focus = row.node;
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
  if (playing && focus && focus !== lastFocus) {
    focus.scrollIntoView({ block: "nearest", behavior: "smooth" });
    lastFocus = focus;
  }
}
function tick(now) {
  if (!playing) return;
  const master = clips.find((clip) => clip.active);
  if (master) {
    // Media time remains still while the browser loads or buffers audio. The
    // event clock must remain still with it, including in a background tab.
    cursor = master.node.ended
      ? master.offset + master.duration
      : clipPosition(master);
    origin = cursor;
    started = now;
  } else {
    cursor = Math.min(duration, origin + (now - started) * rate);
  }
  if (segmentClip && cursor >= segmentClip.offset + segmentClip.duration) {
    pause();
    return;
  }
  update(true);
  clips.forEach((clip) => {
    const active =
      (segmentClip ? clip === segmentClip : clip.auto) &&
      cursor >= clip.offset &&
      cursor < clip.offset + clip.duration;
    if (active && !clip.active) {
      clip.active = true;
      clip.button.textContent = "暂停此段";
      clip.node.currentTime = Math.max(0, (cursor - clip.offset) / 1000);
      clip.node.playbackRate = rate;
      const generation = audioGeneration;
      clip.node.play().catch(() => {
        if (!playing || !clip.active || generation !== audioGeneration) return;
        pause();
        notice("浏览器未能开始播放当前步骤的音频，请点击播放此段重试。");
      });
    } else if (!active && clip.active) {
      clip.node.pause();
      clip.active = false;
      clip.button.textContent = "播放此段";
    }
  });
  if (cursor >= duration) {
    pause();
    $("play").textContent = "重新回放";
    return;
  }
  frame = requestAnimationFrame(tick);
}
$("play").addEventListener("click", () => {
  if (playing) {
    pause();
    return;
  }
  segmentClip = null;
  startPlayback();
});
$("reset").addEventListener("click", () => {
  pause();
  segmentClip = null;
  cursor = 0;
  update(false);
  $("play").textContent = "开始回放";
});
$("seek").addEventListener("input", () => {
  stopAudio();
  segmentClip = null;
  cursor = Number($("seek").value);
  origin = cursor;
  started = performance.now();
  update(true);
});
$("speed").addEventListener("change", () => {
  rate = Number($("speed").value);
  origin = cursor;
  started = performance.now();
  clips.forEach((clip) => {
    clip.node.playbackRate = rate;
  });
});
window.addEventListener("pagehide", pause);
update(false);
