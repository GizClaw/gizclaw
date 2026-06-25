import { Check, Copy, Plus, RefreshCw } from "lucide-react";
import type { KeyboardEvent, MouseEvent } from "react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { expectData } from "../../components/api";
import { listVoices, type Voice } from "@gizclaw/adminservice";

import { ErrorBanner } from "../../components/banners";
import { EmptyState } from "../../components/empty-state";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { useCursorListPage } from "../../hooks/useCursorListPage";
import { formatDate } from "../../lib/format";

export function VoicesListPage(): JSX.Element {
  const navigate = useNavigate();
  const [copiedID, setCopiedID] = useState("");
  const { error, hasNext, items, loading, nextPage, pageNumber, prevPage, refresh } = useCursorListPage<Voice>(async (query) => {
    const result = await expectData(listVoices({ query }));
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });

  const openVoice = (id: string): void => {
    navigate(`/ai/voices/${encodeURIComponent(id)}`);
  };

  const handleRowKeyDown = (event: KeyboardEvent<HTMLTableRowElement>, id: string): void => {
    if (isInteractiveTarget(event.target)) {
      return;
    }
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    openVoice(id);
  };

  const copyVoiceID = async (event: MouseEvent<HTMLButtonElement>, id: string): Promise<void> => {
    event.stopPropagation();
    await navigator.clipboard.writeText(id);
    setCopiedID(id);
    window.setTimeout(() => {
      setCopiedID((current) => (current === id ? "" : current));
    }, 1500);
  };

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" variant="outline">
              <Link to="/resources?kind=Voice">
                <Plus className="size-4" />
                New Voice
              </Link>
            </Button>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" onClick={() => void refresh()} variant="outline">
              <RefreshCw className="size-4" />
              Refresh
            </Button>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { label: "Voices" }]}
      />

      <PageSummaryCard
        description="Global voice catalog across providers, including both manually managed entries and synced upstream voices."
        eyebrow="AI"
        meta={
          <>
            <Badge variant="outline">Page {pageNumber}</Badge>
            <Badge variant="secondary">{items.length} loaded</Badge>
            {hasNext ? <Badge variant="outline">More Available</Badge> : null}
          </>
        }
        title="Voices"
      />

      {error !== "" ? <ErrorBanner message={error} /> : null}

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>Voice catalog</CardTitle>
            <CardDescription>Provider voices stored in the shared catalog and ready for downstream use.</CardDescription>
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
            <EmptyState description="Voices will appear here after manual creation or provider sync." title="No voices" />
          ) : (
            <div className="rounded-md border">
              <Table className="table-fixed">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-48">ID</TableHead>
                    <TableHead>Provider</TableHead>
                    <TableHead>Name</TableHead>
                    <TableHead>Source</TableHead>
                    <TableHead className="text-right">Updated</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((voice) => (
                    <TableRow
                      className="cursor-pointer hover:bg-muted/40"
                      key={voice.id}
                      onClick={() => openVoice(voice.id)}
                      onKeyDown={(event) => handleRowKeyDown(event, voice.id)}
                      role="link"
                      tabIndex={0}
                    >
                      <TableCell className="w-48 max-w-48">
                        <button
                          className="inline-flex w-44 max-w-44 items-center gap-2 rounded-sm text-left font-mono text-xs font-medium underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          onClick={(event) => void copyVoiceID(event, voice.id)}
                          title={`Copy full ID: ${voice.id}`}
                          type="button"
                        >
                          <span className="truncate">{compactVoiceID(voice.id)}</span>
                          {copiedID === voice.id ? <Check className="size-3 shrink-0 text-emerald-600" /> : <Copy className="size-3 shrink-0" />}
                        </button>
                      </TableCell>
                      <TableCell className="text-sm font-medium">
                        <ProviderLabel kind={voice.provider.kind} name={voice.provider.name} />
                      </TableCell>
                      <TableCell className="max-w-[22rem]">
                        <div className="block truncate font-medium">{voice.name?.trim() || "Unnamed voice"}</div>
                      </TableCell>
                      <TableCell>
                        <Badge variant={voice.source === "sync" ? "secondary" : "outline"}>{voice.source}</Badge>
                      </TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground">{formatDate(voice.updated_at)}</TableCell>
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

function compactVoiceID(id: string): string {
  const trimmed = id.trim();
  if (trimmed === "") {
    return "—";
  }
  const parts = trimmed.split(":");
  const last = parts[parts.length - 1] ?? trimmed;
  if (last.length <= 28) {
    return last;
  }
  return `${last.slice(0, 14)}...${last.slice(-8)}`;
}

function ProviderLabel({ kind, name }: { kind: string; name: string }): JSX.Element {
  return (
    <span className="inline-flex max-w-[14rem] items-baseline font-mono text-xs">
      <span className="shrink-0 text-muted-foreground">{providerPrefix(kind)}/</span>
      <span className="truncate text-foreground">{name}</span>
    </span>
  );
}

function providerPrefix(kind: string): string {
  switch (kind) {
    case "minimax-tenant":
      return "minimax";
    case "volc-tenant":
      return "volc";
    default:
      return kind;
  }
}

function isInteractiveTarget(target: EventTarget): boolean {
  return target instanceof Element && target.closest("a,button,input,select,textarea") !== null;
}
