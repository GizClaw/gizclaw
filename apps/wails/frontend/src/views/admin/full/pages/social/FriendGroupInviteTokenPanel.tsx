import { RefreshCw, Save, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";

import { deleteFriendGroupInviteToken, getFriendGroupInviteToken, putFriendGroupInviteToken } from "@gizclaw/gizclaw/admin";
import { expectData, toMessage } from "../../components/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";

import { ErrorBanner, NoticeBanner } from "../../components/banners";
import { DateTimeInput } from "../../components/date-time-input";
import { DeleteConfirmButton } from "../../components/delete-confirm-button";
import { FormField } from "../../components/form-field";
import { formatDate } from "../../lib/format";

type FriendGroupInviteTokenPanelProps = {
  groupID: string;
};

export function FriendGroupInviteTokenPanel({ groupID }: FriendGroupInviteTokenPanelProps): JSX.Element {
  const [token, setToken] = useState("");
  const [expiresAt, setExpiresAt] = useState(defaultExpiresAt());
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState<{ message: string; tone: "error" | "success" } | null>(null);

  const load = async (): Promise<void> => {
    setLoading(true);
    setError("");
    try {
      const current = await expectData(getFriendGroupInviteToken({ path: { id: groupID } }));
      setToken(current.invite_token ?? "");
      setExpiresAt(current.expires_at ?? defaultExpiresAt());
    } catch (err) {
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
    setError("");
    try {
      const current = await expectData(putFriendGroupInviteToken({ body: { invite_token: token.trim(), expires_at: expiresAt }, path: { id: groupID } }));
      setToken(current.invite_token ?? "");
      setExpiresAt(current.expires_at ?? defaultExpiresAt());
      setNotice({ message: "Invite token saved.", tone: "success" });
    } catch (err) {
      setNotice({ message: toMessage(err), tone: "error" });
    } finally {
      setBusy("");
    }
  };

  const clear = async (): Promise<void> => {
    setBusy("clear");
    setNotice(null);
    setError("");
    try {
      await expectData(deleteFriendGroupInviteToken({ path: { id: groupID } }));
      setToken("");
      setExpiresAt(defaultExpiresAt());
      setNotice({ message: "Invite token cleared.", tone: "success" });
    } catch (err) {
      setNotice({ message: toMessage(err), tone: "error" });
    } finally {
      setBusy("");
    }
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
        <div className="space-y-1">
          <CardTitle>Invite Token</CardTitle>
          <CardDescription>Admin can directly set or clear the group invite token and expiration time.</CardDescription>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={token.trim() === "" ? "outline" : "secondary"}>{token.trim() === "" ? "Absent" : "Present"}</Badge>
          <Button disabled={loading || busy !== ""} onClick={() => void load()} size="sm" type="button" variant="outline">
            <RefreshCw className="size-4" />
            Reload
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {error !== "" ? <ErrorBanner message={error} /> : null}
        {notice !== null ? <NoticeBanner message={notice.message} tone={notice.tone} /> : null}

        {loading ? (
          <div className="space-y-3">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
          </div>
        ) : (
          <>
            <div className="grid gap-4 xl:grid-cols-2">
              <FormField description="Leave empty only when you intend to clear the token." label="Invite token">
                <Input onChange={(event) => setToken(event.target.value)} placeholder="Invite token" value={token} />
              </FormField>
              <FormField description={`Current effective expiration: ${formatDate(expiresAt)}`} label="Expires at">
                <DateTimeInput disabled={busy !== ""} onChange={setExpiresAt} value={expiresAt} />
              </FormField>
            </div>
            <div className="flex flex-wrap justify-end gap-2 border-t pt-4">
              <DeleteConfirmButton
                description="Clear the current group invite token."
                disabled={busy !== "" || token.trim() === ""}
                onConfirm={() => void clear()}
                size="sm"
                title="Clear invite token?"
              >
                <Trash2 className="size-4" />
                Clear
              </DeleteConfirmButton>
              <Button disabled={busy !== "" || token.trim() === "" || expiresAt.trim() === ""} onClick={() => void save()} size="sm" type="button">
                <Save className="size-4" />
                Save Token
              </Button>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}

function defaultExpiresAt(): string {
  return new Date(Date.now() + 5 * 60 * 1000).toISOString().replace(/\.\d{3}Z$/, "Z");
}
