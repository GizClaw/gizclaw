import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { timestamp } from "@/lib/format";

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

/** A key's presence is independent of its JSON value, including null/false/0. */
export function PeerKV({ value }: { value: unknown }) {
  const entries = isRecord(value)
    ? Object.entries(value).sort(([a], [b]) => a.localeCompare(b))
    : [];
  return (
    <Card>
      <CardHeader>
        <CardTitle>设备自报 KV</CardTitle>
        <CardDescription>
          设备最后上报的 JSON 快照；离线时保留，内容由设备提供。
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {entries.length === 0 ? (
          <p className="text-xs text-muted-foreground">暂无自报数据</p>
        ) : (
          entries.map(([key, entry]) => {
            if (!isRecord(entry) || !Object.hasOwn(entry, "value")) return null;
            return (
              <details key={key} className="rounded-md border p-3">
                <summary className="cursor-pointer text-xs">
                  <code>{key}</code>
                  <span className="ml-3 text-muted-foreground">
                    {timestamp(
                      typeof entry.observed_at === "string"
                        ? entry.observed_at
                        : undefined,
                    )}
                  </span>
                  <span className="ml-3 font-mono">
                    {entry.value === null
                      ? "null"
                      : Array.isArray(entry.value)
                        ? "数组"
                        : typeof entry.value === "object"
                          ? "对象"
                          : typeof entry.json_value === "string"
                            ? entry.json_value
                            : JSON.stringify(entry.value)}
                  </span>
                </summary>
                <pre className="mt-3 overflow-auto whitespace-pre-wrap break-words text-xs">
                  {typeof entry.json_value === "string"
                    ? entry.json_value
                    : JSON.stringify(entry.value, null, 2)}
                </pre>
              </details>
            );
          })
        )}
      </CardContent>
    </Card>
  );
}
