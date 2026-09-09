/** One persistent log record as shown by the console. */
export type LogRecord = {
  id: number;
  time: string;
  level: string;
  message: string;
  error?: string;
  peer_public_key?: string;
  fields?: Record<string, string>;
  node: string;
  nodeName: string;
};

export type LogQuery = {
  text: string[];
  clauses: { key: string; value: string; negate: boolean }[];
};

const FIELD = /^(-?)([A-Za-z_][A-Za-z0-9_.]*):(.*)$/;

/**
 * Splits a filter into free text and `key:value` clauses; `-key:value`
 * excludes. Unparsed input stays free text so a half-typed filter still
 * searches instead of failing.
 */
export function parseQuery(input: string): LogQuery {
  const query: LogQuery = { text: [], clauses: [] };
  for (const token of input.trim().split(/\s+/)) {
    if (token === "") continue;
    const match = FIELD.exec(token);
    if (match && match[3] !== "") {
      query.clauses.push({
        key: match[2].toLowerCase(),
        value: match[3].toLowerCase(),
        negate: match[1] === "-",
      });
    } else {
      query.text.push(token.toLowerCase());
    }
  }
  return query;
}

function fieldValue(record: LogRecord, key: string): string | undefined {
  switch (key) {
    case "node":
      return record.node;
    case "level":
      return record.level;
    case "message":
      return record.message;
    case "error":
      return record.error;
    case "peer":
    case "peer_public_key":
      return record.peer_public_key;
    default:
      return record.fields?.[key];
  }
}

function haystack(record: LogRecord): string {
  return [
    record.message,
    summarize(record),
    record.error ?? "",
    record.peer_public_key ?? "",
    record.nodeName,
    ...Object.entries(record.fields ?? {}).map(
      ([key, value]) => `${key}:${value}`,
    ),
  ]
    .join(" ")
    .toLowerCase();
}

/** A record matches when every clause and every free-text term matches. */
export function matches(record: LogRecord, query: LogQuery): boolean {
  const text = query.text.length > 0 ? haystack(record) : "";
  for (const term of query.text) {
    if (!text.includes(term)) return false;
  }
  for (const clause of query.clauses) {
    const value = fieldValue(record, clause.key);
    const hit =
      value !== undefined && value.toLowerCase().includes(clause.value);
    if (hit === clause.negate) return false;
  }
  return true;
}

/**
 * One readable line per record: HTTP uses access fields, RPC uses its operation
 * and outcome, a conversation record shows who said what, and anything else keeps
 * its message plus the few fields that carry the meaning.
 */
export function summarize(record: LogRecord): string {
  const fields = record.fields ?? {};
  if (fields.content !== undefined) {
    const role =
      fields.role ?? fields.content_role ?? fields.content_source ?? "";
    const scope = [role, fields.boundary, fields.event]
      .filter(Boolean)
      .join(" · ");
    const turn =
      fields.turn_index === undefined ? "" : ` #${fields.turn_index}`;
    return `${scope}${turn}: ${fields.content}`;
  }
  if (fields.operation !== undefined || fields.method !== undefined) {
    const parts =
      fields.method === undefined &&
      fields.operation &&
      fields.operation !== "unknown"
        ? [fields.operation]
        : [];
    const path =
      fields.request_path ??
      (fields.route && fields.route !== "unknown" ? fields.route : undefined);
    if (fields.method !== undefined) {
      parts.push(`${fields.method} ${path ?? "unknown"}`);
    } else if (path !== undefined) {
      parts.push(path);
    }
    if (fields.client_ip) parts.push(fields.client_ip);
    if (fields.status !== undefined) parts.push(`HTTP ${fields.status}`);
    if (fields.rpc_code !== undefined && fields.rpc_code !== "0") {
      const label = rpcStatusLabels[fields.rpc_code];
      parts.push(
        label ? `${label}（RPC ${fields.rpc_code}）` : `RPC ${fields.rpc_code}`,
      );
    }
    if (
      fields.result !== undefined &&
      fields.result !== "success" &&
      fields.error_code === undefined
    )
      parts.push(fields.result);
    if (fields.error_code !== undefined) parts.push(fields.error_code);

    if (fields.duration_ms !== undefined)
      parts.push(`${fields.duration_ms} ms`);
    return parts.join(" · ");
  }
  const extras = [
    "component",
    "event",
    "role",
    "boundary",
    "stream_id",
    "event_type",
    "state",
    "reason",
    "duration_ms",
  ]
    .filter((key) => fields[key] !== undefined)
    .map((key) => `${key}=${fields[key]}`);
  return extras.length === 0
    ? record.message
    : `${record.message} · ${extras.join(" ")}`;
}

const rpcStatusLabels: Record<string, string> = {
  "1": "请求已取消",
  "2": "未知错误",
  "3": "参数无效",
  "4": "请求超时",
  "5": "未找到",
  "6": "已存在",
  "7": "权限不足",
  "8": "资源耗尽",
  "9": "前置条件不满足",
  "10": "操作中止",
  "11": "超出范围",
  "12": "未实现",
  "13": "内部错误",
  "14": "服务不可用",
  "15": "数据丢失",
  "16": "未认证",
};

export function requestId(record: LogRecord): string | undefined {
  return record.fields?.request_id;
}
