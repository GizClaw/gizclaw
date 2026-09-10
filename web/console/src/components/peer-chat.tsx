import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { CalendarDays, Pause, Play, Search, X } from "lucide-react";
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

/** Local midnight after `day` (YYYY-MM-DD): the exclusive end of that day. */
function endOfDay(day: string): number {
  const [year, month, date] = day.split("-").map(Number);
  return new Date(year, month - 1, date + 1).getTime();
}

function today(): string {
  const now = new Date();
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}

// Scrolling within this many pixels of an edge loads the next page there.
const EDGE_PX = 48;

/** Persisted Workspace history: what the device and the agent actually said. */
export function PeerChat({ peer }: { peer: WatchedPeer }) {
  const [workspaces, setWorkspaces] = useState<Workspaces>([]);
  const [workspace, setWorkspace] = useState("");
  const [timeline, setTimeline] = useState<Timeline>();
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [day, setDay] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [paging, setPaging] = useState<"older" | "newer">();
  const scroller = useRef<HTMLDivElement>(null);
  // One edge request at a time; a new query aborts it so a stale page can
  // never be stitched onto a different timeline.
  const pending = useRef<AbortController>(null);
  // Prepending newer entries must not move what the reader is looking at.
  const anchor = useRef<{ height: number; top: number }>(null);

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

  useEffect(() => {
    if (workspace === "") return;
    const controller = new AbortController();
    pending.current?.abort();
    pending.current = null;
    setPaging(undefined);
    setBusy(true);
    setError("");
    const load = async (): Promise<Timeline> => {
      if (day === "") {
        const page = await loadHistory(
          peer.endpoint,
          peer.publicKey,
          workspace,
          { query: search, order: "desc" },
          controller.signal,
        );
        return {
          items: page.items,
          older: page.has_next ? page.next_cursor : undefined,
          hasNewer: false,
        };
      }
      // Jumping to a day shows that day's newest entries first, then older
      // ones below; one probe tells whether anything exists after that day.
      const end = endOfDay(day);
      const [page, after] = await Promise.all([
        loadHistory(
          peer.endpoint,
          peer.publicKey,
          workspace,
          { query: search, order: "desc", endTimeMs: end },
          controller.signal,
        ),
        loadHistory(
          peer.endpoint,
          peer.publicKey,
          workspace,
          { query: search, order: "asc", startTimeMs: end, limit: 1 },
          controller.signal,
        ),
      ]);
      return {
        items: page.items,
        older: page.has_next ? page.next_cursor : undefined,
        hasNewer: after.items.length > 0,
      };
    };
    load()
      .then((result) => {
        if (controller.signal.aborted) return;
        setTimeline(result);
        if (scroller.current) scroller.current.scrollTop = 0;
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
  }, [peer.endpoint, peer.publicKey, workspace, search, day]);

  useLayoutEffect(() => {
    const element = scroller.current;
    const saved = anchor.current;
    if (!element || !saved) return;
    anchor.current = null;
    element.scrollTop = saved.top + (element.scrollHeight - saved.height);
  }, [timeline]);

  const loadOlder = () => {
    if (!timeline?.older || busy || pending.current) return;
    const controller = new AbortController();
    pending.current = controller;
    setPaging("older");
    loadHistory(
      peer.endpoint,
      peer.publicKey,
      workspace,
      { query: search, order: "desc", cursor: timeline.older },
      controller.signal,
    )
      .then((page) => {
        if (controller.signal.aborted) return;
        setTimeline(
          (current) =>
            current && {
              ...current,
              items: [...current.items, ...page.items],
              older: page.has_next ? page.next_cursor : undefined,
            },
        );
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

  const loadNewer = () => {
    if (!timeline?.hasNewer || busy || pending.current) return;
    const controller = new AbortController();
    pending.current = controller;
    setPaging("newer");
    const top = timeline.items[0]?.name;
    loadHistory(
      peer.endpoint,
      peer.publicKey,
      workspace,
      {
        query: search,
        order: "asc",
        // An empty jump has no entry to page from; start after the day instead.
        cursor: top,
        startTimeMs:
          top === undefined && day !== "" ? endOfDay(day) : undefined,
      },
      controller.signal,
    )
      .then((page) => {
        if (controller.signal.aborted) return;
        const element = scroller.current;
        if (element) {
          anchor.current = {
            height: element.scrollHeight,
            top: element.scrollTop,
          };
        }
        // The server returns this page oldest first; it sits above the
        // current newest entry, so its newest entry goes on top.
        const newer = [...page.items].reverse();
        setTimeline(
          (current) =>
            current && {
              ...current,
              items: [...newer, ...current.items],
              hasNewer: page.has_next,
            },
        );
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

  const onScroll = () => {
    const element = scroller.current;
    if (!element) return;
    if (element.scrollTop < EDGE_PX) loadNewer();
    if (
      element.scrollHeight - element.scrollTop - element.clientHeight <
      EDGE_PX
    )
      loadOlder();
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setSearch(query);
  };

  const selected = workspaces.find((item) => item.id === workspace);
  return (
    <Card>
      <CardHeader>
        <CardTitle>对话记录</CardTitle>
        <CardDescription>
          {
            "该设备拥有的 Workspace 及其持久化聊天记录，最新的在最上面。选择日期可跳到那一天，向上滚动读取更新的记录，向下滚动读取更早的记录。"
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
                  setQuery("");
                  setSearch("");
                  setDay("");
                  setTimeline(undefined);
                }}
              >
                {workspaces.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name} · {item.workflow_id}
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
              </form>
              <label className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                <CalendarDays size={14} />
                <Input
                  type="date"
                  aria-label="跳到日期"
                  value={day}
                  max={today()}
                  onChange={(event) => setDay(event.target.value)}
                  className="w-40"
                />
              </label>
              {day !== "" && (
                <Button variant="ghost" size="sm" onClick={() => setDay("")}>
                  <X size={13} /> 回到最新
                </Button>
              )}
              {selected && (
                <span className="text-[11px] text-muted-foreground">
                  最后活跃 {new Date(selected.last_active_at).toLocaleString()}
                </span>
              )}
            </div>
            {busy ? (
              <p className="text-xs text-muted-foreground">正在读取历史记录…</p>
            ) : timeline === undefined ? null : timeline.items.length === 0 &&
              !timeline.hasNewer ? (
              <p className="text-xs text-muted-foreground">
                {day === ""
                  ? "没有匹配的历史记录。"
                  : "这一天及之前没有匹配的历史记录。"}
              </p>
            ) : (
              <div
                ref={scroller}
                onScroll={onScroll}
                className="max-h-[70vh] overflow-y-auto rounded-md border border-border"
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
                      <div key={item.name}>
                        {divider && (
                          <div className="bg-secondary/60 px-3 py-1 text-[10px] font-medium text-muted-foreground">
                            {date}
                          </div>
                        )}
                        <Message
                          peer={peer}
                          workspace={workspace}
                          item={item}
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
}: {
  peer: WatchedPeer;
  workspace: string;
  item: HistoryItem;
}) {
  // A transcript reads like a log: one line per turn, with the speaking side
  // named explicitly rather than implied by which edge it sits on.
  const role = ROLES[item.type] ?? {
    label: item.type,
    className: "bg-secondary text-muted-foreground",
  };
  return (
    <article className="grid grid-cols-[auto_2.75rem_7rem_minmax(0,1fr)_auto] items-baseline gap-3 px-3 py-1.5 hover:bg-secondary/60">
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
