import { useEffect, useState, type ReactNode } from "react";
import { RefreshCw } from "lucide-react";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  loadDeviceConfig,
  type DeviceConfig,
  type DeviceSection,
  type WatchedPeer,
} from "@/lib/peers";
import { nodeErrorMessage } from "@/lib/api";

function SectionState<T>({
  section,
  render,
}: {
  section: DeviceSection<T> | undefined;
  render: (data: T) => ReactNode;
}) {
  if (section === undefined)
    return <p className="px-5 text-xs text-muted-foreground">读取中…</p>;
  if (section.state === "ok") return render(section.data);
  const [badge, text] =
    section.state === "offline"
      ? ["设备离线", "设备当前没有连接，上线后刷新即可读取。"]
      : section.state === "unsupported"
        ? ["设备不支持", "当前固件没有实现这个接口。"]
        : ["读取失败", section.message];
  return (
    <div className="flex flex-wrap items-center gap-2 px-5 text-xs text-muted-foreground">
      <Badge variant={section.state === "error" ? "destructive" : "outline"}>
        {badge}
      </Badge>
      <span className="break-words">{text}</span>
    </div>
  );
}

/** Read-only view of the bound hardware manifest and installed device procedures. */
export function PeerConfig({ peer }: { peer: WatchedPeer }) {
  const [config, setConfig] = useState<DeviceConfig>();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [reload, setReload] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    loadDeviceConfig(peer.endpoint, peer.publicKey, controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return;
        setConfig(result);
        setError("");
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setError(nodeErrorMessage(failure));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [peer.endpoint, peer.publicKey, reload]);

  const refresh = (
    <Button
      variant="outline"
      size="sm"
      disabled={loading}
      onClick={() => setReload((value) => value + 1)}
    >
      <RefreshCw size={13} /> {loading ? "读取中" : "刷新"}
    </Button>
  );

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>硬件状态清单</CardTitle>
          <CardDescription>
            当前 RuntimeProfile 声明的 MHS v0 设备与状态。
          </CardDescription>
          <CardAction>{refresh}</CardAction>
        </CardHeader>
        <CardContent className="px-0">
          {error && <p className="px-5 text-xs text-destructive">{error}</p>}
          <SectionState
            section={config?.manifest}
            render={(manifest) =>
              manifest.devices.length === 0 ? (
                <p className="px-5 text-xs text-muted-foreground">
                  未声明硬件状态。
                </p>
              ) : (
                <div className="mx-5 divide-y divide-border rounded-md border border-border">
                  {manifest.devices.map((device) => (
                    <div key={device.id} className="px-3 py-2 text-xs">
                      <div className="font-medium">
                        {device.id}{" "}
                        <span className="text-muted-foreground">
                          {device.kind}
                        </span>
                      </div>
                      {device.description && (
                        <p className="text-muted-foreground">
                          {device.description}
                        </p>
                      )}
                      <div className="mt-1 flex flex-wrap gap-1">
                        {device.states.map((state) => (
                          <code
                            key={state.name}
                            className="rounded border px-1.5 py-0.5"
                          >
                            {state.name}: {state.type} · {state.access}
                          </code>
                        ))}
                      </div>
                    </div>
                  ))}
                </div>
              )
            }
          />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>设备过程</CardTitle>
          <CardDescription>
            设备当前通过 tool/v0 声明的预定义过程；离线时需上线后刷新。
          </CardDescription>
        </CardHeader>
        <CardContent className="px-0">
          <SectionState
            section={config?.tools}
            render={(tools) =>
              tools.length === 0 ? (
                <p className="px-5 text-xs text-muted-foreground">
                  设备未安装可调用的过程。
                </p>
              ) : (
                <div className="flex flex-wrap gap-1.5 px-5">
                  {tools.map((tool) => (
                    <code
                      key={tool}
                      className="rounded border px-1.5 py-0.5 font-mono text-[11px]"
                    >
                      {tool}
                    </code>
                  ))}
                </div>
              )
            }
          />
        </CardContent>
      </Card>
    </div>
  );
}
