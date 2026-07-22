import { Plus, RefreshCw, Trash2 } from "lucide-react";
import { DashboardPager } from "@/dashboard";
import { DashboardTable } from "@/dashboard";
import { useState } from "react";

import {
  createFriendGroupMember,
  deleteFriendGroupMember,
  listFriendGroupMembers,
  putFriendGroupMember,
  type FriendGroupMemberObject,
  type FriendGroupMemberRole,
} from "@gizclaw/gizclaw/admin";
import { expectData, toMessage } from "@/dashboard";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

import { ErrorBanner, NoticeBanner } from "@/dashboard";
import { DashboardDeleteButton as DeleteConfirmButton } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { FormField } from "@/dashboard";
import { useDashboardCursorPage as useCursorListPage } from "@/dashboard";
import { formatDate, formatShortKey } from "../../lib/format";

type FriendGroupMemberEditorProps = {
  groupID: string;
};

const memberRoles: FriendGroupMemberRole[] = ["owner", "admin", "member"];

export function FriendGroupMemberEditor({
  groupID,
}: FriendGroupMemberEditorProps): JSX.Element {
  const {
    error,
    hasNext,
    items,
    loading,
    nextPage,
    pageNumber,
    prevPage,
    refresh,
  } = useCursorListPage<FriendGroupMemberObject>(async (query) => {
    const result = await expectData(
      listFriendGroupMembers({ path: { id: groupID }, query }),
    );
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });
  const [peerPublicKey, setPeerPublicKey] = useState("");
  const [role, setRole] = useState<FriendGroupMemberRole>("member");
  const [notice, setNotice] = useState<{
    message: string;
    tone: "error" | "success";
  } | null>(null);
  const [busy, setBusy] = useState("");

  const addMember = async (): Promise<void> => {
    setBusy("add");
    setNotice(null);
    try {
      await expectData(
        createFriendGroupMember({
          body: { peer_public_key: peerPublicKey.trim(), role },
          path: { id: groupID },
        }),
      );
      setPeerPublicKey("");
      setRole("member");
      await refresh();
      setNotice({ message: "Member added.", tone: "success" });
    } catch (err) {
      setNotice({ message: toMessage(err), tone: "error" });
    } finally {
      setBusy("");
    }
  };

  const updateRole = async (
    member: FriendGroupMemberObject,
    nextRole: FriendGroupMemberRole,
  ): Promise<void> => {
    const publicKey = member.peer_public_key ?? "";
    if (publicKey === "") {
      return;
    }
    setBusy(`role:${publicKey}`);
    setNotice(null);
    try {
      await expectData(
        putFriendGroupMember({
          body: { role: nextRole },
          path: { id: groupID, publicKey },
        }),
      );
      await refresh();
      setNotice({ message: "Member role updated.", tone: "success" });
    } catch (err) {
      setNotice({ message: toMessage(err), tone: "error" });
    } finally {
      setBusy("");
    }
  };

  const removeMember = async (
    member: FriendGroupMemberObject,
  ): Promise<void> => {
    const publicKey = member.peer_public_key ?? "";
    if (publicKey === "") {
      return;
    }
    setBusy(`delete:${publicKey}`);
    setNotice(null);
    try {
      await expectData(
        deleteFriendGroupMember({ path: { id: groupID, publicKey } }),
      );
      await refresh();
      setNotice({ message: "Member removed.", tone: "success" });
    } catch (err) {
      setNotice({ message: toMessage(err), tone: "error" });
    } finally {
      setBusy("");
    }
  };

  return (
    <div className="space-y-4">
      {error !== "" ? <ErrorBanner message={error} /> : null}
      {notice !== null ? (
        <NoticeBanner message={notice.message} tone={notice.tone} />
      ) : null}

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Add Member</CardTitle>
          <CardDescription>
            Add/remove controls sync the backing workspace access. Role updates
            only change social membership metadata.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_14rem_auto] lg:items-end">
            <FormField label="Peer public key">
              <Input
                onChange={(event) => setPeerPublicKey(event.target.value)}
                placeholder="Peer public key"
                value={peerPublicKey}
              />
            </FormField>
            <FormField label="Role">
              <Select
                onValueChange={(value) =>
                  setRole(value as FriendGroupMemberRole)
                }
                value={role}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select role" />
                </SelectTrigger>
                <SelectContent>
                  {memberRoles.map((item) => (
                    <SelectItem key={item} value={item}>
                      {item}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormField>
            <Button
              disabled={busy !== "" || peerPublicKey.trim() === ""}
              onClick={() => void addMember()}
              type="button"
            >
              <Plus className="size-4" />
              Add
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>Members</CardTitle>
            <CardDescription>
              Social membership rows for this group.
            </CardDescription>
          </div>
          <Button
            disabled={loading}
            onClick={() => void refresh()}
            size="sm"
            type="button"
            variant="outline"
          >
            <RefreshCw className="size-4" />
            Reload
          </Button>
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
              {Array.from({ length: 4 }).map((_, index) => (
                <Skeleton className="h-14 w-full" key={index} />
              ))}
            </div>
          ) : items.length === 0 ? (
            <EmptyState
              description="This group has no members yet."
              title="No members"
            />
          ) : (
            <DashboardTable>
              <TableHeader>
                <TableRow>
                  <TableHead>Peer</TableHead>
                  <TableHead className="w-44">Role</TableHead>
                  <TableHead className="w-44">Updated</TableHead>
                  <TableHead className="w-32 text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((member) => {
                  const publicKey = member.peer_public_key ?? "";
                  return (
                    <TableRow key={member.id ?? publicKey}>
                      <TableCell>
                        <div className="font-medium">
                          {formatShortKey(publicKey)}
                        </div>
                        <div className="break-all font-mono text-xs text-muted-foreground">
                          {publicKey}
                        </div>
                      </TableCell>
                      <TableCell>
                        <Select
                          disabled={busy !== ""}
                          onValueChange={(value) =>
                            void updateRole(
                              member,
                              value as FriendGroupMemberRole,
                            )
                          }
                          value={member.role ?? "member"}
                        >
                          <SelectTrigger>
                            <SelectValue placeholder="Select role" />
                          </SelectTrigger>
                          <SelectContent>
                            {memberRoles.map((item) => (
                              <SelectItem key={item} value={item}>
                                {item}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {formatDate(member.updated_at)}
                      </TableCell>
                      <TableCell className="text-right">
                        <DeleteConfirmButton
                          description={`Remove ${formatShortKey(publicKey)} from this group and revoke workspace access.`}
                          disabled={busy !== "" || publicKey === ""}
                          onConfirm={() => void removeMember(member)}
                          size="sm"
                          title="Remove group member?"
                        >
                          <Trash2 className="size-4" />
                          Remove
                        </DeleteConfirmButton>
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
