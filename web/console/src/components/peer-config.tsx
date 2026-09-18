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
  type DeviceTool,
  type WatchedPeer,
} from "@/lib/peers";
import { nodeErrorMessage } from "@/lib/api";

const onOff = (value: unknown) =>
  typeof value === "boolean" ? (value ? "开启" : "关闭") : undefined;
const percent = (value: unknown) =>
  typeof value === "number" ? `${value}%` : undefined;
const idleMs = (zero: string) => (value: unknown) => {
  if (typeof value !== "number") return undefined;
  if (value === 0) return zero;
  return value % 1000 === 0 ? `${value / 1000} 秒` : `${value} ms`;
};
const choice =
  (names: Record<string, string>) =>
  (value: unknown): string | undefined =>
    typeof value === "string" ? (names[value] ?? value) : undefined;

/** Every member DeviceSettings defines, in contract order. */
export const SETTINGS: [
  string,
  string,
  (value: unknown) => string | undefined,
][] = [
  ["cellular_enabled", "蜂窝网络", onOff],
  ["screen_off_timeout_ms", "息屏时间", idleMs("常亮")],
  ["screen_brightness", "屏幕亮度", percent],
  ["led_brightness", "指示灯亮度", percent],
  [
    "locale",
    "语言",
    (value) => (typeof value === "string" ? value : undefined),
  ],
  [
    "default_interaction_mode",
    "默认交互模式",
    choice({ "push-to-talk": "按键说话", realtime: "实时对话" }),
  ],
  [
    "key_feedback",
    "按键反馈",
    choice({
      none: "无",
      sound: "声音",
      vibrate: "振动",
      sound_and_vibrate: "声音和振动",
    }),
  ],
  [
    "alert_mode",
    "提醒方式",
    choice({ silent: "静音", vibrate: "振动", ring: "响铃" }),
  ],
  ["auto_sleep_timeout_ms", "自动休眠", idleMs("不自动休眠")],
  ["nfc_enabled", "NFC", onOff],
];

/** Display text for one locale, falling back to zh-CN, en, then the name. */
export function toolText(tool: DeviceTool) {
  const text = tool.i18n["zh-CN"] ?? tool.i18n.en;
  return {
    name: text?.display_name ?? tool.name,
    description: text?.description,
  };
}

/** `name: type` per input property; optional ones carry a `?`. */
export function schemaSummary(schema: Record<string, unknown>): string {
  const properties = schema.properties;
  if (typeof properties !== "object" || properties === null) {
    return Object.keys(schema).length === 0 ? "无参数" : "见原始 Schema";
  }
  const required = new Set(
    Array.isArray(schema.required) ? schema.required.map(String) : [],
  );
  const entries = Object.entries(properties as Record<string, unknown>);
  if (entries.length === 0) return "无参数";
  return entries
    .map(([name, property]) => {
      const type =
        typeof property === "object" && property !== null
          ? (property as Record<string, unknown>).type
          : undefined;
      const typeText = Array.isArray(type)
        ? type.join("|")
        : typeof type === "string"
          ? type
          : "any";
      return `${name}${required.has(name) ? "" : "?"}: ${typeText}`;
    })
    .join(", ");
}

