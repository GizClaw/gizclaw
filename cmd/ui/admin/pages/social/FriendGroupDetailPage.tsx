import { ChevronLeft, RefreshCw, Save, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import { deleteFriendGroup, getFriendGroup, putFriendGroup, type FriendGroupObject } from "@gizclaw/adminservice";
import { expectData, toMessage } from "../../components/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";

import { ErrorBanner, NoticeBanner } from "../../components/banners";
import { DeleteConfirmButton } from "../../components/delete-confirm-button";
import { DetailBlock } from "../../components/detail-block";
import { EmptyState } from "../../components/empty-state";
import { FormField } from "../../components/form-field";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { formatDate, formatShortKey } from "../../lib/format";
import { FriendGroupInviteTokenPanel } from "./FriendGroupInviteTokenPanel";
import { FriendGroupMemberEditor } from "./FriendGroupMemberEditor";
import { WorkspaceHistoryPanel } from "./WorkspaceHistoryPanel";
import { decodeRouteParam, socialWorkspaceName } from "./social-utils";

export function FriendGroupDetailPage(): JSX.Element {
  const params = useParams();
  const navigate = useNavigate();
  const groupID = useMemo(() => decodeRouteParam(params.id ?? ""), [params.id]);
  const [group, setGroup] = useState<FriendGroupObject | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState<{ message: string; tone: "error" | "success" } | null>(null);

  const load = async (): Promise<void> => {
    if (groupID === "") {
      setLoading(false);
      setError("Missing friend group id in the URL.");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const next = await expectData(getFriendGroup({ path: { id: groupID } }));
      setGroup(next);
      setName(next.name ?? "");
      setDescription(next.description ?? "");
    } catch (err) {
      setGroup(null);
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, [groupID]);

  const save = async (): Promise<void> => {
    setBusy("save");
    setNotice(null);
    try {
      const next = await expectData(putFriendGroup({ body: { name: name.trim() || undefined, description: description.trim() || undefined }, path: { id: groupID } }));
      setGroup(next);
      setName(next.name ?? "");
      setDescription(next.description ?? "");
      setNotice({ message: "Friend group saved.", tone: "success" });
    } catch (err) {
      setNotice({ message: toMessage(err), tone: "error" });
    } finally {
      setBusy("");
    }
  };

  const remove = async (): Promise<void> => {
    setBusy("delete");
    setNotice(null);
    try {
      await expectData(deleteFriendGroup({ path: { id: groupID } }));
      navigate("/social/friend-groups");
    } catch (err) {
      setNotice({ message: toMessage(err), tone: "error" });
    } finally {
      setBusy("");
    }
  };

  if (groupID === "") {
    return <EmptyState description="Missing friend group id in the URL." title="Invalid route" />;
  }

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild size="sm" variant="outline">
              <Link to="/social/friend-groups">
                <ChevronLeft className="size-4" />
                Back to list
              </Link>
            </Button>
            <Button className="min-w-fit shrink-0 whitespace-nowrap" disabled={loading} onClick={() => void load()} size="sm" variant="outline">
              <RefreshCw className="size-4" />
              Reload
            </Button>
            {group ? (
              <DeleteConfirmButton
                description="Deleting a group removes group metadata, invite token, membership rows, workspace access, and the backing workspace."
                disabled={busy !== ""}
                onConfirm={() => void remove()}
                size="sm"
                title="Delete friend group?"
              >
                <Trash2 className="size-4" />
                Delete
              </DeleteConfirmButton>
            ) : null}
          </>
        }
        items={[
          { href: "/overview", label: "Overview" },
          { href: "/social/friend-groups", label: "Friend Groups" },
          { label: formatShortKey(groupID) },
        ]}
      />

      <PageSummaryCard
        description={group?.description?.trim() || <span className="break-all font-mono text-xs">{groupID}</span>}
        eyebrow="Social Friend Group"
        meta={group ? <Badge variant="outline">{socialWorkspaceName(group.workspace_name)}</Badge> : null}
        title={group?.name?.trim() || groupID}
      />

      {notice !== null ? <NoticeBanner message={notice.message} tone={notice.tone} /> : null}

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-80 w-full" />
        </div>
      ) : error !== "" ? (
        <ErrorBanner message={error} />
      ) : group === null ? (
        <EmptyState description="This friend group could not be loaded." title="Friend group not found" />
      ) : (
        <Tabs className="space-y-4" defaultValue="info">
          <TabsList className="grid h-auto w-full grid-cols-4 lg:w-[34rem]">
            <TabsTrigger value="info">Info</TabsTrigger>
            <TabsTrigger value="members">Members</TabsTrigger>
            <TabsTrigger value="invite-token">Invite Token</TabsTrigger>
            <TabsTrigger value="history">History</TabsTrigger>
          </TabsList>

          <TabsContent className="space-y-4" value="info">
            <div className="grid gap-4 xl:grid-cols-2">
              <DetailBlock
                items={[
                  ["Group id", group.id],
                  ["Workspace", group.workspace_name],
                  ["Created", formatDate(group.created_at)],
                  ["Updated", formatDate(group.updated_at)],
                ]}
                title="Group"
              />
              <DetailBlock
                items={[
                  ["Resource", "FriendGroup"],
                  ["Implicit owner", group.created_by_peer_public_key ? group.created_by_peer_public_key : "None"],
                  ["My role", group.my_role ?? "None"],
                  ["Workspace access", "Members only"],
                ]}
                title="Runtime Model"
              />
            </div>

            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-base">Edit Group</CardTitle>
                <CardDescription>Update group metadata. Member roles are managed on the Members tab.</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid gap-4 xl:grid-cols-2">
                  <FormField label="Name">
                    <Input onChange={(event) => setName(event.target.value)} value={name} />
                  </FormField>
                  <FormField label="Description">
                    <Textarea onChange={(event) => setDescription(event.target.value)} value={description} />
                  </FormField>
                </div>
                <div className="flex justify-end border-t pt-4">
                  <Button disabled={busy !== "" || name.trim() === ""} onClick={() => void save()} type="button">
                    <Save className="size-4" />
                    Save Info
                  </Button>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent className="space-y-4" value="members">
            <FriendGroupMemberEditor groupID={groupID} />
          </TabsContent>

          <TabsContent className="space-y-4" value="invite-token">
            <FriendGroupInviteTokenPanel groupID={groupID} />
          </TabsContent>

          <TabsContent className="space-y-4" value="history">
            <WorkspaceHistoryPanel workspaceName={group.workspace_name} />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}
