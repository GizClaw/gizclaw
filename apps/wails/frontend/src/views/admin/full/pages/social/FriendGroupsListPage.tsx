import { Check, Copy, Plus, RefreshCw } from "lucide-react";
import { DashboardPager } from "@/dashboard";
import { DashboardTable } from "@/dashboard";
import type { KeyboardEvent, MouseEvent } from "react";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

import { createFriendGroup, getFriendGroup, listFriendGroups, type FriendGroupObject } from "@gizclaw/gizclaw/admin";
import { expectData, toMessage } from "@/dashboard";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";

import { ErrorBanner, NoticeBanner } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { FormField } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { useDashboardCursorPage as useCursorListPage } from "@/dashboard";
import { formatDate } from "../../lib/format";
import { friendGroupDetailPath, socialWorkspaceName } from "./social-utils";

export function FriendGroupsListPage(): JSX.Element {
  const navigate = useNavigate();
  const { error, hasNext, items, loading, nextPage, pageNumber, prevPage, refresh } = useCursorListPage<FriendGroupObject>(async (query) => {
    const result = await expectData(listFriendGroups({ query }));
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [notice, setNotice] = useState<{ message: string; tone: "error" | "success" } | null>(null);
  const [busy, setBusy] = useState("");
  const [copiedID, setCopiedID] = useState("");

  const openGroup = (group: FriendGroupObject): void => {
    navigate(friendGroupDetailPath(group));
  };

  const handleRowKeyDown = (event: KeyboardEvent<HTMLTableRowElement>, group: FriendGroupObject): void => {
    if (isInteractiveTarget(event.target)) {
      return;
    }
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    openGroup(group);
  };

  const copyGroupID = async (event: MouseEvent<HTMLButtonElement>, id: string): Promise<void> => {
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
    const groupID = name.trim();
    try {
      const group = await expectData(createFriendGroup({ body: { name: groupID, description: description.trim() || undefined } }));
      setName("");
      setDescription("");
      setCreateDialogOpen(false);
      navigate(friendGroupDetailPath(group));
    } catch (err) {
      try {
        const group = await expectData(getFriendGroup({ path: { id: groupID } }));
        setName("");
        setDescription("");
        setCreateDialogOpen(false);
        navigate(friendGroupDetailPath(group));
        return;
      } catch {
        // Keep the original create error when the group was not created.
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
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" onClick={() => setCreateDialogOpen(true)} type="button" variant="outline">
              <Plus className="size-4" />
              New Friend Group
            </Button>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" disabled={loading} onClick={() => void refresh()} variant="outline">
              <RefreshCw className="size-4" />
              Refresh
            </Button>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { label: "Friend Groups" }]}
      />

      <PageSummaryCard
        description="Global group resources with backing chatroom workspaces. Admin-created groups start without an implicit owner member."
        eyebrow="Social"
        meta={
          <>
            <Badge variant="outline">Page {pageNumber}</Badge>
            <Badge variant="secondary">{items.length} loaded</Badge>
            {hasNext ? <Badge variant="outline">More Available</Badge> : null}
          </>
        }
        title="Friend Groups"
      />

      {error !== "" ? <ErrorBanner message={error} /> : null}
      {notice !== null ? <NoticeBanner message={notice.message} tone={notice.tone} /> : null}

      <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New Friend Group</DialogTitle>
            <DialogDescription>Create group metadata and a backing chatroom workspace. Members are added separately.</DialogDescription>
          </DialogHeader>
          <form
            className="flex flex-col gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              void create();
            }}
          >
            <FormField label="Name">
              <Input onChange={(event) => setName(event.target.value)} placeholder="story-club" value={name} />
            </FormField>
            <FormField label="Description">
              <Textarea className="min-h-10" onChange={(event) => setDescription(event.target.value)} placeholder="Group description" value={description} />
            </FormField>
            <DialogFooter>
              <Button disabled={busy !== ""} onClick={() => setCreateDialogOpen(false)} type="button" variant="outline">
                Cancel
              </Button>
              <Button disabled={busy !== "" || name.trim() === ""} onClick={() => void create()} type="button">
                Create
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>Groups</CardTitle>
            <CardDescription>Cursor-paginated friend group resources.</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex justify-end">
            <DashboardPager canNext={hasNext} canPrevious={pageNumber > 1} loading={loading} onNext={nextPage} onPrevious={prevPage} onRefresh={() => void refresh()} pageIndex={pageNumber} />
          </div>

          {loading ? (
            <div className="space-y-3">
              {Array.from({ length: 6 }).map((_, index) => (
                <Skeleton className="h-14 w-full" key={index} />
              ))}
            </div>
          ) : items.length === 0 ? (
            <EmptyState description="Friend groups will appear here after they are created." title="No friend groups" />
          ) : (
            <DashboardTable className="table-fixed">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[22%]">Group ID</TableHead>
                    <TableHead className="w-[28%]">Description</TableHead>
                    <TableHead className="w-[34%]">Workspace</TableHead>
                    <TableHead className="w-32 text-right">Updated</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((group) => {
                    const id = group.id ?? "";
                    return (
                      <TableRow
                        className="cursor-pointer hover:bg-muted/40"
                        key={id}
                        onClick={() => openGroup(group)}
                        onKeyDown={(event) => handleRowKeyDown(event, group)}
                        role="link"
                        tabIndex={0}
                      >
                        <TableCell className="min-w-0">
                          <div className="flex min-w-0 items-center gap-1.5">
                            <div className="min-w-0 flex-1">
                              <div className="truncate font-medium" title={group.name?.trim() || id}>
                                {group.name?.trim() || id}
                              </div>
                              <button
                                className="block w-full truncate rounded-sm text-left font-mono text-xs text-muted-foreground underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                                disabled={id === ""}
                                onClick={(event) => {
                                  event.stopPropagation();
                                  openGroup(group);
                                }}
                                title={id}
                                type="button"
                              >
                                {id}
                              </button>
                            </div>
                            <button
                              aria-label={`Copy group id ${id}`}
                              className="shrink-0 rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                              disabled={id === ""}
                              onClick={(event) => void copyGroupID(event, id)}
                              title="Copy group id"
                              type="button"
                            >
                              {copiedID === id ? <Check className="size-3 shrink-0 text-emerald-600" /> : <Copy className="size-3 shrink-0" />}
                            </button>
                          </div>
                        </TableCell>
                        <TableCell className="truncate text-sm leading-6 text-muted-foreground" title={group.description?.trim() || "—"}>
                          {group.description?.trim() || "—"}
                        </TableCell>
                        <TableCell className="truncate font-mono text-xs" title={socialWorkspaceName(group.workspace_name)}>
                          {socialWorkspaceName(group.workspace_name)}
                        </TableCell>
                        <TableCell className="text-right text-sm text-muted-foreground">{formatDate(group.updated_at)}</TableCell>
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
  return target instanceof Element && target.closest("a,button,input,select,textarea") !== null;
}
