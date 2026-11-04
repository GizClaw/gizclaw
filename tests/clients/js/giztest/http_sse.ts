const maxResponseBytes = 4 << 20;
const maxEvents = 16384;

/** Projects a finite SSE response into the shared Giztest HTTP step value. */
export function decodeHTTPEventStream(text: string): Record<string, unknown> {
  if (new TextEncoder().encode(text).byteLength > maxResponseBytes) {
    throw new Error("giztest: SSE response exceeds 4 MiB");
  }
  const events: Array<{ event: string; data: unknown }> = [];
  const result: Record<string, unknown> = { events, raw: text };
  let pending = text.replace(/^\uFEFF/u, "").replace(/\r\n?/gu, "\n");
  let name = "message";
  let data: string[] = [];
  while (true) {
    const end = pending.indexOf("\n");
    if (end < 0) break;
    const line = pending.slice(0, end);
    pending = pending.slice(end + 1);
    if (line === "") {
      if (data.length !== 0) {
        if (events.length >= maxEvents) {
          throw new Error("giztest: SSE response exceeds 16384 events");
        }
        const text = data.join("\n");
        let value: unknown;
        try {
          value = JSON.parse(text);
        } catch {
          value = text;
        }
        const event = { event: name, data: value };
        events.push(event);
        result.last_event = event;
      }
      name = "message";
      data = [];
      continue;
    }
    const separator = line.indexOf(":");
    const field = separator < 0 ? line : line.slice(0, separator);
    const value =
      separator < 0 ? "" : line.slice(separator + 1).replace(/^ /u, "");
    if (field === "event") name = value === "" ? "message" : value;
    if (field === "data") data.push(value);
  }
  return result;
}
