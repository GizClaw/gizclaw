import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import { SUMMARY_FIELDS, type LogRecord } from "@/lib/log-query";

const ROW = 30;

function levelClass(level: string) {
  if (level === "ERROR") return "text-[#e08a76]";
  if (level === "WARN") return "text-[#d1ae72]";
  if (level === "DEBUG") return "text-[#8a8377]";
  return "text-[#759788]";
}

/** One line per record, with the fields that identify the request kept inline. */
export function LogStream({
  records,
  selectedId,
  onSelect,
  emptyMessage,
  height = 520,
}: {
  records: LogRecord[];
  selectedId?: string;
  onSelect: (record: LogRecord) => void;
  emptyMessage: string;
  height?: number;
}) {
  const [follow, setFollow] = useState(true);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (follow && box.current) box.current.scrollTop = box.current.scrollHeight;
  }, [records, follow]);
  return (
    <section className="overflow-hidden rounded-lg border border-surface-dark-border bg-surface-dark">
      <div className="flex items-center gap-3 border-b border-surface-dark-border bg-[#211f1b] px-4 py-2.5 text-[11px] text-surface-dark-foreground">
        <span>{records.length} 条记录</span>
        <span className="text-[#817c73]">最新在下方</span>
        <button
          className="ml-auto rounded px-2 py-1 text-[10px] text-[#aba08f] hover:bg-[#2a2721]"
          onClick={() => setFollow(!follow)}
        >
          {follow ? "跟随中" : "自动跟随"}
        </button>
      </div>
      <div
        ref={box}
        style={{ height }}
        className="overflow-auto font-mono text-[11px]"
        onScroll={(event) => {
          const element = event.currentTarget;
          const atBottom =
            element.scrollHeight - element.scrollTop - element.clientHeight <
            ROW;
          if (atBottom !== follow) setFollow(atBottom);
        }}
      >
        {records.length === 0 ? (
          <div className="px-6 py-24 text-center text-[#6c675f]">
            {emptyMessage}
          </div>
        ) : (
          records.map((record) => {
            const id = `${record.node}-${record.id}`;
            const summary = SUMMARY_FIELDS.map((key) => record.fields?.[key])
              .filter((value) => value !== undefined)
              .join(" · ");
            const request = record.fields?.request_id;
            return (
              <button
                key={id}
                onClick={() => onSelect(record)}
                className={cn(
                  "flex w-full items-center gap-3 border-b border-[#26231e] px-4 text-left text-[#c8c0b2] hover:bg-[#242017]",
                  selectedId === id && "bg-[#2b2619]",
                )}
                style={{ height: ROW }}
              >
                <time className="whitespace-nowrap text-[#756e62]">
                  {new Date(record.time).toLocaleTimeString()}
                </time>
                <b
                  className={cn(
                    "min-w-9 text-[10px] font-normal",
                    levelClass(record.level),
                  )}
                >
                  {record.level}
                </b>
                <span className="w-24 shrink-0 truncate text-[#ae977f]">
                  {record.nodeName}
                </span>
                <span className="min-w-0 flex-1 truncate">
                  {summary || record.message}
                  {record.error ? (
                    <span className="text-[#e08a76]"> · {record.error}</span>
                  ) : null}
                </span>
                {request && (
                  <span className="hidden w-28 shrink-0 truncate text-right text-[#6f6a60] lg:block">
                    {request}
                  </span>
                )}
              </button>
            );
          })
        )}
      </div>
    </section>
  );
}
