import { Download, Play, RefreshCw } from "lucide-react";
import {
  DashboardActionButton,
  DashboardPager,
  DashboardTable,
} from "@/dashboard";
import { useEffect, useState } from "react";

import {
  downloadWorkspaceHistoryAudio,
  listWorkspaceHistory,
  type PeerRunHistoryEntry,
} from "@gizclaw/gizclaw/admin";
import { expectData, toMessage } from "@/dashboard";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

import { ErrorBanner } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { useDashboardCursorPage as useCursorListPage } from "@/dashboard";
import { formatDate } from "../../lib/format";

type WorkspaceHistoryPanelProps = {
  workspaceName: string | undefined;
};

export function WorkspaceHistoryPanel({
  workspaceName,
}: WorkspaceHistoryPanelProps): JSX.Element {
  const normalizedWorkspaceName = workspaceName?.trim() ?? "";
  const {
    error,
    hasNext,
    items,
    loading,
    nextPage,
    pageNumber,
    prevPage,
    refresh,
  } = useCursorListPage<PeerRunHistoryEntry>(async (query) => {
    if (normalizedWorkspaceName === "") {
      return { hasNext: false, items: [], nextCursor: null };
    }
    const result = await expectData(
      listWorkspaceHistory({
        path: { name: normalizedWorkspaceName },
        query: { ...query, order: "asc" },
      }),
    );
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });
  const [audioURL, setAudioURL] = useState<string>("");
  const [audioError, setAudioError] = useState("");
  const [playingHistoryID, setPlayingHistoryID] = useState("");

  useEffect(() => {
    return () => {
      if (audioURL !== "") {
        URL.revokeObjectURL(audioURL);
      }
    };
  }, [audioURL]);

  const playAudio = async (historyID: string): Promise<void> => {
    if (normalizedWorkspaceName === "") {
      return;
    }
    setPlayingHistoryID(historyID);
    setAudioError("");
    try {
      const audio = await expectData(
        downloadWorkspaceHistoryAudio({
          path: { name: normalizedWorkspaceName, historyId: historyID },
        }),
      );
      setAudioURL((current) => {
        if (current !== "") {
          URL.revokeObjectURL(current);
        }
        return URL.createObjectURL(audio);
      });
    } catch (err) {
      setAudioError(toMessage(err));
    } finally {
      setPlayingHistoryID("");
    }
  };

  if (normalizedWorkspaceName === "") {
    return (
      <EmptyState
        description="This social resource does not have a backing workspace."
        title="No history workspace"
      />
    );
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
        <div className="space-y-1">
          <CardTitle>Workspace History</CardTitle>
          <CardDescription>
            History is read through Admin HTTP APIs and audio is downloaded as
            Ogg Opus.
          </CardDescription>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="outline">Page {pageNumber}</Badge>
          <DashboardActionButton
            disabled={loading}
            onClick={() => void refresh()}
          >
            <RefreshCw className="size-4" />
            Reload
          </DashboardActionButton>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {error !== "" ? <ErrorBanner message={error} /> : null}
        {audioError !== "" ? <ErrorBanner message={audioError} /> : null}
        {audioURL !== "" ? (
          <audio autoPlay className="w-full" controls src={audioURL} />
        ) : null}

        <div className="flex justify-end">
          <DashboardPager
            canNext={hasNext}
            canPrevious={pageNumber > 1}
            loading={loading}
            onNext={nextPage}
            onPrevious={prevPage}
            onRefresh={() => void refresh()}
            pageIndex={pageNumber}
          />
        </div>

        {loading ? (
          <div className="space-y-3">
            {Array.from({ length: 4 }).map((_, index) => (
              <Skeleton className="h-14 w-full" key={index} />
            ))}
          </div>
        ) : items.length === 0 ? (
          <EmptyState
            description="History entries will appear after the backing workspace records conversation history."
            title="No history entries"
          />
        ) : (
          <DashboardTable>
            <TableHeader>
              <TableRow>
                <TableHead className="w-28">Type</TableHead>
                <TableHead className="w-48">Name</TableHead>
                <TableHead>Text</TableHead>
                <TableHead className="w-44">Created</TableHead>
                <TableHead className="w-32 text-right">Audio</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((entry) => (
                <TableRow key={entry.id}>
                  <TableCell>
                    <Badge
                      variant={entry.type === "gear" ? "secondary" : "outline"}
                    >
                      {entry.type}
                    </Badge>
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {entry.name}
                    {entry.gear_id ? (
                      <div className="mt-1 break-all text-muted-foreground">
                        {entry.gear_id}
                      </div>
                    ) : null}
                  </TableCell>
                  <TableCell className="max-w-[32rem] text-sm leading-6">
                    {entry.text}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(entry.created_at)}
                  </TableCell>
                  <TableCell className="text-right">
                    <DashboardActionButton
                      disabled={
                        !entry.replay_available || playingHistoryID === entry.id
                      }
                      onClick={() => void playAudio(entry.id)}
                      type="button"
                    >
                      {entry.replay_available ? (
                        <Play className="size-4" />
                      ) : (
                        <Download className="size-4" />
                      )}
                      Play
                    </DashboardActionButton>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </DashboardTable>
        )}
      </CardContent>
    </Card>
  );
}
