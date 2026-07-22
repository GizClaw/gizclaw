import { Check, Copy, Plus, RefreshCw } from "lucide-react";
import { DashboardActionButton } from "@/dashboard";
import { DashboardPager } from "@/dashboard";
import { DashboardTable } from "@/dashboard";
import type { KeyboardEvent, MouseEvent } from "react";
import { useState } from "react";
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { expectData } from "@/dashboard";
import { listModels, type Model } from "@gizclaw/gizclaw/admin";

import { ErrorBanner } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { useDashboardCursorPage as useCursorListPage } from "@/dashboard";
import { formatDate } from "../../lib/format";

export function ModelsListPage(): JSX.Element {
  const navigate = useNavigate();
  const [copiedID, setCopiedID] = useState("");
  const {
    error,
    hasNext,
    items,
    loading,
    nextPage,
    pageNumber,
    prevPage,
    refresh,
  } = useCursorListPage<Model>(async (query) => {
    const result = await expectData(listModels({ query }));
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });

  const openModel = (id: string): void => {
    navigate(`/ai/models/${encodeURIComponent(id)}`);
  };

  const handleRowKeyDown = (
    event: KeyboardEvent<HTMLTableRowElement>,
    id: string,
  ): void => {
    if (isInteractiveTarget(event.target)) {
      return;
    }
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    openModel(id);
  };

  const copyModelID = async (
    event: MouseEvent<HTMLButtonElement>,
    id: string,
  ): Promise<void> => {
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
            <DashboardActionButton asChild>
              <Link to="/resources?kind=Model">
                <Plus className="size-4" />
                New Model
              </Link>
            </DashboardActionButton>
            <DashboardActionButton onClick={() => void refresh()}>
              <RefreshCw className="size-4" />
              Refresh
            </DashboardActionButton>
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
            <CardDescription>
              Provider models stored in the shared catalog and ready for
              workflow use.
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
              description="Models will appear here after manual creation or provider sync."
              title="No models"
            />
          ) : (
            <DashboardTable className="table-fixed">
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
                  <TableRow
                    className="cursor-pointer hover:bg-muted/40"
                    key={model.id}
                    onClick={() => openModel(model.id)}
                    onKeyDown={(event) => handleRowKeyDown(event, model.id)}
                    role="link"
                    tabIndex={0}
                  >
                    <TableCell className="w-48 max-w-48">
                      <button
                        className="inline-flex w-44 max-w-44 items-center gap-2 rounded-sm text-left font-mono text-xs font-medium underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        onClick={(event) => void copyModelID(event, model.id)}
                        title={`Copy full ID: ${model.id}`}
                        type="button"
                      >
                        <span className="truncate">
                          {compactModelID(model.id)}
                        </span>
                        {copiedID === model.id ? (
                          <Check className="size-3 shrink-0 text-emerald-600" />
                        ) : (
                          <Copy className="size-3 shrink-0" />
                        )}
                      </button>
                    </TableCell>
                    <TableCell className="w-24">
                      <Badge variant="outline">{model.kind}</Badge>
                    </TableCell>
                    <TableCell className="text-sm font-medium">
                      <ProviderLabel
                        kind={model.provider.kind}
                        name={model.provider.name}
                      />
                    </TableCell>
                    <TableCell className="max-w-[24rem]">
                      <div
                        className="block max-w-full truncate font-medium"
                        title={model.name?.trim() || model.id}
                      >
                        {model.name?.trim() || "Unnamed model"}
                      </div>
                      <div
                        className="block truncate text-xs text-muted-foreground"
                        title={model.description?.trim() || undefined}
                      >
                        {model.description?.trim() || "No description"}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          model.source === "sync" ? "secondary" : "outline"
                        }
                      >
                        {model.source}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right text-sm text-muted-foreground">
                      {formatDate(model.updated_at)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </DashboardTable>
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

function ProviderLabel({
  kind,
  name,
}: {
  kind: string;
  name: string;
}): JSX.Element {
  return (
    <span className="inline-flex max-w-[14rem] items-baseline font-mono text-xs">
      <span className="shrink-0 text-muted-foreground">{kind}/</span>
      <span className="truncate text-foreground">{name}</span>
    </span>
  );
}

function isInteractiveTarget(target: EventTarget): boolean {
  return (
    target instanceof Element &&
    target.closest("a,button,input,select,textarea") !== null
  );
}
