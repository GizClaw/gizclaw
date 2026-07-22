import { useMemo, useState } from "react";
import { DashboardActionButton } from "@/dashboard";
import { DashboardPager } from "@/dashboard";
import { DashboardTable } from "@/dashboard";
import type { KeyboardEvent, MouseEvent } from "react";
import { Check, Copy, Eye, EyeOff, Plus, RefreshCw } from "lucide-react";
import { Link, useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { expectData } from "@/dashboard";
import { listCredentials, type Credential } from "@gizclaw/gizclaw/admin";

import { ErrorBanner } from "@/dashboard";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { EmptyState } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { useDashboardCursorPage as useCursorListPage } from "@/dashboard";
import { formatDate } from "../../lib/format";

export function CredentialsListPage(): JSX.Element {
  const navigate = useNavigate();
  const [selectedCredential, setSelectedCredential] =
    useState<Credential | null>(null);
  const [copiedName, setCopiedName] = useState("");
  const {
    error,
    hasNext,
    items,
    loading,
    nextPage,
    pageNumber,
    prevPage,
    refresh,
  } = useCursorListPage<Credential>(async (query) => {
    const result = await expectData(listCredentials({ query }));
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });

  const openCredential = (name: string): void => {
    navigate(`/providers/credentials/${encodeURIComponent(name)}`);
  };

  const handleRowKeyDown = (
    event: KeyboardEvent<HTMLTableRowElement>,
    name: string,
  ): void => {
    if (isInteractiveTarget(event.target)) {
      return;
    }
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    openCredential(name);
  };

  const openBodyDialog = (
    event: MouseEvent<HTMLButtonElement>,
    credential: Credential,
  ): void => {
    event.stopPropagation();
    setSelectedCredential(credential);
  };

  const copyCredentialName = async (
    event: MouseEvent<HTMLButtonElement>,
    name: string,
  ): Promise<void> => {
    event.stopPropagation();
    await navigator.clipboard.writeText(name);
    setCopiedName(name);
    window.setTimeout(() => {
      setCopiedName((current) => (current === name ? "" : current));
    }, 1500);
  };

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <DashboardActionButton asChild>
              <Link to="/resources?kind=Credential">
                <Plus className="size-4" />
                New Credential
              </Link>
            </DashboardActionButton>
            <DashboardActionButton onClick={() => void refresh()}>
              <RefreshCw className="size-4" />
              Refresh
            </DashboardActionButton>
          </>
        }
        items={[
          { href: "/overview", label: "Overview" },
          { label: "Credentials" },
        ]}
      />

      <PageSummaryCard
        description="Shared provider credentials used by services like MiniMax tenants and future external integrations."
        eyebrow="Providers"
        meta={
          <>
            <Badge variant="outline">Page {pageNumber}</Badge>
            <Badge variant="secondary">{items.length} loaded</Badge>
            {hasNext ? <Badge variant="outline">More Available</Badge> : null}
          </>
        }
        title="Credentials"
      />

      {error !== "" ? <ErrorBanner message={error} /> : null}

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>Credential catalog</CardTitle>
            <CardDescription>
              Stored authentication entries keyed by provider and credential
              body shape.
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
              description="Credentials will appear here after they are created through the admin API."
              title="No credentials"
            />
          ) : (
            <DashboardTable className="table-fixed">
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[24%]">Credential ID</TableHead>
                  <TableHead className="w-32">Provider</TableHead>
                  <TableHead className="w-36">Method</TableHead>
                  <TableHead>Description</TableHead>
                  <TableHead className="w-28 text-right">Body Keys</TableHead>
                  <TableHead className="w-40 text-right">Updated</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((credential) => (
                  <TableRow
                    className="cursor-pointer hover:bg-muted/40"
                    key={credential.name}
                    onClick={() => openCredential(credential.name)}
                    onKeyDown={(event) =>
                      handleRowKeyDown(event, credential.name)
                    }
                    role="link"
                    tabIndex={0}
                  >
                    <TableCell className="min-w-0">
                      <div className="flex min-w-0 items-center gap-1.5">
                        <button
                          className="min-w-0 truncate rounded-sm text-left font-medium underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          onClick={(event) => {
                            event.stopPropagation();
                            openCredential(credential.name);
                          }}
                          title={credential.name}
                          type="button"
                        >
                          {credential.name}
                        </button>
                        <button
                          aria-label={`Copy credential name ${credential.name}`}
                          className="shrink-0 rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          onClick={(event) =>
                            void copyCredentialName(event, credential.name)
                          }
                          title="Copy credential name"
                          type="button"
                        >
                          {copiedName === credential.name ? (
                            <Check className="size-3 shrink-0 text-emerald-600" />
                          ) : (
                            <Copy className="size-3 shrink-0" />
                          )}
                        </button>
                      </div>
                    </TableCell>
                    <TableCell className="truncate" title={credential.provider}>
                      {credential.provider}
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">
                        {credentialAuthSummary(credential)}
                      </Badge>
                    </TableCell>
                    <TableCell
                      className="truncate text-sm text-muted-foreground"
                      title={credential.description?.trim() || "—"}
                    >
                      {credential.description?.trim() || "—"}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        aria-label={`View body keys for ${credential.name}`}
                        className="h-8 min-w-fit gap-2 px-2 text-xs"
                        onClick={(event) => openBodyDialog(event, credential)}
                        type="button"
                        variant="outline"
                      >
                        <Eye className="size-3.5" />
                        {Object.keys(credential.body).length}
                      </Button>
                    </TableCell>
                    <TableCell className="text-right text-sm text-muted-foreground">
                      {formatDate(credential.updated_at)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </DashboardTable>
          )}
        </CardContent>
      </Card>
      {selectedCredential != null ? (
        <CredentialBodyDialog
          credential={selectedCredential}
          onClose={() => setSelectedCredential(null)}
        />
      ) : null}
    </div>
  );
}

