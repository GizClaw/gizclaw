import { useCallback, useEffect, useRef, useState } from "react";
import {
  AssistantRuntimeProvider,
  ComposerPrimitive,
  MessagePrimitive,
  ThreadPrimitive,
  useExternalStoreRuntime,
  type AppendMessage,
  type ThreadMessageLike,
  type ToolCallMessagePartProps,
} from "@assistant-ui/react";
import {
  AssistantTurnError,
  createAssistant,
  createGizClawModel,
  type ActionRecord,
  type Model,
} from "@gizclaw/assistant";
import { ArrowUp, Settings, Square, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import type { ConsoleAssistant } from "@/lib/config";

import {
  createConsoleRuntime,
  type ConsoleRuntimeDeps,
} from "./console-runtime";

export type AssistantPanelProps = {
  /** From the console configuration; absent means the chat is not set up. */
  assistant?: ConsoleAssistant;
  runtimeDeps: Omit<ConsoleRuntimeDeps, "signal">;
  /** The node serving /openai/v1 when the assistant names none. */
  endpoint: string;
  onClose(): void;
  onOpenConfig(): void;
  /** Replaced in tests with a scripted model. */
  createModel?: (assistant: ConsoleAssistant, baseURL: string) => Model;
};

type Entry = {
  id: string;
  role: "user" | "assistant";
  text: string;
  actions?: ActionRecord[];
  error?: string;
};

const defaultModel = (assistant: ConsoleAssistant, baseURL: string) =>
  createGizClawModel({
    baseURL,
    apiKey: assistant.apiKey,
    model: assistant.model,
  });

export default function AssistantPanel(props: AssistantPanelProps) {
  const endpoint = props.assistant?.endpoint ?? props.endpoint;
  return (
    <section
      aria-label="诊断助手"
      className="flex h-[min(640px,calc(100vh-6rem))] w-[min(440px,calc(100vw-2rem))] flex-col overflow-hidden rounded-lg border bg-card shadow-xl"
    >
      <header className="flex items-center justify-between border-b px-4 py-3">
        <div>
          <h2 className="text-sm font-medium">诊断助手</h2>
          <p className="text-xs text-muted-foreground">
            {props.assistant
              ? `模型 ${props.assistant.model} · ${new URL(endpoint).host}`
              : "能跳转页面、读日志和设备数据来分析问题"}
          </p>
        </div>
        <Button
          variant="ghost"
          size="sm"
          aria-label="关闭诊断助手"
          onClick={props.onClose}
        >
          <X size={16} />
        </Button>
      </header>
      {props.assistant ? (
        <Conversation
          key={`${props.assistant.apiKey}|${props.assistant.model}|${endpoint}`}
          createModel={() =>
            (props.createModel ?? defaultModel)(
              props.assistant!,
              `${endpoint.replace(/\/+$/, "")}/openai/v1`,
            )
          }
          runtimeDeps={props.runtimeDeps}
        />
      ) : (
        <div className="flex flex-col gap-3 p-4 text-sm">
          <p className="text-xs text-muted-foreground">
            诊断助手使用一台已有设备的 API Key，经节点的 /openai/v1 调用该设备
            RuntimeProfile 里的对话模型。在配置中添加 assistant：
          </p>
          <pre className="rounded-md bg-muted p-3 text-xs">
            {`"assistant": {\n  "apiKey": "gizclaw_sk_v1_...",\n  "model": "llm"\n}`}
          </pre>
          <p className="text-xs text-muted-foreground">
            这把 Key 同时能控制它所属的设备，会随配置加密保存在本浏览器。
          </p>
          <Button variant="outline" size="sm" onClick={props.onOpenConfig}>
            <Settings size={14} />
            编辑配置
          </Button>
        </div>
      )}
    </section>
  );
}

function Conversation({
  createModel,
  runtimeDeps,
}: {
  createModel(): Model;
  runtimeDeps: Omit<ConsoleRuntimeDeps, "signal">;
}) {
  const [entries, setEntries] = useState<Entry[]>([]);
  const [running, setRunning] = useState(false);
  const controller = useRef<AbortController>(new AbortController());
  const sequence = useRef(0);

  // One session per mounted conversation; the panel remounts it when the
  // configured assistant changes. runtimeDeps reads the console's latest state
  // through refs at call time.
  const [assistant] = useState(() =>
    createAssistant({
      model: createModel(),
      runtime: createConsoleRuntime({
        ...runtimeDeps,
        signal: () => controller.current.signal,
      }),
    }),
  );

  useEffect(() => () => controller.current.abort(), []);

  const onNew = useCallback(
    async (message: AppendMessage) => {
      const text = message.content
        .map((part) => (part.type === "text" ? part.text : ""))
        .join("")
        .trim();
      if (text === "") return;
      const id = () => `m${++sequence.current}`;
      setEntries((current) => [...current, { id: id(), role: "user", text }]);
      controller.current = new AbortController();
      setRunning(true);
      try {
        const turn = await assistant.send(text, {
          signal: controller.current.signal,
        });
        setEntries((current) => [
          ...current,
          {
            id: id(),
            role: "assistant",
            text: turn.reply,
            actions: turn.actions,
          },
        ]);
      } catch (cause) {
        const actions =
          cause instanceof AssistantTurnError ? cause.actions : [];
        const stopped = controller.current.signal.aborted;
        setEntries((current) => [
          ...current,
          {
            id: id(),
            role: "assistant",
            text: "",
            actions,
            error: stopped
              ? "已停止。"
              : `助手出错：${cause instanceof Error ? cause.message : String(cause)}`,
          },
        ]);
      } finally {
        setRunning(false);
      }
    },
    [assistant],
  );

  const runtime = useExternalStoreRuntime<Entry>({
    messages: entries,
    isRunning: running,
    onNew,
    onCancel: async () => controller.current.abort(),
    convertMessage,
  });

  return (
    <AssistantRuntimeProvider runtime={runtime}>
      <ThreadPrimitive.Root className="flex min-h-0 flex-1 flex-col">
        <ThreadPrimitive.Viewport className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-4 py-3">
          <ThreadPrimitive.Empty>
            <p className="text-xs text-muted-foreground">
              问我节点、设备或日志的问题，例如"哪个节点最忙？""这台设备最近有什么错误？"。我会先查数据、必要时帮你跳转页面，再给出分析。
            </p>
          </ThreadPrimitive.Empty>
          <ThreadPrimitive.Messages
            components={{ UserMessage, AssistantMessage }}
          />
        </ThreadPrimitive.Viewport>
        <ComposerPrimitive.Root className="flex items-end gap-2 border-t p-3">
          <ComposerPrimitive.Input
            aria-label="向诊断助手提问"
            placeholder="描述你遇到的问题…"
            rows={1}
            className="max-h-32 min-h-9 flex-1 resize-none rounded-md border bg-background px-3 py-2 text-sm outline-none focus-visible:ring-1 focus-visible:ring-ring"
          />
          {running ? (
            <ComposerPrimitive.Cancel asChild>
              <Button size="sm" variant="outline" aria-label="停止">
                <Square size={14} />
              </Button>
            </ComposerPrimitive.Cancel>
          ) : (
            <ComposerPrimitive.Send asChild>
              <Button size="sm" aria-label="发送">
                <ArrowUp size={14} />
              </Button>
            </ComposerPrimitive.Send>
          )}
        </ComposerPrimitive.Root>
      </ThreadPrimitive.Root>
    </AssistantRuntimeProvider>
  );
}

function convertMessage(entry: Entry): ThreadMessageLike {
  if (entry.role === "user") {
    return { id: entry.id, role: "user", content: entry.text };
  }
  const tools = (entry.actions ?? []).map((action, index) => ({
    type: "tool-call" as const,
    toolCallId: `${entry.id}-${index}`,
    toolName: action.tool,
    argsText: JSON.stringify(action.arguments ?? {}),
    result: action.error ?? action.result,
    isError: action.error !== undefined,
  }));
  const text = entry.error ?? entry.text;
  return {
    id: entry.id,
    role: "assistant",
    content: text ? [...tools, { type: "text" as const, text }] : tools,
    status: entry.error
      ? { type: "incomplete", reason: "error" }
      : { type: "complete", reason: "stop" },
  };
}

function UserMessage() {
  return (
    <MessagePrimitive.Root className="ml-8 self-end rounded-lg bg-muted px-3 py-2 text-sm whitespace-pre-wrap">
      <MessagePrimitive.Parts />
    </MessagePrimitive.Root>
  );
}

function AssistantMessage() {
  return (
    <MessagePrimitive.Root className="flex flex-col gap-2 text-sm">
      <MessagePrimitive.Parts
        components={{
          Text: ({ text }) => <p className="whitespace-pre-wrap">{text}</p>,
          tools: { Fallback: ActionCard },
        }}
      />
    </MessagePrimitive.Root>
  );
}

const TOOL_LABELS: Record<string, string> = {
  get_current_page: "读取当前页面",
  navigate: "跳转页面",
  open_link: "打开链接",
  list_nodes: "读取节点状态",
  get_node_traffic: "读取节点流量",
  list_devices: "读取关注设备",
  find_device: "查找设备",
  get_device_status: "读取设备状态",
  get_device_telemetry: "读取设备 telemetry",
  query_device_telemetry: "读取 telemetry 走势",
  get_device_wifi: "读取设备 Wi-Fi",
  list_device_workspaces: "读取对话 Workspace",
  get_conversation_history: "读取对话历史",
  search_logs: "查询设备日志",
};

/** One action the assistant took, collapsed to a line with its outcome. */
function ActionCard({
  toolName,
  argsText,
  result,
  isError,
}: ToolCallMessagePartProps) {
  const label = TOOL_LABELS[toolName] ?? toolName;
  const navigated =
    toolName === "navigate" && !isError
      ? (result as { hash?: string } | undefined)?.hash
      : undefined;
  const failure = isError
    ? (result as { code?: string } | undefined)?.code
    : undefined;
  return (
    <details className="rounded-md border px-3 py-2 text-xs">
      <summary className="cursor-pointer text-muted-foreground">
        {label}
        {navigated ? ` → ${navigated}` : ""}
        {failure ? (
          <span className="ml-1 text-destructive">（{failure}）</span>
        ) : null}
      </summary>
      <pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap break-all text-[11px] text-muted-foreground">
        {argsText}
        {"\n"}
        {JSON.stringify(result, null, 2)}
      </pre>
    </details>
  );
}
