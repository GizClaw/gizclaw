import { useEffect, useRef, useState, type FormEvent } from "react";
import { Pause, Play, Search } from "lucide-react";
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
type HistoryPage = Awaited<ReturnType<typeof loadHistory>>;

/** Persisted Workspace history: what the device and the agent actually said. */
export function PeerChat({ peer }: { peer: WatchedPeer }) {
  const [workspaces, setWorkspaces] = useState<Workspaces>([]);
  const [workspace, setWorkspace] = useState("");
  const [page, setPage] = useState<HistoryPage>();
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [cursor, setCursor] = useState<string>();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    setError("");
    setLoading(true);
    setWorkspaces([]);
    setWorkspace("");
    setPage(undefined);
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
    setBusy(true);
    setError("");
    loadHistory(
      peer.endpoint,
      peer.publicKey,
      workspace,
      search,
      cursor,
      controller.signal,
    )
      .then((result) => {
        if (!controller.signal.aborted) setPage(result);
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setError(nodeErrorMessage(failure));
      })
      .finally(() => {
        if (!controller.signal.aborted) setBusy(false);
      });
    return () => controller.abort();
  }, [peer.endpoint, peer.publicKey, workspace, search, cursor]);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setSearch(query);
    setCursor(undefined);
  };

  const selected = workspaces.find((item) => item.id === workspace);
  return (
    <Card>
      <CardHeader>
        <CardTitle>对话记录</CardTitle>
        <CardDescription>
          该设备拥有的 Workspace 及其持久化聊天记录，按时间顺序展示，每页最多
          100 条。
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
                  setCursor(undefined);
                  setQuery("");
                  setSearch("");
                  setPage(undefined);
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
              {selected && (
                <span className="text-[11px] text-muted-foreground">
                  最后活跃 {new Date(selected.last_active_at).toLocaleString()}
                </span>
              )}
            </div>
            {busy ? (
              <p className="text-xs text-muted-foreground">正在读取历史记录…</p>
            ) : page === undefined ? null : page.items.length === 0 ? (
              <p className="text-xs text-muted-foreground">
                没有匹配的历史记录。
              </p>
            ) : (
              <div className="divide-y divide-border overflow-hidden rounded-md border border-border">
                {[...page.items].reverse().map((item) => (
                  <Message
                    key={item.name}
                    peer={peer}
                    workspace={workspace}
                    item={item}
                  />
                ))}
              </div>
            )}
            {page?.has_next && (
              <Button
                variant="outline"
                size="sm"
                className="self-start"
                onClick={() => setCursor(page.next_cursor)}
              >
                更早的记录
              </Button>
            )}
          </>
        )}
      </CardContent>
    </Card>
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
  item: HistoryPage["items"][number];
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
