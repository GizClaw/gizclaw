import { useState } from "react";
import {
  ArrowDownLeft,
  ArrowUpRight,
  ChevronRight,
  Network,
  Search,
  Server,
} from "lucide-react";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { ConsoleServer } from "@/lib/config";
import type { ServerState } from "@/hooks/use-fleet";
import { bytes, duration, host } from "@/lib/format";
import { countText, transportTotals } from "@/lib/node-metrics";
import { cn } from "@/lib/utils";

type NodeSource = { server: ConsoleServer; state: ServerState | undefined };

export function FleetNodeTable({
  nodes,
  devices = false,
}: {
  nodes: NodeSource[];
  devices?: boolean;
}) {
  const [role, setRole] = useState("all");
  const [query, setQuery] = useState("");
  const roleOf = ({ server, state }: NodeSource) =>
    state?.snapshot?.role ?? server.role;
  const shown = nodes.filter(
    (node) =>
      (role === "all" || roleOf(node) === role) &&
      [node.server.name, host(node.server.url), node.server.region ?? ""].some(
        (value) => value.toLowerCase().includes(query.trim().toLowerCase()),
      ),
  );
  const hasMetrics = nodes.some(
    ({ state }) => state?.status === "online" && state.snapshot?.device_metrics,
  );

  return (
    <Card className="gap-0 overflow-hidden p-0">
      <div className="flex flex-wrap items-center justify-between gap-4 px-5 py-5">
        <div className="flex items-baseline gap-2.5">
          <h2 className="text-[15px] font-semibold">
            {devices ? "节点采集明细" : "节点列表"}
          </h2>
          <span className="text-xs text-muted-foreground">
            {nodes.length} 个节点
          </span>
        </div>
        <label className="relative w-full sm:w-56">
          <Search
            size={14}
            className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            aria-label="搜索节点"
            placeholder="搜索节点或区域"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            className="h-9 bg-background/50 pl-9 text-xs"
          />
        </label>
      </div>
      <div
        className="flex gap-1 px-5 pb-4"
        role="group"
        aria-label="节点类型筛选"
      >
        {[
          ["all", "全部"],
          ["server", "Server"],
          ["edge", "Edge"],
        ].map(([value, label]) => (
          <button
            key={value}
            type="button"
            aria-pressed={role === value}
            onClick={() => setRole(value)}
            className={cn(
              "inline-flex items-center gap-2 rounded-md px-3 py-1.5 text-xs transition-colors hover:bg-secondary",
              role === value
                ? "bg-secondary font-medium text-foreground"
                : "text-muted-foreground",
            )}
          >
            {label}
            <span className="text-[10px] tabular-nums text-muted-foreground">
              {value === "all"
                ? nodes.length
                : nodes.filter((node) => roleOf(node) === value).length}
            </span>
          </button>
        ))}
      </div>
      <Table className="text-[12px]">
        <TableHeader className="bg-secondary/45">
          <TableRow className="hover:bg-transparent">
            <TableHead className="pl-5">节点</TableHead>
            <TableHead>状态</TableHead>
            {devices ? (
              hasMetrics ? (
                <>
                  <TableHead className="text-right">在线设备</TableHead>
                  <TableHead className="text-right">RTP 收 / 发</TableHead>
                  <TableHead className="text-right">
                    DataChannel{" "}
                    <span className="block text-[10px] font-normal">
                      packet / service / native
                    </span>
                  </TableHead>
                </>
              ) : (
                <TableHead>指标采集</TableHead>
              )
            ) : (
              <>
                <TableHead className="text-right">连接</TableHead>
                <TableHead className="text-right">收 / 发速率</TableHead>
                <TableHead className="text-right">运行时间</TableHead>
                <TableHead className="text-right">进程资源</TableHead>
              </>
            )}
            <TableHead>版本</TableHead>
            <TableHead className="pr-5">
              <span className="sr-only">详情</span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {shown.map(({ server, state }) => {
            const live = state?.status === "online";
            const metrics = live ? state.snapshot?.device_metrics : undefined;
            const counts = metrics ? transportTotals(metrics) : undefined;
            const sample = state?.samples.at(-1);
            const nodeRole = state?.snapshot?.role ?? server.role;
            const nodeHost = host(server.url);
            const href = `#/server/${encodeURIComponent(server.id)}`;
            return (
              <TableRow key={server.id}>
                <TableCell className="py-4 pl-5">
                  <div className="flex items-center gap-3">
                    <span
                      className={cn(
                        "hidden size-9 shrink-0 items-center justify-center rounded-lg border sm:flex",
                        nodeRole === "edge"
                          ? "border-chart-rx/15 bg-chart-rx/5 text-chart-rx"
                          : "border-border bg-secondary/60 text-muted-foreground",
                      )}
                    >
                      {nodeRole === "edge" ? (
                        <Network size={16} />
                      ) : (
                        <Server size={16} />
                      )}
                    </span>
                    <div className="min-w-0">
                      <a href={href} className="font-medium hover:text-primary">
                        {server.name}
                      </a>
                      <div className="mt-1 flex flex-wrap items-center gap-x-2 text-[10px] text-muted-foreground">
                        <span className="uppercase tracking-wider">
                          {nodeRole}
                        </span>
                        {nodeHost !== server.name && <span>{nodeHost}</span>}
                        {server.region && <span>{server.region}</span>}
                      </div>
                    </div>
                  </div>
                </TableCell>
                <TableCell>
                  <span className="inline-flex items-center gap-2 whitespace-nowrap text-xs">
                    <span
                      className={cn(
                        "size-1.5 rounded-full",
                        live
                          ? "bg-success"
                          : state?.status === "error"
                            ? "bg-destructive"
                            : "bg-muted-foreground/40",
                      )}
                    />
                    {live
                      ? "在线"
                      : state?.status === "error"
                        ? "读取失败"
                        : "连接中"}
                  </span>
                  {state?.error && (
                    <div
                      className="mt-1 max-w-40 truncate text-[10px] text-destructive"
                      title={state.error}
                    >
                      {state.error}
                    </div>
                  )}
                </TableCell>
                {devices ? (
                  counts ? (
                    <>
                      <TableCell className="text-right font-medium tabular-nums">
                        {metrics?.online_devices === undefined
                          ? "—"
                          : countText(metrics.online_devices)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {countText(counts.rtpRxTracks)} /{" "}
                        {countText(counts.rtpTxTracks)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {countText(counts.packetChannels)} /{" "}
                        {countText(counts.serviceChannels)} /{" "}
                        {countText(counts.nativeChannels)}
                      </TableCell>
                    </>
                  ) : (
                    <TableCell
                      colSpan={hasMetrics ? 3 : 1}
                      className="text-xs text-muted-foreground"
                    >
                      <span
                        className={cn(
                          "inline-flex items-center gap-2",
                          live && "text-warning",
                        )}
                      >
                        <span
                          className={cn(
                            "size-1.5 rounded-full",
                            live ? "bg-warning/70" : "bg-muted-foreground/40",
                          )}
                        />
                        {live
                          ? "等待节点升级"
                          : state?.status === "error"
                            ? "连接恢复后继续采集"
                            : "等待首次采样"}
                      </span>
                    </TableCell>
                  )
                ) : (
                  <>
                    <TableCell className="text-right font-medium tabular-nums">
                      {live
                        ? (state.snapshot?.transport.connections ?? "—")
                        : "—"}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {live && sample ? (
                        <div className="flex flex-col items-end gap-1 text-[11px]">
                          <span className="inline-flex items-center gap-1.5">
                            <ArrowDownLeft
                              size={11}
                              className="text-chart-rx"
                            />
                            {bytes(sample.rx)}/s
                          </span>
                          <span className="inline-flex items-center gap-1.5">
                            <ArrowUpRight size={11} className="text-chart-tx" />
                            {bytes(sample.tx)}/s
                          </span>
                        </div>
                      ) : (
                        "—"
                      )}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-right text-xs tabular-nums">
                      {live && state.snapshot
                        ? duration(state.snapshot.uptime_seconds)
                        : "—"}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {live && state.snapshot ? (
                        <>
                          <div>{bytes(state.snapshot.heap_bytes)}</div>
                          <div className="mt-1 text-[10px] text-muted-foreground">
                            {state.snapshot.goroutines} goroutines
                          </div>
                        </>
                      ) : (
                        "—"
                      )}
                    </TableCell>
                  </>
                )}
                <TableCell className="font-mono text-[11px]">
                  <div>{state?.snapshot?.version ?? "—"}</div>
                  {state?.snapshot?.build_commit && (
                    <div
                      className="mt-1 text-[10px] text-muted-foreground"
                      title={state.snapshot.build_commit}
                    >
                      {state.snapshot.build_commit.slice(0, 7)}
                    </div>
                  )}
                </TableCell>
                <TableCell className="pr-5 text-right">
                  <a
                    href={href}
                    aria-label={`查看 ${server.name}`}
                    className="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                  >
                    <ChevronRight size={16} />
                  </a>
                </TableCell>
              </TableRow>
            );
          })}
          {shown.length === 0 && (
            <TableRow>
              <TableCell
                colSpan={devices ? (hasMetrics ? 7 : 5) : 8}
                className="py-12 text-center text-sm text-muted-foreground"
              >
                没有匹配的节点
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </Card>
  );
}
