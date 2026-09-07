import type { LogEntry } from "@/lib/api";

/** One record as shown by the console, tagged with the node it came from. */
export type LogRecord = LogEntry & { node: string; nodeName: string };

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

/** Fields worth showing inline; everything else stays in the detail panel. */
export const SUMMARY_FIELDS = [
  "operation",
  "route",
  "method",
  "status",
  "rpc_code",
  "duration_ms",
] as const;

export function requestId(record: LogRecord): string | undefined {
  return record.fields?.request_id;
}
