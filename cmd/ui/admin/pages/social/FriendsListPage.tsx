import { Plus, RefreshCw, UsersRound } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { createFriend, deleteFriend, getFriend, listFriends, type AdminFriendObject } from "@gizclaw/adminservice";
import { expectData, toMessage } from "../../components/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

import { ErrorBanner, NoticeBanner } from "../../components/banners";
import { DeleteConfirmButton } from "../../components/delete-confirm-button";
import { EmptyState } from "../../components/empty-state";
import { FormField } from "../../components/form-field";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { useCursorListPage } from "../../hooks/useCursorListPage";
import { formatDate, formatShortKey } from "../../lib/format";
import { friendDetailPath, friendRelationID, socialPeerLabel, socialWorkspaceName } from "./social-utils";

export function FriendsListPage(): JSX.Element {
  const navigate = useNavigate();
  const { error, hasNext, items, loading, nextPage, pageNumber, prevPage, refresh } = useCursorListPage<AdminFriendObject>(async (query) => {
    const result = await expectData(listFriends({ query }));
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });
  const [ownerPublicKey, setOwnerPublicKey] = useState("");
  const [peerPublicKey, setPeerPublicKey] = useState("");
  const [notice, setNotice] = useState<{ message: string; tone: "error" | "success" } | null>(null);
  const [busy, setBusy] = useState("");

  const create = async (): Promise<void> => {
    setBusy("create");
    setNotice(null);
    const owner = ownerPublicKey.trim();
    const peer = peerPublicKey.trim();
    try {
      const friend = await expectData(createFriend({ body: { owner_public_key: owner, peer_public_key: peer } }));
      setOwnerPublicKey("");
      setPeerPublicKey("");
      navigate(friendDetailPath(friend));
    } catch (err) {
      try {
        const friend = await expectData(getFriend({ path: { ownerPublicKey: owner, id: friendRelationID(owner, peer) } }));
        setOwnerPublicKey("");
        setPeerPublicKey("");
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

  const remove = async (friend: AdminFriendObject): Promise<void> => {
    setBusy(`delete:${friend.owner_public_key}:${friend.id}`);
    setNotice(null);
    try {
      await expectData(deleteFriend({ path: { ownerPublicKey: friend.owner_public_key, id: friend.id } }));
      await refresh();
      setNotice({ message: "Friend relation deleted.", tone: "success" });
    } catch (err) {
      setNotice({ message: toMessage(err), tone: "error" });
    } finally {
      setBusy("");
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" disabled={loading} onClick={() => void refresh()} variant="outline">
            <RefreshCw className="size-4" />
            Refresh
          </Button>
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
      {notice !== null ? <NoticeBanner message={notice.message} tone={notice.tone} /> : null}

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Create Friend</CardTitle>
          <CardDescription>Admin directly creates both owner-view rows and the backing direct workspace.</CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] lg:items-end"
            onSubmit={(event) => {
              event.preventDefault();
              void create();
            }}
          >
            <FormField label="Owner public key">
              <Input onChange={(event) => setOwnerPublicKey(event.target.value)} placeholder="Owner peer public key" value={ownerPublicKey} />
            </FormField>
            <FormField label="Friend public key">
              <Input onChange={(event) => setPeerPublicKey(event.target.value)} placeholder="Friend peer public key" value={peerPublicKey} />
            </FormField>
            <Button disabled={busy !== "" || ownerPublicKey.trim() === "" || peerPublicKey.trim() === ""} type="submit">
              <Plus className="size-4" />
              Create
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>Friend rows</CardTitle>
            <CardDescription>Cursor-paginated social friend resources.</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex justify-end gap-2">
            <Button
              className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm disabled:border-border disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100 disabled:shadow-none"
              disabled={loading || pageNumber === 1}
              onClick={prevPage}
              type="button"
              variant="outline"
            >
              Previous
            </Button>
            <Button
              className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm disabled:border-border disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100 disabled:shadow-none"
              disabled={loading || !hasNext}
              onClick={nextPage}
              type="button"
              variant="outline"
            >
              Next
            </Button>
          </div>

          {loading ? (
            <div className="space-y-3">
              {Array.from({ length: 6 }).map((_, index) => (
                <Skeleton className="h-14 w-full" key={index} />
              ))}
            </div>
          ) : items.length === 0 ? (
            <EmptyState description="Friend rows will appear here after they are created." title="No friends" />
          ) : (
            <div className="rounded-md border">
              <Table className="table-fixed">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[24%]">Owner peer</TableHead>
                    <TableHead className="w-[24%]">Friend peer</TableHead>
                    <TableHead className="w-28">Relation</TableHead>
                    <TableHead>Workspace</TableHead>
                    <TableHead className="w-32 text-right">Updated</TableHead>
                    <TableHead className="w-44 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((friend) => (
                    <TableRow className="hover:bg-muted/40" key={`${friend.owner_public_key}:${friend.id}`}>
                      <TableCell className="min-w-0">
                        <div className="truncate font-medium" title={socialPeerLabel(friend.owner_public_key)}>
                          {socialPeerLabel(friend.owner_public_key)}
                        </div>
                        <div className="truncate font-mono text-xs text-muted-foreground" title={friend.owner_public_key}>
                          {friend.owner_public_key}
                        </div>
                      </TableCell>
                      <TableCell className="min-w-0">
                        <div className="truncate font-medium" title={socialPeerLabel(friend.peer_public_key)}>
                          {socialPeerLabel(friend.peer_public_key)}
                        </div>
                        <div className="truncate font-mono text-xs text-muted-foreground" title={friend.peer_public_key}>
                          {friend.peer_public_key}
                        </div>
                      </TableCell>
                      <TableCell className="truncate font-mono text-xs" title={friend.id}>
                        {formatShortKey(friend.id)}
                      </TableCell>
                      <TableCell className="truncate font-mono text-xs" title={socialWorkspaceName(friend.workspace_name)}>
                        {socialWorkspaceName(friend.workspace_name)}
                      </TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground">{formatDate(friend.updated_at)}</TableCell>
                      <TableCell className="text-right">
                        <div className="flex flex-wrap justify-end gap-2">
                          <Button asChild className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" variant="outline">
                            <Link to={friendDetailPath(friend)}>
                              <UsersRound className="size-4" />
                              Open
                            </Link>
                          </Button>
                          <DeleteConfirmButton
                            description={`Delete relation ${formatShortKey(friend.id)} and its backing direct workspace.`}
                            disabled={busy !== ""}
                            onConfirm={() => void remove(friend)}
                            size="sm"
                            title="Delete friend relation?"
                          >
                            Delete
                          </DeleteConfirmButton>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
