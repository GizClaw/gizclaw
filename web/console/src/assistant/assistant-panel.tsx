import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
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
  BUILTIN_KNOWLEDGE,
  createAssistant,
  createGizClawModel,
  createKnowledgeIndex,
  type Compaction,
  type KnowledgeDocument,
  type KnowledgeIndex,
  type Model,
} from "@gizclaw/assistant";
import {
  ArrowUp,
  BookOpen,
  MessagesSquare,
  Settings,
  Square,
  SquarePen,
  Trash2,
  Upload,
  X,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import type { ConsoleAssistant } from "@/lib/config";
import { assistantRecords } from "@/lib/store";

import {
  createAssistantStore,
  knowledgeDocument,
  MAX_KNOWLEDGE_FILE_BYTES,
  threadTitle,
  type AssistantStore,
  type ChatEntry,
  type StoredThread,
  type ThreadSummary,
} from "./assistant-store";
import { createConsoleRuntime, type ConsoleStateDeps } from "./console-runtime";

export type AssistantPanelProps = {
  /** From the console configuration; absent means the chat is not set up. */
  assistant?: ConsoleAssistant;
  runtimeDeps: ConsoleStateDeps;
  /** The node serving /openai/v1 when the assistant names none. */
  endpoint: string;
  onClose(): void;
  onOpenConfig(): void;
  /** Replaced in tests with a scripted model. */
  createModel?: (assistant: ConsoleAssistant, baseURL: string) => Model;
  /** Conversations and imported knowledge; the encrypted browser store by default. */
  store?: AssistantStore;
};

type View = "chat" | "threads" | "knowledge";

const defaultModel = (assistant: ConsoleAssistant, baseURL: string) =>
  createGizClawModel({
    baseURL,
    apiKey: assistant.apiKey,
    model: assistant.model,
  });

const newThread = (): StoredThread => {
  const now = Date.now();
  return {
    id: crypto.randomUUID(),
    title: "新对话",
    createdAt: now,
    updatedAt: now,
    entries: [],
    history: [],
  };
};

const messageOf = (cause: unknown) =>
  cause instanceof Error ? cause.message : String(cause);

export default function AssistantPanel(props: AssistantPanelProps) {
  const endpoint = props.assistant?.endpoint ?? props.endpoint;
  const [store] = useState(
    () => props.store ?? createAssistantStore(assistantRecords),
  );
  return (
    <section
      aria-label="诊断助手"
      className="flex h-[min(640px,calc(100vh-6rem))] w-[min(440px,calc(100vw-2rem))] flex-col overflow-hidden rounded-lg border bg-card shadow-xl"
    >
      {props.assistant ? (
        <Workspace
          assistant={props.assistant}
          endpoint={endpoint}
          store={store}
          runtimeDeps={props.runtimeDeps}
          createModel={props.createModel ?? defaultModel}
          onClose={props.onClose}
        />
      ) : (
        <>
          <Header
            subtitle="能跳转页面、读日志和设备数据来分析问题"
            onClose={props.onClose}
          />
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
        </>
      )}
    </section>
  );
}

function Header({
  subtitle,
  children,
  onClose,
}: {
  subtitle: string;
  children?: ReactNode;
  onClose(): void;
}) {
  return (
    <header className="flex items-center justify-between gap-2 border-b px-4 py-3">
      <div className="min-w-0">
        <h2 className="text-sm font-medium">诊断助手</h2>
        <p className="truncate text-xs text-muted-foreground">{subtitle}</p>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {children}
        <Button
          variant="ghost"
          size="sm"
          aria-label="关闭诊断助手"
          onClick={onClose}
        >
          <X size={16} />
        </Button>
      </div>
    </header>
  );
}

function Workspace({
  assistant,
  endpoint,
  store,
  runtimeDeps,
  createModel,
  onClose,
}: {
  assistant: ConsoleAssistant;
  endpoint: string;
  store: AssistantStore;
  runtimeDeps: ConsoleStateDeps;
  createModel(assistant: ConsoleAssistant, baseURL: string): Model;
  onClose(): void;
}) {
  const [view, setView] = useState<View>("chat");
  const [threads, setThreads] = useState<ThreadSummary[]>([]);
  const [thread, setThread] = useState<StoredThread>();
  const [documents, setDocuments] = useState<KnowledgeDocument[]>([]);
  const [problem, setProblem] = useState<string>();
  // A turn still running when its thread is deleted saves on abort; those
  // saves are dropped so the thread stays deleted.
  const deleted = useRef(new Set<string>());

  const index = useMemo(
    () => createKnowledgeIndex([...BUILTIN_KNOWLEDGE, ...documents]),
    [documents],
  );
  const indexRef = useRef(index);
  indexRef.current = index;
  const knowledge = useCallback(() => indexRef.current, []);

  const report = useCallback(
    (action: string) => (cause: unknown) =>
      setProblem(`${action}失败：${messageOf(cause)}`),
    [],
  );

  useEffect(() => {
    let active = true;
    (async () => {
      try {
        const [list, imported] = await Promise.all([
          store.listThreads(),
          store.listKnowledge(),
        ]);
        const latest = list[0] && (await store.loadThread(list[0].id));
        if (!active) return;
        setThreads(list);
        setDocuments(imported);
        setThread(latest ?? newThread());
      } catch (cause) {
        if (!active) return;
        report("读取本地对话")(cause);
        setThread(newThread());
      }
    })();
    return () => {
      active = false;
    };
  }, [store, report]);

  const save = useCallback(
    (next: StoredThread) => {
      if (deleted.current.has(next.id)) return;
      // Keep the open thread current, so a conversation remounted for a new
      // assistant configuration resumes from its latest turn.
      setThread((current) => (current?.id === next.id ? next : current));
      store.saveThread(next).then(setThreads, report("保存对话"));
    },
    [store, report],
  );

  const open = async (id: string) => {
    setView("chat");
    if (id === thread?.id) return;
    try {
      setThread((await store.loadThread(id)) ?? newThread());
    } catch (cause) {
      report("读取对话")(cause);
    }
  };

  // A thread is saved with its first message, so an unused one leaves no trace.
  const startNew = () => {
    setView("chat");
    setThread(newThread());
  };

  const remove = async (id: string) => {
    deleted.current.add(id);
    if (id === thread?.id) setThread(newThread());
    try {
      setThreads(await store.deleteThread(id));
    } catch (cause) {
      report("删除对话")(cause);
    }
  };

  const clearAll = async () => {
    for (const item of threads) deleted.current.add(item.id);
    if (thread) deleted.current.add(thread.id);
    setThread(newThread());
    setView("chat");
    try {
      await store.clearThreads();
      setThreads([]);
    } catch (cause) {
      report("清空对话")(cause);
    }
  };

  const updateKnowledge = async (next: KnowledgeDocument[]) => {
    try {
      await store.saveKnowledge(next);
      setDocuments(next);
    } catch (cause) {
      report("保存知识库")(cause);
    }
  };

  const toggle = (target: View) =>
    setView((current) => (current === target ? "chat" : target));

  return (
    <>
      <Header
        subtitle={`模型 ${assistant.model} · ${new URL(endpoint).host}`}
        onClose={onClose}
      >
        <Button
          variant={view === "threads" ? "secondary" : "ghost"}
          size="sm"
          aria-label="历史对话"
          title="历史对话"
          aria-pressed={view === "threads"}
          onClick={() => toggle("threads")}
        >
          <MessagesSquare size={16} />
        </Button>
        <Button
          variant="ghost"
          size="sm"
          aria-label="新对话"
          title="新对话"
          onClick={startNew}
        >
          <SquarePen size={16} />
        </Button>
        <Button
          variant={view === "knowledge" ? "secondary" : "ghost"}
          size="sm"
          aria-label="知识库"
          title="知识库"
          aria-pressed={view === "knowledge"}
          onClick={() => toggle("knowledge")}
        >
          <BookOpen size={16} />
        </Button>
      </Header>
      {problem && (
        <p
          role="alert"
          className="flex items-start justify-between gap-2 border-b bg-destructive/10 px-4 py-2 text-xs text-destructive"
        >
          {problem}
          <button
            type="button"
            aria-label="关闭提示"
            onClick={() => setProblem(undefined)}
          >
            <X size={12} />
          </button>
        </p>
      )}
      {view === "threads" && (
        <ThreadList
          threads={threads}
          currentId={thread?.id}
          onOpen={open}
          onRemove={remove}
          onClear={clearAll}
        />
      )}
      {view === "knowledge" && (
        <KnowledgeView
          documents={documents}
          passages={index.size}
          onChange={updateKnowledge}
          onProblem={setProblem}
        />
      )}
      {/* The conversation stays mounted behind the other views so a running
          turn is not stopped by looking at the history or the knowledge. */}
      <div hidden={view !== "chat"} className="flex min-h-0 flex-1 flex-col">
        {thread ? (
          <Conversation
            key={`${assistant.apiKey}|${assistant.model}|${endpoint}|${thread.id}`}
            thread={thread}
            createModel={() =>
              createModel(
                assistant,
                `${endpoint.replace(/\/+$/, "")}/openai/v1`,
              )
            }
            contextTokens={assistant.contextTokens}
            runtimeDeps={runtimeDeps}
            knowledge={knowledge}
            onSave={save}
          />
        ) : (
          <p className="p-4 text-xs text-muted-foreground">正在读取对话…</p>
        )}
      </div>
    </>
  );
}

function ThreadList({
  threads,
  currentId,
  onOpen,
  onRemove,
  onClear,
}: {
  threads: ThreadSummary[];
  currentId?: string;
  onOpen(id: string): void;
  onRemove(id: string): void;
  onClear(): void;
}) {
  const [confirming, setConfirming] = useState(false);
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <ul aria-label="历史对话列表" className="flex-1 overflow-y-auto p-2">
        {threads.length === 0 && (
          <li className="p-2 text-xs text-muted-foreground">
            还没有保存的对话。对话会加密保存在本浏览器。
          </li>
        )}
        {threads.map((item) => (
          <li
            key={item.id}
            className={`flex items-center gap-1 rounded-md ${item.id === currentId ? "bg-muted" : ""}`}
          >
            <button
              type="button"
              className="min-w-0 flex-1 px-2 py-2 text-left"
              onClick={() => onOpen(item.id)}
            >
              <span className="block truncate text-sm">{item.title}</span>
              <span className="block text-xs text-muted-foreground">
                {new Date(item.updatedAt).toLocaleString()}
              </span>
            </button>
            <Button
              variant="ghost"
              size="sm"
              aria-label={`删除对话 ${item.title}`}
              onClick={() => onRemove(item.id)}
            >
              <Trash2 size={14} />
            </Button>
          </li>
        ))}
      </ul>
      {threads.length > 0 && (
        <div className="flex justify-end gap-2 border-t p-3">
          {confirming && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setConfirming(false)}
            >
              取消
            </Button>
          )}
          <Button
            variant={confirming ? "destructive" : "outline"}
            size="sm"
            onClick={() => {
              if (!confirming) {
                setConfirming(true);
                return;
              }
              setConfirming(false);
              onClear();
            }}
          >
            {confirming ? "确认清空全部对话" : "清空全部对话"}
          </Button>
        </div>
      )}
    </div>
  );
}

