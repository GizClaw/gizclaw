"use strict";
const data = JSON.parse(document.getElementById("recording-data").textContent);
const $ = (id) => document.getElementById(id);
const el = (tag, cls, text) => {
  const node = document.createElement(tag);
  if (cls) node.className = cls;
  if (text !== undefined) node.textContent = text;
  return node;
};
const fmt = (ms) =>
  `${Math.floor(ms / 60000)}:${((ms / 1000) % 60).toFixed(1).padStart(4, "0")}`;
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
  lastFocus = null;
const warnings = [];
if (data.truncated) warnings.push("录制达到容量上限，以下为已保存的部分记录。");
if (data.errors?.length) warnings.push(data.errors.join("\n"));
const notice = (message) => {
  $("notice").textContent = [...warnings, message].filter(Boolean).join("\n");
};
notice("");
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

const relaySent = new Set();
(data.events || []).forEach((event) => {
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
      box.append(el("div", "meta", label), body);
      row.content.append(box);
      message = { body, parts: [], count: -1 };
      row.messages.set(key, message);
      messages.push(message);
    }
    message.parts.push({ at, text: event.text });
  } else if (event.kind === "audio" && event.audio_data) {
    const box = el("div", "audio");
    box.append(
      el(
        "div",
        "meta",
        `${label} · ${((event.duration_ms || 0) / 1000).toFixed(2)} s`,
      ),
    );
    const audio = el("audio");
    audio.controls = true;
    audio.preload = "none";
    audio.src = event.audio_data;
    audio.setAttribute("aria-label", `${row.step.id} ${event.role || ""} 音频`);
    box.append(audio);
    row.content.append(box);
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
    clips.push({
      node: audio,
      offset: at,
      duration: event.duration_ms || 0,
      auto,
      active: false,
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
      row.content.append(
        el("div", "control", `${fmt(at)} · ${label} · ${text}`),
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
    clip.node.pause();
    clip.active = false;
  });
}
function pause() {
  playing = false;
  cancelAnimationFrame(frame);
  stopAudio();
  $("play").textContent = "继续回放";
}
function update(progress) {
  $("seek").value = String(cursor);
  $("clock").textContent = `${fmt(cursor)} / ${fmt(duration)}`;
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
  cursor = Math.min(duration, origin + (now - started) * rate);
  update(true);
  clips.forEach((clip) => {
    const active =
      clip.auto &&
      cursor >= clip.offset &&
      cursor < clip.offset + clip.duration;
    if (active && !clip.active) {
      clip.active = true;
      clip.node.currentTime = Math.max(0, (cursor - clip.offset) / 1000);
      clip.node.playbackRate = rate;
      const generation = audioGeneration;
      clip.node.play().catch(() => {
        if (!playing || !clip.active || generation !== audioGeneration) return;
        pause();
        notice("浏览器未能开始播放当前步骤的音频，请使用该步骤的音频控件。 ");
      });
    } else if (!active && clip.active) {
      clip.node.pause();
      clip.active = false;
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
  stopAudio();
  if (cursor >= duration) cursor = 0;
  playing = true;
  origin = cursor;
  started = performance.now();
  $("play").textContent = "暂停";
  notice("");
  update(true);
  frame = requestAnimationFrame(tick);
});
$("reset").addEventListener("click", () => {
  pause();
  cursor = 0;
  update(false);
  $("play").textContent = "开始回放";
});
$("seek").addEventListener("input", () => {
  stopAudio();
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
