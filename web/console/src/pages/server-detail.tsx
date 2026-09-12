import {
  Activity,
  ArrowDownLeft,
  ArrowUpRight,
  Copy,
  Wifi,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { MetricCard } from "@/components/metric-card";
import { TrafficChart, TrafficLegend } from "@/components/traffic-chart";
import { PageHeading } from "@/components/app-shell";
import { StatusBadge } from "@/pages/overview";
import type { ConsoleServer } from "@/lib/config";
import type { ServerState } from "@/hooks/use-fleet";
import { bytes, duration, host } from "@/lib/format";
import { usePageView } from "@/assistant/page-view";

export function ServerDetailPage({
  server,
  state,
  windowSeconds,
}: {
  server: ConsoleServer;
  state: ServerState | undefined;
  windowSeconds: number;
}) {
  const snapshot = state?.snapshot;
  const sample = state?.samples.at(-1);
  const live = state?.status === "online";
  usePageView({
    page: "节点详情",
    window_seconds: windowSeconds,
    node: {
      id: server.id,
      name: server.name,
      role: server.role,
      region: server.region,
    },
    status: state?.status ?? "loading",
    error: state?.error,
    snapshot,
    rates: sample
      ? { rx_bytes_per_second: sample.rx, tx_bytes_per_second: sample.tx }
      : undefined,
  });
  return (
    <>
      <PageHeading
        eyebrow="A CLOSER LOOK AT THIS NODE"
        title={server.name}
        description={`${host(server.url)}${server.region ? ` · ${server.region}` : ""}`}
        action={
          <div className="flex items-center gap-2">
            <StatusBadge
              status={state?.status}
              role={snapshot?.role ?? server.role}
            />
            <span className="text-[11px] text-muted-foreground">
              {state?.updatedAt
                ? `更新于 ${new Date(state.updatedAt).toLocaleTimeString()}`
                : "等待数据"}
            </span>
          </div>
        }
      />
      {state?.status === "error" && (
        <Alert>
          <AlertTitle>无法读取该节点</AlertTitle>
          <AlertDescription>{state.error}</AlertDescription>
          <AlertDescription className="text-muted-foreground">
            检查地址、Monitor Token，以及该节点是否开启 monitor.token
            配置。恢复后会自动继续。
          </AlertDescription>
        </Alert>
      )}
      <Card className="gap-3">
        <CardHeader>
          <CardTitle className="text-xs">节点公钥</CardTitle>
          <CardAction>
            <Button
              variant="ghost"
              size="sm"
              disabled={!snapshot}
              onClick={() => {
                if (snapshot)
                  void navigator.clipboard?.writeText(snapshot.public_key);
              }}
            >
              <Copy size={14} /> 复制
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent>
          <code className="block truncate font-mono text-[11px] text-muted-foreground">
            {snapshot?.public_key ?? "—"}
          </code>
        </CardContent>
      </Card>
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-5">
        <MetricCard
          label="入站服务 DataChannel"
          value={
            snapshot ? String(snapshot.transport.inbound_service_channels) : "—"
          }
          note="对端打开、尚未释放（含待打开）"
          icon={<ArrowDownLeft size={16} />}
        />
        <MetricCard
          label="WebRTC 连接"
          value={snapshot ? String(snapshot.transport.connections) : "—"}
          note={
            snapshot ? `${snapshot.transport.services} 个服务` : "当前活跃关联"
          }
          icon={<Wifi size={16} />}
        />
        <MetricCard
          label="接收速率"
          value={live && sample ? `${bytes(sample.rx)}/s` : "—"}
          note={
            snapshot ? `累计 ${bytes(snapshot.transport.rx_bytes)}` : "等待采样"
          }
          icon={<ArrowDownLeft size={16} />}
        />
        <MetricCard
          label="发送速率"
          value={live && sample ? `${bytes(sample.tx)}/s` : "—"}
          note={
            snapshot ? `累计 ${bytes(snapshot.transport.tx_bytes)}` : "等待采样"
          }
          icon={<ArrowUpRight size={16} />}
        />
        <MetricCard
          label="运行时间"
          value={snapshot ? duration(snapshot.uptime_seconds) : "—"}
          note={
            snapshot
              ? `${snapshot.goroutines} goroutines · ${bytes(snapshot.heap_bytes)} 堆内存`
              : "当前进程"
          }
          icon={<Activity size={16} />}
        />
      </div>
      <Tabs defaultValue="traffic">
        <TabsList>
          <TabsTrigger value="traffic">流量</TabsTrigger>
          <TabsTrigger value="snapshot">运行快照</TabsTrigger>
        </TabsList>
        <TabsContent value="traffic">
          <Card>
            <CardHeader>
              <CardTitle>流量趋势</CardTitle>
              <CardDescription>
                应用负载字节，不含 ICE / DTLS 开销 · 本次会话采样
              </CardDescription>
              <CardAction>
                <TrafficLegend />
              </CardAction>
            </CardHeader>
            <CardContent>
              <TrafficChart
                samples={state?.samples ?? []}
                windowSeconds={windowSeconds}
                emptyMessage={live ? "等待流量采样" : "连接后显示实时流量"}
              />
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="snapshot">
          <Card>
            <CardHeader>
              <CardTitle>节点运行快照</CardTitle>
              <CardDescription>
                节点自身上报的原始字段，空值保持为空。
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-5">
              <dl className="grid gap-x-10 gap-y-2.5 sm:grid-cols-2 xl:grid-cols-3">
                <Field label="角色" value={snapshot?.role} />
                <Field label="版本" value={snapshot?.version} />
                <Field label="构建提交" value={snapshot?.build_commit} />
                <Field
                  label="快照时间"
                  value={
                    snapshot
                      ? new Date(snapshot.time).toLocaleString()
                      : undefined
                  }
                />
                <Field
                  label="运行时间"
                  value={
                    snapshot ? duration(snapshot.uptime_seconds) : undefined
                  }
                />
                <Field label="Goroutines" value={snapshot?.goroutines} />
                <Field
                  label="堆内存"
                  value={snapshot ? bytes(snapshot.heap_bytes) : undefined}
                />
                <Field label="服务数" value={snapshot?.transport.services} />
                <Field
                  label="累计接收"
                  value={
                    snapshot ? bytes(snapshot.transport.rx_bytes) : undefined
                  }
                />
                <Field
                  label="累计发送"
                  value={
                    snapshot ? bytes(snapshot.transport.tx_bytes) : undefined
                  }
                />
              </dl>
              <details className="border-t border-border pt-4">
                <summary className="cursor-pointer text-xs text-muted-foreground">
                  原始 JSON
                </summary>
                <pre className="mt-3 max-h-96 overflow-auto text-[11px] leading-relaxed text-muted-foreground">
                  {snapshot ? JSON.stringify(snapshot, null, 2) : "—"}
                </pre>
              </details>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </>
  );
}

function Field({ label, value }: { label: string; value?: string | number }) {
  return (
    <div className="flex items-baseline justify-between gap-6 text-xs">
      <dt className="shrink-0 text-muted-foreground">{label}</dt>
      <dd className="m-0 min-w-0 truncate text-right font-mono tabular-nums">
        {value === undefined || value === "" ? "—" : String(value)}
      </dd>
    </div>
  );
}