function CredentialBodyDialog({
  credential,
  onClose,
}: {
  credential: Credential;
  onClose: () => void;
}): JSX.Element {
  const [revealed, setRevealed] = useState(false);
  const entries = useMemo(
    () => Object.entries(credential.body),
    [credential.body],
  );

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) {
          onClose();
        }
      }}
    >
      <DialogContent className="max-w-3xl">
        <DialogHeader className="pr-10">
          <div className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            Credential body
          </div>
          <DialogTitle>{credential.name}</DialogTitle>
          <DialogDescription>
            {credential.provider} · {credentialAuthSummary(credential)}
          </DialogDescription>
        </DialogHeader>
        <div className="flex justify-end">
          <Button
            className="h-8 gap-2 px-3 text-xs"
            onClick={() => setRevealed((value) => !value)}
            type="button"
            variant="outline"
          >
            {revealed ? (
              <EyeOff className="size-3.5" />
            ) : (
              <Eye className="size-3.5" />
            )}
            {revealed ? "Hide values" : "Show values"}
          </Button>
        </div>
        <div className="max-h-[60vh] overflow-auto p-5">
          {entries.length === 0 ? (
            <EmptyState
              description="This credential has an empty body."
              title="No body keys"
            />
          ) : (
            <DashboardTable>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-56">Key</TableHead>
                  <TableHead>Value</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {entries.map(([key, value]) => {
                  const formatted = formatCredentialBodyValue(value);
                  return (
                    <TableRow key={key}>
                      <TableCell className="font-mono text-xs font-medium">
                        {key}
                      </TableCell>
                      <TableCell className="break-all font-mono text-xs">
                        {revealed
                          ? formatted
                          : maskCredentialBodyValue(formatted)}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </DashboardTable>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function formatCredentialBodyValue(value: unknown): string {
  if (typeof value === "string") {
    return value;
  }
  if (value == null) {
    return "";
  }
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function credentialAuthSummary(credential: Credential): string {
  const keys = Object.keys(credential.body).filter((key) => {
    const value = (credential.body as Record<string, unknown>)[key];
    return value !== undefined && value !== "";
  });
  return keys.length === 0 ? "empty body" : keys.join(", ");
}

function maskCredentialBodyValue(value: string): string {
  if (value === "") {
    return "—";
  }
  if (value.length <= 2) {
    return "**";
  }
  if (value.length <= 8) {
    return `${value.slice(0, 1)}****${value.slice(-1)}`;
  }
  return `${value.slice(0, 6)}******${value.slice(-4)}`;
}

function isInteractiveTarget(target: EventTarget): boolean {
  return (
    target instanceof Element &&
    target.closest("a,button,input,select,textarea") !== null
  );
}
