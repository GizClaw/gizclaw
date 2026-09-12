import { useEffect, useState } from "react";
import {
  ArrowDownLeft,
  ArrowUpRight,
  BatteryMedium,
  Copy,
  ScrollText,
  ShieldCheck,
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { MetricCard } from "@/components/metric-card";
import { TrafficChart, TrafficLegend } from "@/components/traffic-chart";
import { PageHeading } from "@/components/app-shell";
import { PeerChat } from "@/components/peer-chat";
import { PeerTelemetry } from "@/components/peer-telemetry";
import { PeerLocation } from "@/components/peer-location";
import type { PeerState } from "@/hooks/use-peers";
import { peerLabel, type WatchedPeer } from "@/lib/peers";
import { bytes, host, timestamp } from "@/lib/format";
import { usePageView } from "@/assistant/page-view";

export function PeerDetailPage({
  peer,
  state,
  windowSeconds,
}: {
  peer: WatchedPeer;
  state: PeerState | undefined;
  windowSeconds: number;
}) {
  const runtime = state?.snapshot?.runtime;
  const sample = state?.samples.at(-1);
  const [copied, setCopied] = useState(false);
  const [tab, setTab] = useState("traffic");
  usePageView({
    page: "设备详情",
    tab,
    device: { public_key: peer.publicKey, label: peer.label },
    status: state?.status ?? "loading",
    error: state?.error,
    runtime,
    info: state?.snapshot?.info,
    reported_status: state?.snapshot?.status,
  });
  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(false), 1500);
    return () => clearTimeout(timer);
  }, [copied]);
  return (
    <>
      <PageHeading
        eyebrow="DEVICE DETAIL"
        title={peerLabel(peer, state?.snapshot)}
        description={`接入点 ${host(peer.endpoint)} · 设备数据由所属 Server 授权返回`}
        action={
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" asChild>
              <a
                href={`#/logs?q=${encodeURIComponent(`peer_public_key:${peer.publicKey}`)}`}
              >
                <ScrollText size={13} /> 在日志查询中打开
              </a>
            </Button>
            {state?.status === "error" ? (
              <Badge variant="destructive">无法读取</Badge>
            ) : runtime ? (
              <Badge variant={runtime.online ? "success" : "outline"}>
                {runtime.online ? "在线" : "离线"}
              </Badge>
            ) : (
              <Badge variant="outline">读取中</Badge>
            )}
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
          <AlertTitle>无法读取该设备</AlertTitle>
          <AlertDescription>{state.error}</AlertDescription>
          <AlertDescription className="text-muted-foreground">
            检查设备公钥、调试权限，以及接入点是否可达。恢复后会自动继续。
          </AlertDescription>
        </Alert>
      )}
      <Card className="gap-3">
        <CardHeader>
          <CardTitle className="text-xs">设备公钥</CardTitle>
          <CardAction>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                void navigator.clipboard?.writeText(peer.publicKey);
                setCopied(true);
              }}
            >
              <Copy size={14} /> {copied ? "已复制" : "复制"}
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent>
          <code className="block truncate font-mono text-[11px] text-muted-foreground">
            {peer.publicKey}
          </code>
        </CardContent>
      </Card>
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-5">
        <MetricCard
          label="设备状态"
          value={runtime ? (runtime.online ? "在线" : "离线") : "—"}
          note={
            timestamp(runtime?.last_seen_at) === "—"
              ? "在线期间由所属 Server 记录"
              : `最后活跃 ${timestamp(runtime?.last_seen_at)}`
          }
          icon={<Wifi size={16} />}
        />
        <MetricCard
          label="设备上行"
          value={sample ? `${bytes(sample.rx)}/s` : "—"}
          note={
            runtime?.rx_bytes !== undefined
              ? `累计 ${bytes(runtime.rx_bytes)}`
              : "等待采样"
          }
          icon={<ArrowUpRight size={16} />}
        />
        <MetricCard
          label="设备下行"
          value={sample ? `${bytes(sample.tx)}/s` : "—"}
          note={
            runtime?.tx_bytes !== undefined
              ? `累计 ${bytes(runtime.tx_bytes)}`
              : "等待采样"
          }
          icon={<ArrowDownLeft size={16} />}
        />
        <MetricCard
          label="调试权限"
          value={runtime?.debug_mode ?? "—"}
          note="由设备自行设定"
          icon={<ShieldCheck size={16} />}
        />
        <MetricCard
          label="最后地址"
          value={runtime?.last_addr ?? "—"}
          note="Server 记录的接入地址"
          icon={<BatteryMedium size={16} />}
        />
      </div>
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="traffic">流量</TabsTrigger>
          <TabsTrigger value="chat">对话记录</TabsTrigger>
          <TabsTrigger value="telemetry">Telemetry</TabsTrigger>
          <TabsTrigger value="location">定位</TabsTrigger>
          <TabsTrigger value="state">状态字段</TabsTrigger>
        </TabsList>
        <TabsContent value="traffic">
          <Card>
            <CardHeader>
              <CardTitle>流量趋势</CardTitle>
              <CardDescription>
                Server 记录的设备累计字节，按本次会话的轮询间隔换算成速率
              </CardDescription>
              <CardAction>
                <TrafficLegend />
              </CardAction>
            </CardHeader>
            <CardContent>
              <TrafficChart
                samples={state?.samples ?? []}
                windowSeconds={windowSeconds}
                emptyMessage="等待流量采样"
              />
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="chat">
          <PeerChat peer={peer} />
        </TabsContent>
        <TabsContent value="telemetry">
          <PeerTelemetry peer={peer} />
        </TabsContent>
        <TabsContent value="location">
          <PeerLocation peer={peer} />
        </TabsContent>
        <TabsContent value="state">
          <Card>
            <CardHeader>
              <CardTitle>状态字段</CardTitle>
              <CardDescription>
                设备信息、运行时与 Server
                记录的状态字段：这些只保留最新值，需要历史曲线的指标在 Telemetry
                里查询。
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4 px-0">
              {state?.snapshot ? (
                <>
                  <div className="mx-5 divide-y divide-border overflow-hidden rounded-md border border-border">
                    <div className="grid grid-cols-[5rem_16rem_minmax(0,1fr)] gap-3 px-3 py-1.5 text-[10px] text-muted-foreground">
                      <span>来源</span>
                      <span>字段</span>
                      <span>值</span>
                    </div>
                    {[
                      ["info", state.snapshot.info],
                      ["runtime", state.snapshot.runtime],
                      ["status", state.snapshot.status],
                    ].flatMap(([source, group]) =>
                      statusRows(group).map(([key, value]) => (
                        <div
                          key={`${String(source)}.${key}`}
                          className="grid grid-cols-[5rem_16rem_minmax(0,1fr)] items-baseline gap-3 px-3 py-1.5 hover:bg-secondary/60"
                        >
                          <span className="text-[10px] text-muted-foreground">
                            {String(source)}
                          </span>
                          <span className="truncate font-mono text-[11px]">
                            {key}
                          </span>
                          <span className="font-mono text-xs break-words">
                            {value}
                          </span>
                        </div>
                      )),
                    )}
                  </div>
                  <details className="mx-5 border-t border-border pt-4">
                    <summary className="cursor-pointer text-xs text-muted-foreground">
                      原始 JSON
                    </summary>
                    <pre className="mt-3 max-h-96 overflow-auto text-[11px] leading-relaxed text-muted-foreground">
                      {JSON.stringify(state.snapshot, null, 2)}
                    </pre>
                  </details>
                </>
              ) : (
                <p className="px-5 text-xs text-muted-foreground">
                  尚未读取到数据。
                </p>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </>
  );
}

