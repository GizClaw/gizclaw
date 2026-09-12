import type { LogEntry } from "./runtime.ts";

/** A log entry reduced to the fields the model needs to reason about it. */
export type CompactLogRecord = {
  time: string;
  level: string;
  message: string;
  operation?: string;
  http_status?: string;
  rpc_code?: string;
  error_code?: string;
  result?: string;
  duration_ms?: number;
  request_id?: string;
};

export type LogAggregate = {
  total: number;
  first_time?: string;
  last_time?: string;
  by_level: Record<string, number>;
  by_operation: Record<string, number>;
  by_error_code: Record<string, number>;
  by_rpc_code: Record<string, number>;
  by_http_status: Record<string, number>;
  /** Error codes ordered by count, most frequent first. */
  top_errors: { error_code: string; count: number; last_message: string }[];
};

export function compactLog(entry: LogEntry): CompactLogRecord {
  const fields = entry.fields ?? {};
  const record: CompactLogRecord = {
    time: new Date(entry.time_ms).toISOString(),
    level: entry.level.toLowerCase(),
    message: entry.message,
  };
  const operation = logOperation(entry);
  if (operation) record.operation = operation;
  const httpStatus = text(fields.status);
  if (httpStatus) record.http_status = httpStatus;
  const rpcCode = text(fields.rpc_code);
  if (rpcCode && rpcCode !== "0") record.rpc_code = rpcCode;
  const errorCode = text(fields.error_code);
  if (errorCode) record.error_code = errorCode;
  const result = text(fields.result);
  if (result && result !== "success") record.result = result;
  const duration = Number(fields.duration_ms);
  if (fields.duration_ms !== undefined && Number.isFinite(duration))
    record.duration_ms = duration;
  const requestId = text(fields.request_id);
  if (requestId) record.request_id = requestId;
  return record;
}

export function aggregateLogs(entries: LogEntry[]): LogAggregate {
  const aggregate: LogAggregate = {
    total: entries.length,
    by_level: {},
    by_operation: {},
    by_error_code: {},
    by_rpc_code: {},
    by_http_status: {},
    top_errors: [],
  };
  const lastMessage = new Map<string, { time: number; message: string }>();
  let first = Infinity;
  let last = -Infinity;
  for (const entry of entries) {
    const record = compactLog(entry);
    first = Math.min(first, entry.time_ms);
    last = Math.max(last, entry.time_ms);
    count(aggregate.by_level, record.level);
    if (record.operation) count(aggregate.by_operation, record.operation);
    if (record.rpc_code) count(aggregate.by_rpc_code, record.rpc_code);
    if (record.http_status) count(aggregate.by_http_status, record.http_status);
    if (record.error_code) {
      count(aggregate.by_error_code, record.error_code);
      const previous = lastMessage.get(record.error_code);
      if (!previous || entry.time_ms >= previous.time) {
        lastMessage.set(record.error_code, {
          time: entry.time_ms,
          message: entry.message,
        });
      }
    }
  }
  if (entries.length > 0) {
    aggregate.first_time = new Date(first).toISOString();
    aggregate.last_time = new Date(last).toISOString();
  }
  aggregate.top_errors = Object.entries(aggregate.by_error_code)
    .sort(([a, left], [b, right]) => right - left || a.localeCompare(b))
    .slice(0, 5)
    .map(([errorCode, total]) => ({
      error_code: errorCode,
      count: total,
      last_message: lastMessage.get(errorCode)?.message ?? "",
    }));
  return aggregate;
}

// operation mirrors the console's log summary: an explicit operation, or the
// HTTP method with its request path or route.
function logOperation(entry: LogEntry): string | undefined {
  const fields = entry.fields ?? {};
  const method = text(fields.method);
  const route = text(fields.route);
  const path =
    text(fields.request_path) ??
    (route && route !== "unknown" ? route : undefined);
  if (method) return `${method} ${path ?? "unknown"}`;
  const operation = text(fields.operation);
  if (operation && operation !== "unknown") return operation;
  return path;
}

function text(value: unknown): string | undefined {
  if (value === undefined || value === null || value === "") return undefined;
  return String(value);
}

function count(target: Record<string, number>, key: string): void {
  target[key] = (target[key] ?? 0) + 1;
}
