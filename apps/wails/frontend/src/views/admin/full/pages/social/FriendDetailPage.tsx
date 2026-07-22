import { ChevronLeft, RefreshCw, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import {
  deleteFriend,
  getFriend,
  type AdminFriendObject,
} from "@gizclaw/gizclaw/admin";
import { expectData, toMessage } from "@/dashboard";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

import { ErrorBanner, NoticeBanner } from "@/dashboard";
import { DashboardDeleteButton as DeleteConfirmButton } from "@/dashboard";
import { DetailBlock } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { formatDate, formatShortKey } from "../../lib/format";
import { WorkspaceHistoryPanel } from "./WorkspaceHistoryPanel";
import { decodeRouteParam, socialPeerLabel } from "./social-utils";

export function FriendDetailPage(): JSX.Element {
  const params = useParams();
  const navigate = useNavigate();
  const ownerPublicKey = useMemo(
    () => decodeRouteParam(params.ownerPublicKey ?? ""),
    [params.ownerPublicKey],
  );
  const friendID = useMemo(
    () => decodeRouteParam(params.id ?? ""),
    [params.id],
  );
  const [friend, setFriend] = useState<AdminFriendObject | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState<{
    message: string;
    tone: "error" | "success";
  } | null>(null);
  const [busy, setBusy] = useState("");

  const load = async (): Promise<void> => {
    if (ownerPublicKey === "" || friendID === "") {
      setLoading(false);
      setError("Missing friend owner or friend id in the URL.");
      return;
    }
    setLoading(true);
    setError("");
    try {
      setFriend(
        await expectData(getFriend({ path: { ownerPublicKey, id: friendID } })),
      );
    } catch (err) {
      setFriend(null);
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, [ownerPublicKey, friendID]);

  const remove = async (): Promise<void> => {
    setBusy("delete");
    setNotice(null);
    try {
      await expectData(
        deleteFriend({ path: { ownerPublicKey, id: friendID } }),
      );
      navigate("/social/friends");
    } catch (err) {
      setNotice({ message: toMessage(err), tone: "error" });
    } finally {
      setBusy("");
    }
  };

  if (ownerPublicKey === "" || friendID === "") {
    return (
      <EmptyState
        description="Missing friend owner or friend id in the URL."
        title="Invalid route"
      />
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild size="sm" variant="outline">
              <Link to="/social/friends">
                <ChevronLeft className="size-4" />
                Back to list
              </Link>
            </Button>
            <Button
              className="min-w-fit shrink-0 whitespace-nowrap"
              disabled={loading}
              onClick={() => void load()}
              size="sm"
              variant="outline"
            >
              <RefreshCw className="size-4" />
              Reload
            </Button>
            {friend ? (
              <DeleteConfirmButton
                description="Deleting a friend relation removes both owner-view rows and the backing direct workspace."
                disabled={busy !== ""}
                onConfirm={() => void remove()}
                size="sm"
                title="Delete friend relation?"
              >
                <Trash2 className="size-4" />
                Delete
              </DeleteConfirmButton>
            ) : null}
          </>
        }
        items={[
          { href: "/overview", label: "Overview" },
          { href: "/social/friends", label: "Friends" },
          { label: formatShortKey(ownerPublicKey) },
        ]}
      />

      <PageSummaryCard
        description={
          <span className="break-all font-mono text-xs">{friendID}</span>
        }
        eyebrow="Social Friend"
        meta={
          friend ? (
            <Badge variant="outline">
              {formatShortKey(friend.workspace_name)}
            </Badge>
          ) : null
        }
        title={friend ? socialPeerLabel(friend.owner_public_key) : "Friend"}
      />

      {notice !== null ? (
        <NoticeBanner message={notice.message} tone={notice.tone} />
      ) : null}

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      ) : error !== "" ? (
        <ErrorBanner message={error} />
      ) : friend === null ? (
        <EmptyState
          description="This friend relation could not be loaded."
          title="Friend not found"
        />
      ) : (
        <div className="space-y-4">
          <div className="grid gap-4 xl:grid-cols-2">
            <DetailBlock
              items={[
                ["Owner peer", friend.owner_public_key],
                ["Friend peer", friend.peer_public_key],
                ["Friend id", friend.id],
                ["Workspace", friend.workspace_name],
              ]}
              title="Friend Row"
            />
            <DetailBlock
              items={[
                ["Owner label", socialPeerLabel(friend.owner_public_key)],
                ["Friend label", socialPeerLabel(friend.peer_public_key)],
                ["Created", formatDate(friend.created_at)],
                ["Updated", formatDate(friend.updated_at)],
              ]}
              title="Summary"
            />
          </div>

          <WorkspaceHistoryPanel workspaceName={friend.workspace_name} />
        </div>
      )}
    </div>
  );
}
