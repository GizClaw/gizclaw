import { ChevronLeft, RefreshCw, Save, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import { deleteContact, getContact, putContact, type AdminContactObject } from "@gizclaw/gizclaw/admin";
import { expectData, toMessage } from "../../components/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";

import { ErrorBanner, NoticeBanner } from "../../components/banners";
import { DeleteConfirmButton } from "../../components/delete-confirm-button";
import { DetailBlock } from "../../components/detail-block";
import { EmptyState } from "../../components/empty-state";
import { FormField } from "../../components/form-field";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { formatDate, formatShortKey } from "../../lib/format";
import { decodeRouteParam, socialPeerLabel } from "./social-utils";

export function ContactDetailPage(): JSX.Element {
  const params = useParams();
  const navigate = useNavigate();
  const ownerPublicKey = useMemo(() => decodeRouteParam(params.ownerPublicKey ?? ""), [params.ownerPublicKey]);
  const contactID = useMemo(() => decodeRouteParam(params.id ?? ""), [params.id]);
  const [contact, setContact] = useState<AdminContactObject | null>(null);
  const [displayName, setDisplayName] = useState("");
  const [phoneNumber, setPhoneNumber] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState<{ message: string; tone: "error" | "success" } | null>(null);

  const load = async (): Promise<void> => {
    if (ownerPublicKey === "" || contactID === "") {
      setLoading(false);
      setError("Missing contact owner or contact id in the URL.");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const next = await expectData(getContact({ path: { ownerPublicKey, id: contactID } }));
      setContact(next);
      setDisplayName(next.display_name ?? "");
      setPhoneNumber(next.phone_number ?? "");
    } catch (err) {
      setContact(null);
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, [ownerPublicKey, contactID]);

  const save = async (): Promise<void> => {
    setBusy("save");
    setNotice(null);
    try {
      const next = await expectData(
        putContact({
          body: { display_name: displayName.trim(), phone_number: phoneNumber.trim() },
          path: { ownerPublicKey, id: contactID },
        }),
      );
      setContact(next);
      setDisplayName(next.display_name ?? "");
      setPhoneNumber(next.phone_number ?? "");
      setNotice({ message: "Contact saved.", tone: "success" });
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
      await expectData(deleteContact({ path: { ownerPublicKey, id: contactID } }));
      navigate("/social/contacts");
    } catch (err) {
      setNotice({ message: toMessage(err), tone: "error" });
    } finally {
      setBusy("");
    }
  };

  if (ownerPublicKey === "" || contactID === "") {
    return <EmptyState description="Missing contact owner or contact id in the URL." title="Invalid route" />;
  }

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild size="sm" variant="outline">
              <Link to="/social/contacts">
                <ChevronLeft className="size-4" />
                Back to list
              </Link>
            </Button>
            <Button className="min-w-fit shrink-0 whitespace-nowrap" disabled={loading} onClick={() => void load()} size="sm" variant="outline">
              <RefreshCw className="size-4" />
              Reload
            </Button>
            {contact ? (
              <DeleteConfirmButton
                description="Deleting a contact removes only this owner-scoped address book row."
                disabled={busy !== ""}
                onConfirm={() => void remove()}
                size="sm"
                title="Delete contact?"
              >
                <Trash2 className="size-4" />
                Delete
              </DeleteConfirmButton>
            ) : null}
          </>
        }
        items={[
          { href: "/overview", label: "Overview" },
          { href: "/social/contacts", label: "Contacts" },
          { label: formatShortKey(ownerPublicKey) },
        ]}
      />

      <PageSummaryCard
        description={<span className="break-all font-mono text-xs">{`${ownerPublicKey}/${contactID}`}</span>}
        eyebrow="Social Contact"
        meta={contact ? <Badge variant="outline">{socialPeerLabel(contact.owner_public_key)}</Badge> : null}
        title={contact?.display_name?.trim() || contactID}
      />

      {notice !== null ? <NoticeBanner message={notice.message} tone={notice.tone} /> : null}

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      ) : error !== "" ? (
        <ErrorBanner message={error} />
      ) : contact === null ? (
        <EmptyState description="This contact could not be loaded." title="Contact not found" />
      ) : (
        <div className="space-y-4">
          <div className="grid gap-4 xl:grid-cols-2">
            <DetailBlock
              items={[
                ["Owner peer", contact.owner_public_key],
                ["Contact id", contact.id],
                ["Created", formatDate(contact.created_at)],
                ["Updated", formatDate(contact.updated_at)],
              ]}
              title="Contact Row"
            />
            <DetailBlock
              items={[
                ["Display name", contact.display_name],
                ["Phone number", contact.phone_number],
                ["Resource", "Contact"],
                ["ACL", "Not managed by ACL"],
              ]}
              title="Summary"
            />
          </div>

          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">Edit Contact</CardTitle>
              <CardDescription>Update contact display fields for this owner-scoped address book row.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-4 xl:grid-cols-2">
                <FormField label="Display name">
                  <Input onChange={(event) => setDisplayName(event.target.value)} value={displayName} />
                </FormField>
                <FormField label="Phone number">
                  <Input onChange={(event) => setPhoneNumber(event.target.value)} value={phoneNumber} />
                </FormField>
              </div>
              <div className="flex justify-end border-t pt-4">
                <Button disabled={busy !== "" || (displayName.trim() === "" && phoneNumber.trim() === "")} onClick={() => void save()} type="button">
                  <Save className="size-4" />
                  Save Contact
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}
