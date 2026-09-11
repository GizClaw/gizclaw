import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";
import { ArrowLeft, Pause, Play, Search, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { cn } from "@/lib/utils";
import { nodeErrorMessage } from "@/lib/api";
import {
  loadHistory,
  loadHistoryAudio,
  loadWorkspaces,
  type WatchedPeer,
} from "@/lib/peers";

type Workspaces = Awaited<ReturnType<typeof loadWorkspaces>>;
type HistoryItem = Awaited<ReturnType<typeof loadHistory>>["items"][number];

/**
 * A contiguous slice of History, newest first. Both edges page on the server:
 * `older` is the descending next_cursor, and newer entries are read in
 * ascending order from the topmost loaded entry.
 */
type Timeline = {
  items: HistoryItem[];
  older?: string;
  hasNewer: boolean;
};

/** Search hits, newest first, paged toward older matches. */
type Results = {
  items: HistoryItem[];
  older?: string;
};

/** Where the timeline starts: the newest entry, or one entry and its context. */
type Anchor = { kind: "latest" } | { kind: "entry"; entry: HistoryItem };

// Scrolling within this many pixels of an edge loads the next page there.
const EDGE_PX = 48;
// Context loaded around a located entry before edge paging takes over.
const CONTEXT_NEWER = 20;
const CONTEXT_OLDER = 40;

/** Persisted Workspace history: what the device and the agent actually said. */
export function PeerChat({ peer }: { peer: WatchedPeer }) {
  const [workspaces, setWorkspaces] = useState<Workspaces>([]);
  const [workspace, setWorkspace] = useState("");
  const [anchor, setAnchor] = useState<Anchor>({ kind: "latest" });
  const [timeline, setTimeline] = useState<Timeline>();
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [results, setResults] = useState<Results>();
  const [showResults, setShowResults] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [resultsBusy, setResultsBusy] = useState(false);
  const [paging, setPaging] = useState<"older" | "newer" | "results">();
  const scroller = useRef<HTMLDivElement>(null);
  // One edge request at a time; a new query aborts it so a stale page can
  // never be stitched onto a different list.
  const pending = useRef<AbortController>(null);
  // Prepending newer entries must not move what the reader is looking at.
  const keep = useRef<{ height: number; top: number }>(null);
  // A located entry is scrolled into the middle once its context renders.
  const reveal = useRef<string>(null);

  useEffect(() => {
    const controller = new AbortController();
    setError("");
    setLoading(true);
    setWorkspaces([]);
    setWorkspace("");
    setTimeline(undefined);
    loadWorkspaces(peer.endpoint, peer.publicKey, controller.signal)
      .then((items) => {
        if (controller.signal.aborted) return;
        setWorkspaces(items);
        setWorkspace(items[0]?.id ?? "");
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setError(nodeErrorMessage(failure));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [peer.endpoint, peer.publicKey]);

  const abortPaging = () => {
    pending.current?.abort();
    pending.current = null;
    setPaging(undefined);
  };

  useEffect(() => {
    if (workspace === "") return;
    const controller = new AbortController();
    abortPaging();
    setBusy(true);
    setError("");
    const load = async (): Promise<Timeline> => {
      if (anchor.kind === "latest") {
        const page = await loadHistory(
          peer.endpoint,
          peer.publicKey,
          workspace,
          { query: "", order: "desc" },
          controller.signal,
        );
        return {
          items: page.items,
          older: page.has_next ? page.next_cursor : undefined,
          hasNewer: false,
        };
      }
      // An entry's own name is an exclusive cursor in both directions, so the
      // two reads return exactly what surrounds it.
      const name = anchor.entry.name;
      const [newer, older] = await Promise.all([
        loadHistory(
          peer.endpoint,
          peer.publicKey,
          workspace,
          { query: "", order: "asc", cursor: name, limit: CONTEXT_NEWER },
          controller.signal,
        ),
        loadHistory(
          peer.endpoint,
          peer.publicKey,
          workspace,
          { query: "", order: "desc", cursor: name, limit: CONTEXT_OLDER },
          controller.signal,
        ),
      ]);
      return {
        items: [...[...newer.items].reverse(), anchor.entry, ...older.items],
        older: older.has_next ? older.next_cursor : undefined,
        hasNewer: newer.has_next,
      };
    };
    load()
      .then((result) => {
        if (controller.signal.aborted) return;
        reveal.current = anchor.kind === "entry" ? anchor.entry.name : null;
        setTimeline(result);
        if (anchor.kind === "latest" && scroller.current)
          scroller.current.scrollTop = 0;
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setError(nodeErrorMessage(failure));
      })
      .finally(() => {
        if (!controller.signal.aborted) setBusy(false);
      });
    return () => {
      controller.abort();
      pending.current?.abort();
      pending.current = null;
    };
  }, [peer.endpoint, peer.publicKey, workspace, anchor]);

  useEffect(() => {
    if (workspace === "" || search === "") {
      setResults(undefined);
      return;
    }
    const controller = new AbortController();
    abortPaging();
    setResultsBusy(true);
    setError("");
    loadHistory(
      peer.endpoint,
      peer.publicKey,
      workspace,
      { query: search, order: "desc" },
      controller.signal,
    )
      .then((page) => {
        if (controller.signal.aborted) return;
        setResults({
          items: page.items,
          older: page.has_next ? page.next_cursor : undefined,
        });
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setError(nodeErrorMessage(failure));
      })
      .finally(() => {
        if (!controller.signal.aborted) setResultsBusy(false);
      });
    return () => controller.abort();
  }, [peer.endpoint, peer.publicKey, workspace, search]);

  useLayoutEffect(() => {
    const element = scroller.current;
    if (!element) return;
    const saved = keep.current;
    if (saved) {
      keep.current = null;
      element.scrollTop = saved.top + (element.scrollHeight - saved.height);
      return;
    }
    const name = reveal.current;
    if (name === null) return;
    reveal.current = null;
    const row = element.querySelector<HTMLElement>(
      `[data-history="${CSS.escape(name)}"]`,
    );
    if (row) {
      element.scrollTop =
        row.offsetTop - element.clientHeight / 2 + row.offsetHeight / 2;
    }
  }, [timeline, showResults]);

  const page = (
    kind: "older" | "newer" | "results",
    request: Parameters<typeof loadHistory>[3],
    apply: (items: HistoryItem[], hasNext: boolean, next?: string) => void,
  ) => {
    if (busy || resultsBusy || pending.current) return;
    const controller = new AbortController();
    pending.current = controller;
    setPaging(kind);
    loadHistory(
      peer.endpoint,
      peer.publicKey,
      workspace,
      request,
      controller.signal,
    )
      .then((result) => {
        if (controller.signal.aborted) return;
        apply(result.items, result.has_next, result.next_cursor);
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setError(nodeErrorMessage(failure));
      })
      .finally(() => {
        if (pending.current !== controller) return;
        pending.current = null;
        setPaging(undefined);
      });
  };

  const loadOlder = () => {
    if (!timeline?.older) return;
    page(
      "older",
      { query: "", order: "desc", cursor: timeline.older },
      (items, hasNext, next) =>
        setTimeline(
          (current) =>
            current && {
              ...current,
              items: [...current.items, ...items],
              older: hasNext ? next : undefined,
            },
        ),
    );
  };

  const loadNewer = () => {
    const top = timeline?.items[0]?.name;
    if (!timeline?.hasNewer || top === undefined) return;
    page(
      "newer",
      { query: "", order: "asc", cursor: top },
      (items, hasNext) => {
        const element = scroller.current;
        if (element) {
          keep.current = {
            height: element.scrollHeight,
            top: element.scrollTop,
          };
        }
        // The server returns this page oldest first; it sits above the current
        // newest entry, so its newest entry goes on top.
        const newer = [...items].reverse();
        setTimeline(
          (current) =>
            current && {
              ...current,
              items: [...newer, ...current.items],
              hasNewer: hasNext,
            },
        );
      },
    );
  };

  const loadMoreResults = () => {
    if (!results?.older) return;
    page(
      "results",
      { query: search, order: "desc", cursor: results.older },
      (items, hasNext, next) =>
        setResults(
          (current) =>
            current && {
              items: [...current.items, ...items],
              older: hasNext ? next : undefined,
            },
        ),
    );
  };

  const onScroll = () => {
    const element = scroller.current;
    if (!element) return;
    const nearBottom =
      element.scrollHeight - element.scrollTop - element.clientHeight < EDGE_PX;
    if (showResults) {
      if (nearBottom) loadMoreResults();
      return;
    }
    if (element.scrollTop < EDGE_PX) loadNewer();
    if (nearBottom) loadOlder();
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const text = query.trim();
    setSearch(text);
    setShowResults(text !== "");
  };

  const clearSearch = () => {
    setQuery("");
    setSearch("");
    setShowResults(false);
  };

  const locate = (entry: HistoryItem) => {
    setShowResults(false);
    setAnchor({ kind: "entry", entry });
  };

  const selected = workspaces.find((item) => item.id === workspace);
  const focused = anchor.kind === "entry" ? anchor.entry.name : undefined;
  return (
    <Card>
      <CardHeader>
        <CardTitle>对话记录</CardTitle>
        <CardDescription>
          {
            "该设备拥有的 Workspace 及其持久化聊天记录，最新的在最上面，向上滚动读取更新的记录，向下滚动读取更早的记录。搜索会列出命中的记录，点击一条即可定位到那个时间点并查看前后上下文。"
          }
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {error && (
          <Alert>
            <AlertTitle>读取失败</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        {loading ? (
          <p className="text-xs text-muted-foreground">正在读取 Workspace…</p>
        ) : workspaces.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            这台设备还没有可展示的 Workspace。
          </p>
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <Select
                aria-label="Workspace"
                value={workspace}
                onChange={(event) => {
                  setWorkspace(event.target.value);
                  clearSearch();
                  setAnchor({ kind: "latest" });
                  setTimeline(undefined);
                }}
              >
                {workspaces.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name} ·{" "}
                    {[item.collection, item.workflow_name]
                      .filter(Boolean)
                      .join("/") || "—"}
                  </option>
                ))}
              </Select>
              <form className="flex items-center gap-2" onSubmit={submit}>
                <Input
                  aria-label="搜索聊天记录"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="搜索历史聊天内容…"
                  className="w-64"
                />
                <Button type="submit" variant="outline">
                  <Search size={14} /> 搜索
                </Button>
                {search !== "" && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={clearSearch}
                  >
                    <X size={13} /> 清除搜索
                  </Button>
                )}
              </form>
              {selected && (
                <span className="text-[11px] text-muted-foreground">
                  最后活跃 {new Date(selected.last_active_at).toLocaleString()}
                </span>
              )}
            </div>
            {!showResults && (search !== "" || anchor.kind === "entry") && (
              <div className="flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
                {anchor.kind === "entry" && (
                  <span>
                    已定位到 {formatStamp(anchor.entry.created_at)} 的记录
                  </span>
                )}
                {search !== "" && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setShowResults(true)}
                  >
                    <ArrowLeft size={13} /> 返回搜索结果
                  </Button>
                )}
                {anchor.kind === "entry" && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setAnchor({ kind: "latest" })}
                  >
                    回到最新
                  </Button>
                )}
              </div>
            )}
            {showResults ? (
              resultsBusy ? (
                <p className="text-xs text-muted-foreground">正在搜索…</p>
              ) : results === undefined ? null : results.items.length === 0 ? (
                <p className="text-xs text-muted-foreground">
                  没有匹配“{search}”的记录。
                </p>
              ) : (
                <div className="flex flex-col gap-1.5">
                  <p className="m-0 text-[11px] text-muted-foreground">
                    “{search}”的搜索结果，点击一条记录定位到它的上下文。
                  </p>
                  <div
                    ref={scroller}
                    onScroll={onScroll}
                    className="relative max-h-[70vh] overflow-y-auto rounded-md border border-border"
                  >
                    <div className="divide-y divide-border">
                      {results.items.map((item) => (
                        <ResultRow
                          key={item.name}
                          item={item}
                          search={search}
                          onSelect={() => locate(item)}
                        />
                      ))}
                    </div>
                    {results.older !== undefined && (
                      <EdgeButton
                        busy={paging === "results"}
                        label="更多结果"
                        onClick={loadMoreResults}
                      />
                    )}
                  </div>
                </div>
              )
            ) : busy ? (
              <p className="text-xs text-muted-foreground">正在读取历史记录…</p>
            ) : timeline === undefined ? null : timeline.items.length === 0 ? (
              <p className="text-xs text-muted-foreground">还没有聊天记录。</p>
            ) : (
              <div
                ref={scroller}
                onScroll={onScroll}
                className="relative max-h-[70vh] overflow-y-auto rounded-md border border-border"
              >
                {timeline.hasNewer && (
                  <EdgeButton
                    busy={paging === "newer"}
                    label="更新的记录"
                    onClick={loadNewer}
                  />
                )}
                <div className="divide-y divide-border">
                  {timeline.items.map((item, index) => {
                    const date = new Date(item.created_at).toLocaleDateString();
                    const previous = timeline.items[index - 1];
                    const divider =
                      previous === undefined ||
                      new Date(previous.created_at).toLocaleDateString() !==
                        date;
                    return (
                      <div key={item.name} data-history={item.name}>
                        {divider && (
                          <div className="bg-secondary/60 px-3 py-1 text-[10px] font-medium text-muted-foreground">
                            {date}
                          </div>
                        )}
                        <Message
                          peer={peer}
                          workspace={workspace}
                          item={item}
                          focused={item.name === focused}
                        />
                      </div>
                    );
                  })}
                </div>
                {timeline.older !== undefined && (
                  <EdgeButton
                    busy={paging === "older"}
                    label="更早的记录"
                    onClick={loadOlder}
                  />
                )}
              </div>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}

function formatStamp(value: string): string {
  const date = new Date(value);
  return `${date.toLocaleDateString()} ${date.toLocaleTimeString()}`;
}

/** Literal, case-sensitive highlighting, matching the server's text search. */
function Highlight({ text, search }: { text: string; search: string }) {
  if (search === "") return <>{text}</>;
  const parts: ReactNode[] = [];
  let from = 0;
  for (
    let at = text.indexOf(search);
    at !== -1;
    at = text.indexOf(search, from)
  ) {
    if (at > from) parts.push(text.slice(from, at));
    parts.push(
      <mark key={at} className="rounded-sm bg-[#f3e2d6] px-0.5 text-inherit">
        {search}
      </mark>,
    );
    from = at + search.length;
  }
  parts.push(text.slice(from));
  return <>{parts}</>;
}

function ResultRow({
  item,
  search,
  onSelect,
}: {
  item: HistoryItem;
  search: string;
  onSelect: () => void;
}) {
  const role = ROLES[item.type] ?? {
    label: item.type,
    className: "bg-secondary text-muted-foreground",
  };
  return (
    <button
      type="button"
      onClick={onSelect}
      className="grid w-full cursor-pointer grid-cols-[auto_2.75rem_7rem_minmax(0,1fr)] items-baseline gap-3 px-3 py-1.5 text-left hover:bg-secondary/60"
    >
      <time className="font-mono text-[10px] whitespace-nowrap text-muted-foreground">
        {formatStamp(item.created_at)}
      </time>
      <span
        className={cn(
          "rounded px-1.5 py-0.5 text-center text-[10px] font-medium",
          role.className,
        )}
      >
        {role.label}
      </span>
      <span className="truncate text-[11px] text-muted-foreground">
        {item.actor_name || item.type}
      </span>
      <span className="text-xs leading-relaxed break-words whitespace-pre-wrap">
        <Highlight text={item.text} search={search} />
      </span>
    </button>
  );
}

function EdgeButton({
  busy,
  label,
  onClick,
}: {
  busy: boolean;
  label: string;
  onClick: () => void;
}) {
  return (
    <div className="flex justify-center px-3 py-1.5">
      <Button variant="ghost" size="sm" disabled={busy} onClick={onClick}>
        {busy ? "正在读取…" : label}
      </Button>
    </div>
  );
}

const ROLES: Record<string, { label: string; className: string }> = {
  gear: { label: "设备", className: "bg-[#f3eae1] text-primary" },
  agent: { label: "助手", className: "bg-[#e7efeb] text-[#3f7d70]" },
  user: { label: "用户", className: "bg-[#f3eae1] text-primary" },
  system: { label: "系统", className: "bg-secondary text-muted-foreground" },
};

function Message({
  peer,
  workspace,
  item,
  focused = false,
}: {
  peer: WatchedPeer;
  workspace: string;
  item: HistoryItem;
  focused?: boolean;
}) {
  // A transcript reads like a log: one line per turn, with the speaking side
  // named explicitly rather than implied by which edge it sits on.
  const role = ROLES[item.type] ?? {
    label: item.type,
    className: "bg-secondary text-muted-foreground",
  };
  return (
    <article
      aria-current={focused ? "true" : undefined}
      className={cn(
        "grid grid-cols-[auto_2.75rem_7rem_minmax(0,1fr)_auto] items-baseline gap-3 px-3 py-1.5 hover:bg-secondary/60",
        focused && "bg-[#f3e2d6] hover:bg-[#f3e2d6]",
      )}
    >
      <time className="font-mono text-[10px] whitespace-nowrap text-muted-foreground">
        {new Date(item.created_at).toLocaleTimeString()}
      </time>
      <span
        className={cn(
          "rounded px-1.5 py-0.5 text-center text-[10px] font-medium",
          role.className,
        )}
        title={`type=${item.type}`}
      >
        {role.label}
      </span>
      <span className="truncate text-[11px] text-muted-foreground">
        {item.actor_name || item.type}
      </span>
      <p className="m-0 text-xs leading-relaxed break-words whitespace-pre-wrap">
        {item.text || (
          <span className="text-muted-foreground">此记录没有文本</span>
        )}
      </p>
      {item.replay_available ? (
        <HistoryAudio peer={peer} workspace={workspace} history={item.name} />
      ) : (
        <span />
      )}
    </article>
  );
}

function HistoryAudio({
  peer,
  workspace,
  history,
}: {
  peer: WatchedPeer;
  workspace: string;
  history: string;
}) {
  // A transcript line has room for one small control, not a full audio player.
  const [audio, setAudio] = useState<HTMLAudioElement>();
  const [playing, setPlaying] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  // A download started here must not outlive the row: leaving the workspace,
  // page or device while it is pending would otherwise start playback on a
  // component that no longer exists and leak its object URL.
  const pending = useRef<AbortController>(null);

  useEffect(() => {
    return () => {
      pending.current?.abort();
      pending.current = null;
    };
  }, []);

  useEffect(() => {
    if (!audio) return;
    const stop = () => setPlaying(false);
    audio.addEventListener("ended", stop);
    audio.addEventListener("pause", stop);
    return () => {
      audio.removeEventListener("ended", stop);
      audio.removeEventListener("pause", stop);
      audio.pause();
      URL.revokeObjectURL(audio.src);
    };
  }, [audio]);

  const toggle = () => {
    if (audio) {
      if (playing) {
        audio.pause();
      } else {
        setPlaying(true);
        void audio.play().catch((failure: unknown) => {
          setPlaying(false);
          setError(nodeErrorMessage(failure));
        });
      }
      return;
    }
    setBusy(true);
    setError("");
    const controller = new AbortController();
    pending.current = controller;
    loadHistoryAudio(
      peer.endpoint,
      peer.publicKey,
      workspace,
      history,
      controller.signal,
    )
      .then((blob) => {
        const url = URL.createObjectURL(blob);
        if (controller.signal.aborted) {
          URL.revokeObjectURL(url);
          return;
        }
        const element = new Audio(url);
        setAudio(element);
        setPlaying(true);
        return element.play();
      })
      .catch((failure: unknown) => {
        if (controller.signal.aborted) return;
        setPlaying(false);
        setError(nodeErrorMessage(failure));
      })
      .finally(() => {
        if (pending.current === controller) pending.current = null;
        if (!controller.signal.aborted) setBusy(false);
      });
  };

  return (
    <span className="flex items-center gap-1.5">
      {error && (
        <span className="max-w-40 truncate text-[10px] text-destructive">
          {error}
        </span>
      )}
      <Button
        variant="ghost"
        size="sm"
        className="h-6 gap-1 px-1.5 text-[10px]"
        disabled={busy}
        aria-label={playing ? "暂停录音" : "播放录音"}
        onClick={toggle}
      >
        {playing ? <Pause size={11} /> : <Play size={11} />}
        {busy ? "…" : "录音"}
      </Button>
    </span>
  );
}
