import {
  ArrowDownLeft,
  ArrowUpRight,
  CircleAlert,
  Server as ServerIcon,
  Wifi,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { MetricCard } from "@/components/metric-card";
import { TrafficChart, TrafficLegend } from "@/components/traffic-chart";
import { LogView, type LogRow } from "@/components/log-view";
import { PageHeading } from "@/components/app-shell";
import type { ConsoleServer } from "@/lib/config";
import type { FleetState } from "@/hooks/use-fleet";
import { bytes, duration, host, type Sample } from "@/lib/format";

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
  const alerts: LogRow[] = states
    .flatMap(({ server, state }) =>
      (state?.snapshot?.logs ?? [])
        .filter((entry) => entry.level === "ERROR" || entry.level === "WARN")
        .map((entry) => ({ ...entry, source: server.name })),
    )
    .sort((a, b) => Date.parse(a.time) - Date.parse(b.time))
    .slice(-200);

  return (
    <>
      <PageHeading
        eyebrow="FLEET AT A GLANCE"
        title={config.name}
        description={`${config.servers.length} 个节点 · 每 5 秒轮询各节点自身的 Monitor 快照`}
      />
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <MetricCard
          label="在线节点"
          value={`${online.length} / ${config.servers.length}`}
          note={
            failing.length > 0 ? `${failing.length} 个无法读取` : "全部可读"
          }
          icon={<ServerIcon size={16} />}
        />
        <MetricCard
          label="WebRTC 连接"
          value={online.length > 0 ? String(connections) : "—"}
          note="在线节点当前活跃关联合计"
          icon={<Wifi size={16} />}
        />
        <MetricCard
          label="接收速率"
          value={online.length > 0 ? `${bytes(rx)}/s` : "—"}
          note="各节点最近一次采样之和"
          icon={<ArrowDownLeft size={16} />}
        />
        <MetricCard
          label="发送速率"
          value={online.length > 0 ? `${bytes(tx)}/s` : "—"}
          note="各节点最近一次采样之和"
          icon={<ArrowUpRight size={16} />}
        />
      </div>
      <Card>
        <CardHeader>
          <CardTitle>集群流量</CardTitle>
          <CardDescription>
            应用负载字节，不含 ICE / DTLS 开销 · 本次会话采样，按 5 秒对齐求和
          </CardDescription>
          <CardAction>
            <TrafficLegend />
          </CardAction>
        </CardHeader>
        <CardContent>
          <TrafficChart
            samples={aggregate(states.map(({ state }) => state?.samples ?? []))}
            windowSeconds={windowSeconds}
            emptyMessage="等待流量采样"
          />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>节点</CardTitle>
          <CardDescription>点击节点查看流量、日志与运行快照。</CardDescription>
        </CardHeader>
        <CardContent className="px-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="pl-5">节点</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>连接</TableHead>
                <TableHead>接收 / 发送</TableHead>
                <TableHead>运行时间</TableHead>
                <TableHead>Goroutines · 堆</TableHead>
                <TableHead className="pr-5 text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {states.map(({ server, state }) => {
                const sample = state?.samples.at(-1);
                return (
                  <TableRow key={server.id}>
                    <TableCell className="pl-5">
                      <div className="font-medium">{server.name}</div>
                      <div className="mt-1 font-mono text-[10px] text-muted-foreground">
                        {host(server.url)}
                        {server.region ? ` · ${server.region}` : ""}
                      </div>
                    </TableCell>
                    <TableCell>
                      <StatusBadge
                        status={state?.status}
                        role={state?.snapshot?.role ?? server.role}
                      />
                      {state?.status === "error" && (
                        <div className="mt-1 max-w-64 truncate text-[10px] text-destructive">
                          {state.error}
                        </div>
                      )}
                    </TableCell>
                    <TableCell className="tabular-nums">
                      {state?.snapshot
                        ? state.snapshot.transport.connections
                        : "—"}
                    </TableCell>
                    <TableCell className="tabular-nums">
                      {sample && state?.status === "online"
                        ? `${bytes(sample.rx)}/s · ${bytes(sample.tx)}/s`
                        : "—"}
                    </TableCell>
                    <TableCell>
                      {state?.snapshot
                        ? duration(state.snapshot.uptime_seconds)
                        : "—"}
                    </TableCell>
                    <TableCell className="tabular-nums">
                      {state?.snapshot
                        ? `${state.snapshot.goroutines} · ${bytes(state.snapshot.heap_bytes)}`
                        : "—"}
                    </TableCell>
                    <TableCell className="pr-5 text-right">
                      <Button variant="outline" size="sm" asChild>
                        <a href={`#/server/${encodeURIComponent(server.id)}`}>
                          查看
                        </a>
                      </Button>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <CircleAlert size={15} className="text-warning" /> 集群告警
          </CardTitle>
          <CardDescription>
            各节点最新快照中的 WARN 与 ERROR 记录，按节点标注来源。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <LogView entries={alerts} title="WARN / ERROR" height={240} />
        </CardContent>
      </Card>
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