/**
 * Flattens one status group into printable rows: nested objects become dotted
 * keys, and Unix millisecond or Go zero timestamps render as times.
 */
function statusRows(group: unknown, prefix = ""): [string, string][] {
  if (typeof group !== "object" || group === null) return [];
  const rows: [string, string][] = [];
  for (const [key, value] of Object.entries(group as Record<string, unknown>)) {
    // `details` is an untyped open map; its current contents duplicate the
    // observation times already shown by Telemetry, so it stays in raw JSON.
    if (prefix === "" && key === "details") continue;
    const name = prefix === "" ? key : `${prefix}.${key}`;
    if (value !== null && typeof value === "object" && !Array.isArray(value)) {
      rows.push(...statusRows(value, name));
      continue;
    }
    if (value === null || value === undefined || value === "") {
      rows.push([name, "—"]);
      continue;
    }
    if (
      key.endsWith("_at_unix_ms") ||
      key.endsWith("_at") ||
      key === "reported_at"
    ) {
      const stamp =
        typeof value === "string" && /^\d+$/.test(value)
          ? Number(value)
          : (value as string | number);
      rows.push([name, timestamp(stamp)]);
      continue;
    }
    rows.push([
      name,
      Array.isArray(value) ? JSON.stringify(value) : String(value),
    ]);
  }
  return rows;
}
