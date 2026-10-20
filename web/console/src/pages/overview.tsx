import { useState } from "react";
import {
  ArrowDownLeft,
  ArrowUpRight,
  LayoutGrid,
  Network,
  Radio,
  Server as ServerIcon,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { MonitorSummary } from "@/components/monitor-summary";
import { FleetNodeTable } from "@/components/fleet-node-table";
import { TrafficChart, TrafficLegend } from "@/components/traffic-chart";
import type { ConsoleServer } from "@/lib/config";
import type { FleetState } from "@/hooks/use-fleet";
import { bytes, type Sample } from "@/lib/format";
import { usePageView } from "@/assistant/page-view";
import { NodeDeviceMetrics } from "@/components/node-device-metrics";
import { cn } from "@/lib/utils";

function aggregate(samples: Sample[][]): Sample[] {
  const buckets = new Map<number, Sample>();
  for (const series of samples) {
    for (const sample of series) {
      const key = Math.floor(sample.timestamp / 5000) * 5000;
      const current = buckets.get(key);
      buckets.set(key, {
        timestamp: key,
        time: new Date(key).toLocaleTimeString(),
        rx: (current?.rx ?? 0) + sample.rx,
        tx: (current?.tx ?? 0) + sample.tx,
      });
    }
  }
  return [...buckets.values()].sort((a, b) => a.timestamp - b.timestamp);
}

export function OverviewPage({
  config,
  fleet,
  windowSeconds,
}: {
  config: { name: string; servers: ConsoleServer[] };
  fleet: FleetState;
  windowSeconds: number;
}) {
  const [tab, setTab] = useState("nodes");
  const states = config.servers.map((server) => ({
    server,
    state: fleet[server.id],
  }));
  const online = states.filter(({ state }) => state?.status === "online");
  const failing = states.filter(({ state }) => state?.status === "error");
  const connections = online.reduce(
    (total, { state }) => total + (state?.snapshot?.transport.connections ?? 0),
    0,
  );
  const latest = (id: string) => fleet[id]?.samples.at(-1);
  const rx = online.reduce(
    (total, { server }) => total + (latest(server.id)?.rx ?? 0),
    0,
  );
  const tx = online.reduce(
    (total, { server }) => total + (latest(server.id)?.tx ?? 0),
    0,
  );
  usePageView({
    page: "集群总览",
    tab,
    window_seconds: windowSeconds,
    online_nodes: online.length,
    total_nodes: config.servers.length,
    failing_nodes: failing.map(({ server }) => server.id),
    connections,
    rx_bytes_per_second: rx,
    tx_bytes_per_second: tx,
    nodes: states.map(({ server, state }) => ({
      id: server.id,
      name: server.name,
      role: server.role,
      status: state?.status ?? "loading",
      connections: state?.snapshot?.transport.connections,
      device_metrics:
        state?.status === "online" ? state.snapshot?.device_metrics : undefined,
      message_rates_per_second:
        state?.status === "online" ? state.deviceRates : undefined,
      error: state?.error,
    })),
  });

  const serverCount = states.filter(
    ({ server, state }) => (state?.snapshot?.role ?? server.role) === "server",
  ).length;
  return (
    <>
      <div className="flex flex-wrap items-end justify-between gap-4 pb-1">
        <div>
          <p className="mb-2 text-xs font-medium text-muted-foreground">
            集群监控
          </p>
          <h1 className="text-[30px] font-semibold leading-tight tracking-tight">
            {config.name}
          </h1>
          <p className="mt-2.5 text-xs text-muted-foreground">
            {serverCount} Server <span className="mx-2 text-border">/</span>{" "}
            {states.length - serverCount} Edge{" "}
            <span className="mx-2 text-border">·</span> 每 5 秒更新
          </p>
        </div>
        <div className="flex items-center gap-2 rounded-full border bg-card px-3 py-2 text-[11px] text-muted-foreground">
          <span
            className={cn(
              "size-1.5 rounded-full",
              online.length === states.length && states.length > 0
                ? "bg-success"
                : failing.length > 0
                  ? "bg-warning"
                  : "bg-muted-foreground/40",
            )}
          />
          {online.length} / {states.length} 个节点数据可读
        </div>
      </div>
      <Tabs value={tab} onValueChange={setTab} className="gap-6">
        <TabsList aria-label="集群视图" className="monitor-tabs">
          <TabsTrigger value="nodes">
            <LayoutGrid size={14} />
            节点概览
          </TabsTrigger>
          <TabsTrigger value="devices">
            <Radio size={14} />
            设备与通道
          </TabsTrigger>
        </TabsList>
        <TabsContent value="nodes" className="gap-6">
          <MonitorSummary
            metrics={[
              {
                label: "在线节点",
                value: (
                  <>
                    {online.length}
                    <span className="ml-1.5 text-xl font-normal text-muted-foreground/60">
                      / {states.length}
                    </span>
                  </>
                ),
                note: failing.length
                  ? `${failing.length} 个节点读取失败`
                  : online.length === states.length
                    ? "全部节点可连接"
                    : "正在连接节点",
                icon: <ServerIcon size={17} />,
              },
              {
                label: "WebRTC 连接",
                value: online.length ? String(connections) : "—",
                note: "在线节点的活跃连接合计",
                icon: <Network size={17} />,
              },
              {
                label: "接收速率",
                value: online.length ? (
                  <>
                    {bytes(rx)}
                    <span className="ml-1 text-lg text-muted-foreground">
                      /s
                    </span>
                  </>
                ) : (
                  "—"
                ),
                note: "最近采样间隔 · 所有在线节点",
                icon: <ArrowDownLeft size={17} />,
                tone: "rx",
              },
              {
                label: "发送速率",
                value: online.length ? (
                  <>
                    {bytes(tx)}
                    <span className="ml-1 text-lg text-muted-foreground">
                      /s
                    </span>
                  </>
                ) : (
                  "—"
                ),
                note: "最近采样间隔 · 所有在线节点",
                icon: <ArrowUpRight size={17} />,
                tone: "tx",
              },
            ]}
          />
          <Card>
            <CardHeader>
              <CardTitle className="text-[15px]">集群流量</CardTitle>
              <CardDescription className="text-[11px]">
                最近 {windowSeconds / 60} 分钟 · 集群应用负载流量
              </CardDescription>
              <CardAction className="self-center">
                <TrafficLegend />
              </CardAction>
            </CardHeader>
            <CardContent>
              <TrafficChart
                samples={aggregate(
                  states.map(({ state }) => state?.samples ?? []),
                )}
                windowSeconds={windowSeconds}
                height={230}
                emptyMessage="正在收集流量采样"
              />
            </CardContent>
          </Card>
          <FleetNodeTable nodes={states} />
        </TabsContent>
        <TabsContent value="devices" className="gap-6">
          <NodeDeviceMetrics nodes={states} windowSeconds={windowSeconds} />
          <FleetNodeTable nodes={states} devices />
        </TabsContent>
      </Tabs>
    </>
  );
}

export function StatusBadge({
  status,
  role,
}: {
  status: "loading" | "online" | "error" | undefined;
  role: string;
}) {
  if (status === "online") {
    return <Badge variant="success">在线 · {role}</Badge>;
  }
  if (status === "error") return <Badge variant="destructive">无法读取</Badge>;
  return <Badge variant="outline">连接中</Badge>;
}
