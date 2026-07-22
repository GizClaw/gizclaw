import { Check, Copy, Plus, RefreshCw } from "lucide-react";
import { DashboardActionButton } from "@/dashboard";
import { DashboardPager } from "@/dashboard";
import { DashboardTable } from "@/dashboard";
import type { KeyboardEvent, MouseEvent } from "react";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

import {
  createFriend,
  getFriend,
  listFriends,
  type AdminFriendObject,
} from "@gizclaw/gizclaw/admin";
import { expectData, toMessage } from "@/dashboard";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

import { ErrorBanner, NoticeBanner } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { FormField } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { useDashboardCursorPage as useCursorListPage } from "@/dashboard";
import { formatDate, formatShortKey } from "../../lib/format";
import {
  friendDetailPath,
  friendRelationID,
  socialWorkspaceName,
} from "./social-utils";

export function FriendsListPage(): JSX.Element {
  const navigate = useNavigate();
  const {
    error,
    hasNext,
    items,
    loading,
    nextPage,
    pageNumber,
    prevPage,
    refresh,
  } = useCursorListPage<AdminFriendObject>(async (query) => {
    const result = await expectData(listFriends({ query }));
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });
  const [ownerPublicKey, setOwnerPublicKey] = useState("");
  const [peerPublicKey, setPeerPublicKey] = useState("");
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [notice, setNotice] = useState<{
    message: string;
    tone: "error" | "success";
  } | null>(null);
  const [busy, setBusy] = useState("");
  const [copiedID, setCopiedID] = useState("");

  const openFriend = (friend: AdminFriendObject): void => {
    navigate(friendDetailPath(friend));
  };

  const handleRowKeyDown = (
    event: KeyboardEvent<HTMLTableRowElement>,
    friend: AdminFriendObject,
  ): void => {
    if (isInteractiveTarget(event.target)) {
      return;
    }
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    openFriend(friend);
  };

  const copyFriendListID = async (
    event: MouseEvent<HTMLButtonElement>,
    id: string,
  ): Promise<void> => {
    event.stopPropagation();
    await navigator.clipboard.writeText(id);
    setCopiedID(id);
    window.setTimeout(() => {
      setCopiedID((current) => (current === id ? "" : current));
    }, 1500);
  };

  const create = async (): Promise<void> => {
    setBusy("create");
    setNotice(null);
    const owner = ownerPublicKey.trim();
    const peer = peerPublicKey.trim();
    try {
      const friend = await expectData(
        createFriend({
          body: { owner_public_key: owner, peer_public_key: peer },
        }),
      );
      setOwnerPublicKey("");
      setPeerPublicKey("");
      setCreateDialogOpen(false);
      navigate(friendDetailPath(friend));
    } catch (err) {
      try {
        const friend = await expectData(
          getFriend({
            path: { ownerPublicKey: owner, id: friendRelationID(owner, peer) },
          }),
        );
        setOwnerPublicKey("");
        setPeerPublicKey("");
        setCreateDialogOpen(false);
        navigate(friendDetailPath(friend));
        return;
      } catch {
        // Keep the original create error when the relation was not created.
      }
      setNotice({ message: toMessage(err), tone: "error" });
    } finally {
      setBusy("");
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <DashboardActionButton
              onClick={() => setCreateDialogOpen(true)}
              type="button"
            >
              <Plus className="size-4" />
              New Friend
            </DashboardActionButton>
            <DashboardActionButton
              disabled={loading}
              onClick={() => void refresh()}
            >
              <RefreshCw className="size-4" />
              Refresh
            </DashboardActionButton>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { label: "Friends" }]}
      />

      <PageSummaryCard
        description="Global owner-view friend rows. Duplicated rows are expected because each peer owns its own row for the same relation."
        eyebrow="Social"
        meta={
          <>
            <Badge variant="outline">Page {pageNumber}</Badge>
            <Badge variant="secondary">{items.length} loaded</Badge>
            {hasNext ? <Badge variant="outline">More Available</Badge> : null}
          </>
        }
        title="Friends"
      />

      {error !== "" ? <ErrorBanner message={error} /> : null}
      {notice !== null ? (
        <NoticeBanner message={notice.message} tone={notice.tone} />
      ) : null}

      <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New Friend</DialogTitle>
            <DialogDescription>
              Admin directly creates both owner-view rows and the backing direct
              workspace.
            </DialogDescription>
          </DialogHeader>
          <form
            className="flex flex-col gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              void create();
            }}
          >
            <FormField label="Owner public key">
              <Input
                onChange={(event) => setOwnerPublicKey(event.target.value)}
                placeholder="Owner peer public key"
                value={ownerPublicKey}
              />
            </FormField>
            <FormField label="Friend public key">
              <Input
                onChange={(event) => setPeerPublicKey(event.target.value)}
                placeholder="Friend peer public key"
                value={peerPublicKey}
              />
            </FormField>
            <DialogFooter>
              <Button
                disabled={busy !== ""}
                onClick={() => setCreateDialogOpen(false)}
                type="button"
                variant="outline"
              >
                Cancel
              </Button>
              <Button
                disabled={
                  busy !== "" ||
                  ownerPublicKey.trim() === "" ||
                  peerPublicKey.trim() === ""
                }
                onClick={() => void create()}
                type="button"
              >
                Create
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>Friend rows</CardTitle>
            <CardDescription>
              Cursor-paginated social friend resources.
            </CardDescription>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
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
              {Array.from({ length: 6 }).map((_, index) => (
                <Skeleton className="h-14 w-full" key={index} />
              ))}
            </div>
          ) : items.length === 0 ? (
            <EmptyState
              description="Friend rows will appear here after they are created."
              title="No friends"
            />
          ) : (
            <DashboardTable className="table-fixed">
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[48%]">ID</TableHead>
                  <TableHead className="w-[32%]">Workspace</TableHead>
                  <TableHead className="w-32 text-right">Updated</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((friend) => {
                  const listID = friendListID(friend);
                  return (
                    <TableRow
                      className="cursor-pointer hover:bg-muted/40"
                      key={`${friend.owner_public_key}:${friend.id}`}
                      onClick={() => openFriend(friend)}
                      onKeyDown={(event) => handleRowKeyDown(event, friend)}
                      role="link"
                      tabIndex={0}
                    >
                      <TableCell className="min-w-0">
                        <div
                          className="flex min-w-0 items-center gap-1.5"
                          title={listID}
                        >
                          <div className="min-w-0 flex-1">
                            <button
                              className="block w-full truncate rounded-sm text-left font-mono text-xs font-medium leading-5 underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                              onClick={(event) => {
                                event.stopPropagation();
                                openFriend(friend);
                              }}
                              title={listID}
                              type="button"
                            >
                              {friendListDisplayID(friend)}
                            </button>
                            <div
                              className="mt-1 truncate font-mono text-xs leading-5 text-muted-foreground"
                              title={friendPeerRelation(friend)}
                            >
                              {friendPeerRelation(friend)}
                            </div>
                          </div>
                          <button
                            aria-label={`Copy friend id ${listID}`}
                            className="shrink-0 rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            onClick={(event) =>
                              void copyFriendListID(event, listID)
                            }
                            title="Copy friend id"
                            type="button"
                          >
                            {copiedID === listID ? (
                              <Check className="size-3 shrink-0 text-emerald-600" />
                            ) : (
                              <Copy className="size-3 shrink-0" />
                            )}
                          </button>
                        </div>
                      </TableCell>
                      <TableCell
                        className="truncate font-mono text-xs"
                        title={socialWorkspaceName(friend.workspace_name)}
                      >
                        {socialWorkspaceName(friend.workspace_name)}
                      </TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground">
                        {formatDate(friend.updated_at)}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </DashboardTable>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function isInteractiveTarget(target: EventTarget): boolean {
  return (
    target instanceof Element &&
    target.closest("a,button,input,select,textarea") !== null
  );
}

function friendListID(friend: AdminFriendObject): string {
  return `${friend.owner_public_key}/${friend.id}`;
}

function friendPeerRelation(friend: AdminFriendObject): string {
  return `${friend.owner_public_key} <-> ${friend.peer_public_key}`;
}

function friendListDisplayID(friend: AdminFriendObject): string {
  return `${formatShortKey(friend.owner_public_key)}/${formatShortKey(friend.id)}`;
}