function SectionState<T>({
  section,
  render,
}: {
  section: DeviceSection<T> | undefined;
  render: (data: T) => ReactNode;
}) {
  if (section === undefined) {
    return <p className="px-5 text-xs text-muted-foreground">读取中…</p>;
  }
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

/**
 * Read-only view of what the device and its RuntimeProfile expose to the
 * control app. Writes (settings, factory reset, Workspace switch, Tool invoke)
 * belong to the owner's control app, not the monitor.
 */
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
  const settings = config?.settings;
  const methods = config?.rpcMethods;
  const tools = config?.tools;

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>设备设置</CardTitle>
          <CardDescription>
            设备当前的配置，只读展示；修改由设备主人的控制端完成。未返回的项表示设备不支持。
          </CardDescription>
          <CardAction>{refresh}</CardAction>
        </CardHeader>
        <CardContent className="px-0">
          {error && <p className="px-5 text-xs text-destructive">{error}</p>}
          <SectionState
            section={settings}
            render={(values) => {
              const known = new Set(SETTINGS.map(([key]) => key));
              const extra = Object.keys(values).filter(
                (key) => !known.has(key),
              );
              return (
                <div className="mx-5 divide-y divide-border overflow-hidden rounded-md border border-border">
                  {SETTINGS.map(([key, label, format]) => {
                    const present = key in values && values[key] !== null;
                    const text = present
                      ? (format(values[key]) ?? JSON.stringify(values[key]))
                      : undefined;
                    return (
                      <div
                        key={key}
                        className="grid grid-cols-[8rem_minmax(0,1fr)_12rem] items-baseline gap-3 px-3 py-1.5 text-xs max-md:grid-cols-[7rem_minmax(0,1fr)]"
                      >
                        <span>{label}</span>
                        <span
                          className={
                            text === undefined
                              ? "text-muted-foreground"
                              : "font-mono"
                          }
                        >
                          {text ?? "不支持"}
                        </span>
                        <span className="truncate font-mono text-[10px] text-muted-foreground max-md:hidden">
                          {key}
                        </span>
                      </div>
                    );
                  })}
                  {extra.map((key) => (
                    <div
                      key={key}
                      className="grid grid-cols-[8rem_minmax(0,1fr)_12rem] items-baseline gap-3 px-3 py-1.5 text-xs max-md:grid-cols-[7rem_minmax(0,1fr)]"
                    >
                      <span className="truncate font-mono">{key}</span>
                      <span className="font-mono break-words">
                        {JSON.stringify(values[key])}
                      </span>
                      <span className="text-[10px] text-muted-foreground max-md:hidden">
                        未识别的设置
                      </span>
                    </div>
                  ))}
                </div>
              );
            }}
          />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>控制端 Tools</CardTitle>
          <CardDescription>
            设备绑定的 RuntimeProfile 开放给控制端调用的
            Tool；设备离线时也能读取。
          </CardDescription>
        </CardHeader>
        <CardContent className="px-0">
          <SectionState
            section={tools}
            render={(items) => {
              if (items.length === 0) {
                return (
                  <p className="px-5 text-xs text-muted-foreground">
                    没有开放给控制端的 Tool。
                  </p>
                );
              }
              return (
                <div className="mx-5 divide-y divide-border overflow-hidden rounded-md border border-border">
                  {items.map((tool) => {
                    const text = toolText(tool);
                    return (
                      <div
                        key={tool.name}
                        className="flex flex-col gap-1 px-3 py-2"
                      >
                        <div className="flex flex-wrap items-baseline gap-2">
                          <span className="text-xs font-medium">
                            {text.name}
                          </span>
                          <code className="font-mono text-[11px] text-muted-foreground">
                            {tool.name}
                          </code>
                          <Badge variant="outline">
                            权限 {tool.control_access}
                          </Badge>
                        </div>
                        {text.description && (
                          <p className="text-[11px] text-muted-foreground">
                            {text.description}
                          </p>
                        )}
                        <details>
                          <summary className="cursor-pointer font-mono text-[11px] break-words text-muted-foreground">
                            参数 {schemaSummary(tool.input_schema)}
                          </summary>
                          <pre className="mt-2 max-h-64 overflow-auto text-[11px] leading-relaxed text-muted-foreground">
                            {JSON.stringify(tool.input_schema, null, 2)}
                          </pre>
                        </details>
                      </div>
                    );
                  })}
                </div>
              );
            }}
          />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>设备能力</CardTitle>
          <CardDescription>
            固件实现的 RPC 方法
            {methods?.state === "ok" ? `，共 ${methods.data.length} 个` : ""}
            ；控制端据此隐藏设备无法完成的操作。
          </CardDescription>
        </CardHeader>
        <CardContent className="px-0">
          <SectionState
            section={methods}
            render={(names) =>
              names.length === 0 ? (
                <p className="px-5 text-xs text-muted-foreground">
                  设备没有声明任何方法。
                </p>
              ) : (
                <div className="flex flex-wrap gap-1.5 px-5">
                  {names.map((name) => (
                    <code
                      key={name}
                      className="rounded border border-border px-1.5 py-0.5 font-mono text-[11px]"
                    >
                      {name}
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
