import { Check, Copy, Plus, RefreshCw } from "lucide-react";
import type { KeyboardEvent, MouseEvent } from "react";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

import { createContact, listContacts, type AdminContactObject } from "@gizclaw/adminservice";
import { expectData, toMessage } from "../../components/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

import { ErrorBanner, NoticeBanner } from "../../components/banners";
import { EmptyState } from "../../components/empty-state";
import { FormField } from "../../components/form-field";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { useCursorListPage } from "../../hooks/useCursorListPage";
import { formatDate, formatShortKey } from "../../lib/format";
import { contactDetailPath } from "./social-utils";

export function ContactsListPage(): JSX.Element {
  const navigate = useNavigate();
  const { error, hasNext, items, loading, nextPage, pageNumber, prevPage, refresh } = useCursorListPage<AdminContactObject>(async (query) => {
    const result = await expectData(listContacts({ query }));
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });
  const [ownerPublicKey, setOwnerPublicKey] = useState("");
  const [contactID, setContactID] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [phoneNumber, setPhoneNumber] = useState("");
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [notice, setNotice] = useState<{ message: string; tone: "error" | "success" } | null>(null);
  const [busy, setBusy] = useState("");
  const [copiedID, setCopiedID] = useState("");

  const openContact = (contact: AdminContactObject): void => {
    navigate(contactDetailPath(contact));
  };

  const handleRowKeyDown = (event: KeyboardEvent<HTMLTableRowElement>, contact: AdminContactObject): void => {
    if (isInteractiveTarget(event.target)) {
      return;
    }
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    openContact(contact);
  };

  const copyContactID = async (event: MouseEvent<HTMLButtonElement>, id: string): Promise<void> => {
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
    try {
      const contact = await expectData(
        createContact({
          body: {
            display_name: displayName.trim() || undefined,
            id: contactID.trim() || undefined,
            owner_public_key: ownerPublicKey.trim(),
            phone_number: phoneNumber.trim() || undefined,
          },
        }),
      );
      setOwnerPublicKey("");
      setContactID("");
      setDisplayName("");
      setPhoneNumber("");
      setCreateDialogOpen(false);
      navigate(contactDetailPath(contact));
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
          <>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" onClick={() => setCreateDialogOpen(true)} type="button" variant="outline">
              <Plus className="size-4" />
              New Contact
            </Button>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" disabled={loading} onClick={() => void refresh()} variant="outline">
              <RefreshCw className="size-4" />
              Refresh
            </Button>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { label: "Contacts" }]}
      />

      <PageSummaryCard
        description="Global owner-view contact rows. Each row belongs to one peer address book."
        eyebrow="Social"
        meta={
          <>
            <Badge variant="outline">Page {pageNumber}</Badge>
            <Badge variant="secondary">{items.length} loaded</Badge>
            {hasNext ? <Badge variant="outline">More Available</Badge> : null}
          </>
        }
        title="Contacts"
      />

      {error !== "" ? <ErrorBanner message={error} /> : null}
      {notice !== null ? <NoticeBanner message={notice.message} tone={notice.tone} /> : null}

      <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New Contact</DialogTitle>
            <DialogDescription>Create an owner-scoped contact row in the peer address book.</DialogDescription>
          </DialogHeader>
          <form
            className="flex flex-col gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              void create();
            }}
          >
            <FormField label="Owner public key">
              <Input onChange={(event) => setOwnerPublicKey(event.target.value)} placeholder="Owner peer public key" value={ownerPublicKey} />
            </FormField>
            <FormField label="Contact id">
              <Input onChange={(event) => setContactID(event.target.value)} placeholder="Optional owner-scoped id" value={contactID} />
            </FormField>
            <FormField label="Display name">
              <Input onChange={(event) => setDisplayName(event.target.value)} placeholder="Contact display name" value={displayName} />
            </FormField>
            <FormField label="Phone number">
              <Input onChange={(event) => setPhoneNumber(event.target.value)} placeholder="Optional phone number" value={phoneNumber} />
            </FormField>
            <DialogFooter>
              <Button disabled={busy !== ""} onClick={() => setCreateDialogOpen(false)} type="button" variant="outline">
                Cancel
              </Button>
              <Button disabled={busy !== "" || ownerPublicKey.trim() === "" || (displayName.trim() === "" && phoneNumber.trim() === "")} onClick={() => void create()} type="button">
                Create
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>Contact rows</CardTitle>
            <CardDescription>Cursor-paginated social contact resources.</CardDescription>
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
            <EmptyState description="Contacts will appear here after they are created." title="No contacts" />
          ) : (
            <div className="rounded-md border">
              <Table className="table-fixed">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[42%]">ID</TableHead>
                    <TableHead className="w-[22%]">Display</TableHead>
                    <TableHead className="w-[20%]">Phone</TableHead>
                    <TableHead className="w-32 text-right">Updated</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((contact) => {
                    const id = contactListID(contact);
                    return (
                      <TableRow
                        className="cursor-pointer hover:bg-muted/40"
                        key={id}
                        onClick={() => openContact(contact)}
                        onKeyDown={(event) => handleRowKeyDown(event, contact)}
                        role="link"
                        tabIndex={0}
                      >
                        <TableCell className="min-w-0">
                          <div className="flex min-w-0 items-center gap-1.5" title={id}>
                            <div className="min-w-0 flex-1">
                              <button
                                className="block w-full truncate rounded-sm text-left font-mono text-xs font-medium leading-5 underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                                onClick={(event) => {
                                  event.stopPropagation();
                                  openContact(contact);
                                }}
                                title={id}
                                type="button"
                              >
                                {contactListDisplayID(contact)}
                              </button>
                              <div className="mt-1 truncate font-mono text-xs leading-5 text-muted-foreground" title={contactOwnerLine(contact)}>
                                {contactOwnerLine(contact)}
                              </div>
                            </div>
                            <button
                              aria-label={`Copy contact id ${id}`}
                              className="shrink-0 rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                              onClick={(event) => void copyContactID(event, id)}
                              title="Copy contact id"
                              type="button"
                            >
                              {copiedID === id ? <Check className="size-3 shrink-0 text-emerald-600" /> : <Copy className="size-3 shrink-0" />}
                            </button>
                          </div>
                        </TableCell>
                        <TableCell className="truncate text-sm" title={contact.display_name?.trim() || "—"}>
                          {contact.display_name?.trim() || "—"}
                        </TableCell>
                        <TableCell className="truncate font-mono text-xs text-muted-foreground" title={contact.phone_number?.trim() || "—"}>
                          {contact.phone_number?.trim() || "—"}
                        </TableCell>
                        <TableCell className="text-right text-sm text-muted-foreground">{formatDate(contact.updated_at)}</TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function isInteractiveTarget(target: EventTarget): boolean {
  return target instanceof Element && target.closest("a,button,input,select,textarea") !== null;
}

function contactListID(contact: AdminContactObject): string {
  return `${contact.owner_public_key}:${contact.id}`;
}

function contactListDisplayID(contact: AdminContactObject): string {
  return `${formatShortKey(contact.owner_public_key)}:${formatShortKey(contact.id)}`;
}

function contactOwnerLine(contact: AdminContactObject): string {
  return `${contact.owner_public_key} owns ${contact.id}`;
}
