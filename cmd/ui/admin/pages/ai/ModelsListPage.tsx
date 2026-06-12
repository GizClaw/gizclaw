import { Copy, Plus, RefreshCw } from "lucide-react";
import type { MouseEvent } from "react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { expectData } from "../../components/api";
import { listModels, type Model } from "@gizclaw/adminservice";

import { ErrorBanner } from "../../components/banners";
import { EmptyState } from "../../components/empty-state";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { useCursorListPage } from "../../hooks/useCursorListPage";
import { formatDate } from "../../lib/format";

export function ModelsListPage(): JSX.Element {
  const navigate = useNavigate();
  const [copiedID, setCopiedID] = useState("");
  const { error, hasNext, items, loading, nextPage, pageNumber, prevPage, refresh } = useCursorListPage<Model>(async (query) => {
    const result = await expectData(listModels({ query }));
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });

  const copyModelID = async (event: MouseEvent<HTMLButtonElement>, id: string): Promise<void> => {
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
              <Link to="/resources?kind=Model">
                <Plus className="size-4" />
                New Model
              </Link>
            </Button>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" onClick={() => void refresh()} variant="outline">
              <RefreshCw className="size-4" />
              Refresh
            </Button>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { label: "Models" }]}
      />

      <PageSummaryCard
        description="Global model catalog across providers, including manually managed entries and synced upstream models."
        eyebrow="AI"
        meta={
          <>
            <Badge variant="outline">Page {pageNumber}</Badge>
            <Badge variant="secondary">{items.length} loaded</Badge>
            {hasNext ? <Badge variant="outline">More Available</Badge> : null}
          </>
        }
        title="Models"
      />

      {error !== "" ? <ErrorBanner message={error} /> : null}

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>Model catalog</CardTitle>
            <CardDescription>Provider models stored in the shared catalog and ready for workflow use.</CardDescription>
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
            <EmptyState description="Models will appear here after manual creation or provider sync." title="No models" />
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-48">ID</TableHead>
                    <TableHead className="w-24">Kind</TableHead>
                    <TableHead>Provider</TableHead>
                    <TableHead>Name</TableHead>
                    <TableHead>Source</TableHead>
                    <TableHead className="text-right">Updated</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((model) => (
                    <TableRow key={model.id}>
                      <TableCell className="w-48 max-w-48">
                        <button
                          className="inline-flex w-44 max-w-44 items-center gap-2 rounded-sm text-left font-mono text-xs font-medium underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          onClick={(event) => void copyModelID(event, model.id)}
                          title={`Copy full ID: ${model.id}`}
                          type="button"
                        >
                          <span className="truncate">{compactModelID(model.id)}</span>
                          <Copy className="size-3 shrink-0 text-muted-foreground" />
                        </button>
                        {copiedID === model.id ? <div className="text-[0.65rem] text-muted-foreground">Copied</div> : null}
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline">{model.kind}</Badge>
                      </TableCell>
                      <TableCell className="text-sm font-medium">
                        <ProviderLabel kind={model.provider.kind} name={model.provider.name} />
                      </TableCell>
                      <TableCell className="max-w-[24rem]">
                        <button
                          className="block max-w-full truncate text-left font-medium underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          onClick={() => navigate(`/ai/models/${encodeURIComponent(model.id)}`)}
                          title={model.name?.trim() || model.id}
                          type="button"
                        >
                          {model.name?.trim() || "Unnamed model"}
                        </button>
                        <div className="block truncate text-xs text-muted-foreground" title={model.description?.trim() || undefined}>
                          {model.description?.trim() || "No description"}
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant={model.source === "sync" ? "secondary" : "outline"}>{model.source}</Badge>
                      </TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground">{formatDate(model.updated_at)}</TableCell>
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

function compactModelID(id: string): string {
  const trimmed = id.trim();
  if (trimmed === "") {
    return "-";
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
      <span className="shrink-0 text-muted-foreground">{kind}/</span>
      <span className="truncate text-foreground">{name}</span>
    </span>
  );
}