function KnowledgeView({
  documents,
  passages,
  onChange,
  onProblem,
}: {
  documents: KnowledgeDocument[];
  passages: number;
  onChange(documents: KnowledgeDocument[]): void;
  onProblem(problem: string): void;
}) {
  const input = useRef<HTMLInputElement>(null);

  const importFiles = async (files: FileList | null) => {
    if (!files || files.length === 0) return;
    let next = documents;
    for (const file of Array.from(files)) {
      if (file.size > MAX_KNOWLEDGE_FILE_BYTES) {
        onProblem(`${file.name} 超过 512 KB，未导入`);
        continue;
      }
      const document = knowledgeDocument(
        file.name,
        await file.text(),
        crypto.randomUUID(),
      );
      // Importing a file with the same name replaces the earlier version.
      next = [...next.filter((item) => item.source !== file.name), document];
    }
    if (next !== documents) onChange(next);
    if (input.current) input.current.value = "";
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-col gap-2 border-b p-3 text-xs text-muted-foreground">
        <p>
          诊断助手回答错误码、调试模式等问题时会先检索知识库。可以导入团队的排障手册（Markdown
          或纯文本），文档加密保存在本浏览器，按标题切分后检索。
        </p>
        <div className="flex items-center justify-between">
          <span>共 {passages} 个段落</span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => input.current?.click()}
          >
            <Upload size={14} />
            导入文档
          </Button>
          <input
            ref={input}
            type="file"
            accept=".md,.markdown,.txt,text/markdown,text/plain"
            multiple
            hidden
            aria-label="选择知识库文档"
            onChange={(event) => void importFiles(event.target.files)}
          />
        </div>
      </div>
      <ul aria-label="知识库文档" className="flex-1 overflow-y-auto p-2">
        {documents.map((item) => (
          <li key={item.id} className="flex items-center gap-1">
            <div className="min-w-0 flex-1 px-2 py-1.5">
              <span className="block truncate text-sm">{item.title}</span>
              <span className="block text-xs text-muted-foreground">
                {item.source}
              </span>
            </div>
            <Button
              variant="ghost"
              size="sm"
              aria-label={`删除文档 ${item.title}`}
              onClick={() =>
                onChange(documents.filter((other) => other.id !== item.id))
              }
            >
              <Trash2 size={14} />
            </Button>
          </li>
        ))}
        {BUILTIN_KNOWLEDGE.map((item) => (
          <li key={item.id} className="px-2 py-1.5">
            <span className="block truncate text-sm">{item.title}</span>
            <span className="block text-xs text-muted-foreground">内置</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function compactionNotice(compaction: Compaction): string {
  return compaction.summarizedTurns > 0
    ? `较早的 ${compaction.summarizedTurns} 轮对话已压缩成摘要，以控制上下文长度。`
    : `较早对话中的 ${compaction.trimmedResults} 个查询结果已截短，以控制上下文长度。`;
}

function Conversation({
  thread,
  createModel,
  contextTokens,
  runtimeDeps,
  knowledge,
  onSave,
}: {
  thread: StoredThread;
  createModel(): Model;
  contextTokens?: number;
  runtimeDeps: ConsoleStateDeps;
  knowledge(): KnowledgeIndex;
  onSave(thread: StoredThread): void;
}) {
  const [entries, setEntries] = useState<ChatEntry[]>(thread.entries);
  // The latest entries, read when a turn finishes to save the thread.
  const latest = useRef(thread.entries);
  const commit = useCallback((next: ChatEntry[]) => {
    latest.current = next;
    setEntries(next);
  }, []);
  const [running, setRunning] = useState(false);
  const controller = useRef<AbortController>(new AbortController());

  // One session per mounted conversation, resumed from the saved history; the
  // panel remounts it for another thread or assistant configuration.
  // runtimeDeps reads the console's latest state through refs at call time.
  const [assistant] = useState(() =>
    createAssistant({
      model: createModel(),
      runtime: createConsoleRuntime({
        ...runtimeDeps,
        signal: () => controller.current.signal,
        knowledge,
      }),
      history: thread.history,
      contextTokens,
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
      const id = () => crypto.randomUUID();
      commit([...latest.current, { id: id(), role: "user", text }]);
      controller.current = new AbortController();
      setRunning(true);
      try {
        const turn = await assistant.send(text, {
          signal: controller.current.signal,
        });
        commit([
          ...latest.current,
          ...(turn.compaction
            ? [
                {
                  id: id(),
                  role: "notice" as const,
                  text: compactionNotice(turn.compaction),
                },
              ]
            : []),
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
        commit([
          ...latest.current,
          {
            id: id(),
            role: "assistant",
            text: "",
            actions,
            error: stopped ? "已停止。" : `助手出错：${messageOf(cause)}`,
          },
        ]);
      } finally {
        setRunning(false);
        onSave({
          ...thread,
          title: threadTitle(latest.current),
          updatedAt: Date.now(),
          entries: latest.current,
          history: assistant.history(),
        });
      }
    },
    [assistant, thread, onSave, commit],
  );

  const runtime = useExternalStoreRuntime<ChatEntry>({
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
            components={{ UserMessage, AssistantMessage, SystemMessage }}
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

function convertMessage(entry: ChatEntry): ThreadMessageLike {
  if (entry.role === "user") {
    return { id: entry.id, role: "user", content: entry.text };
  }
  if (entry.role === "notice") {
    return { id: entry.id, role: "system", content: entry.text };
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

function SystemMessage() {
  return (
    <MessagePrimitive.Root className="self-center text-center text-xs text-muted-foreground">
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
  search_knowledge: "检索知识库",
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
