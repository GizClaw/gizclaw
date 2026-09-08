import { useEffect, useRef, useState } from "react";
import { Terminal } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { LogEntry } from "@/lib/api";
import { summarize } from "@/lib/log-query";

export type LogRow = LogEntry & { source?: string };

const LEVELS = ["ALL", "DEBUG", "INFO", "WARN", "ERROR"] as const;

/** Compact single-line records on the warm dark surface; never a table. */
export function LogView({
  entries,
  title = "运行日志",
  height = 300,
}: {
  entries: LogRow[];
  title?: string;
  height?: number;
}) {
  const [filter, setFilter] = useState("");
  const [level, setLevel] = useState<string>("ALL");
  const [follow, setFollow] = useState(true);
  const box = useRef<HTMLDivElement>(null);
  const shown = entries.filter(
    (entry) =>
      (level === "ALL" || entry.level === level) &&
      `${summarize(entry)} ${entry.message} ${entry.error ?? ""} ${entry.peer_public_key ?? ""} ${entry.source ?? ""} ${Object.values(entry.fields ?? {}).join(" ")}`
        .toLowerCase()
        .includes(filter.toLowerCase()),
  );
  useEffect(() => {
    if (follow && box.current) box.current.scrollTop = box.current.scrollHeight;
  }, [entries, follow]);
  return (
    <section className="overflow-hidden rounded-lg border border-surface-dark-border bg-surface-dark">
      <div className="flex flex-wrap items-center gap-3 border-b border-surface-dark-border bg-[#211f1b] px-4 py-3 text-surface-dark-foreground">
        <span className="flex items-center gap-2 text-xs">
          <Terminal size={15} /> {title}
          <small className="ml-1 text-[10px] text-[#817c73]">
            {shown.length} 条
          </small>
        </span>
        <Input
          aria-label="筛选日志"
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
          placeholder="搜索消息、错误或设备公钥…"
          className="ml-auto h-8 w-56 border-[#3c372e] bg-surface-dark text-[11px] text-[#bbb2a4]"
        />
        <Select
          aria-label="日志级别"
          value={level}
          onChange={(event) => setLevel(event.target.value)}
          className="h-8 border-[#3c372e] bg-surface-dark text-[11px] text-[#bbb2a4]"
        >
          {LEVELS.map((value) => (
            <option key={value}>{value}</option>
          ))}
        </Select>
        <Button
          variant="ghost"
          size="sm"
          className="h-8 text-[10px] text-[#aba08f] hover:bg-[#2a2721]"
          onClick={() => setFollow(!follow)}
        >
          {follow ? "跟随中" : "自动跟随"}
        </Button>
      </div>
      <div
        ref={box}
        style={{ height }}
        className="log-scroll overflow-auto font-mono text-[11px]"
      >
        {shown.length === 0 ? (
          <div className="px-6 py-20 text-center text-[#6c675f]">
            暂无匹配日志。只显示节点实际记录的数据。
          </div>
        ) : (
          shown.map((entry, index) => (
            <div
              key={`${entry.source ?? ""}-${entry.id}-${index}`}
              className="flex h-[26px] items-center gap-3.5 border-b border-[#26231e] px-4 text-[#c8c0b2]"
            >
              <time className="whitespace-nowrap text-[#756e62]">
                {new Date(entry.time).toLocaleTimeString()}
              </time>
              <b
                className={cn(
                  "min-w-9 text-[10px] font-normal text-[#759788]",
                  entry.level === "ERROR" && "text-[#e08a76]",
                  entry.level === "WARN" && "text-[#d1ae72]",
                )}
              >
                {entry.level}
              </b>
              {entry.source && (
                <span className="w-28 shrink-0 truncate text-[#ae977f]">
                  {entry.source}
                </span>
              )}
              <span
                className="truncate"
                title={`${summarize(entry)}${entry.error ? ` · ${entry.error}` : ""}`}
              >
                {summarize(entry)}
                {entry.error ? ` · ${entry.error}` : ""}
              </span>
            </div>
          ))
        )}
      </div>
    </section>
  );
}
