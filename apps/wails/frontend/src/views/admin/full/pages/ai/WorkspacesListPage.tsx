import { Check, Copy, Plus, RefreshCw } from "lucide-react";
import { DashboardActionButton } from "@/dashboard";
import { DashboardPager } from "@/dashboard";
import { DashboardTable } from "@/dashboard";
import type { KeyboardEvent, MouseEvent } from "react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
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
import { listWorkspaces, type Workspace } from "@gizclaw/gizclaw/admin";

import { ErrorBanner } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { useDashboardCursorPage as useCursorListPage } from "@/dashboard";
import { formatDate } from "../../lib/format";

export function WorkspacesListPage(): JSX.Element {
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
  } = useCursorListPage<Workspace>(async (query) => {
    const result = await expectData(listWorkspaces({ query }));
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });

  const openWorkspace = (id: string): void => {
    navigate(`/resources?kind=Workspace&id=${encodeURIComponent(id)}`);
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
    openWorkspace(id);
  };

  const copyWorkspaceID = async (
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
              <Link to="/resources?kind=Workspace">
                <Plus className="size-4" />
                New Workspace
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
          { label: "Workspaces" },
        ]}
      />

      <PageSummaryCard
        description="Concrete workspace instances bound to workflow documents with optional instantiation parameters."
        eyebrow="AI"
        meta={
          <>
            <Badge variant="outline">Page {pageNumber}</Badge>
            <Badge variant="secondary">{items.length} loaded</Badge>
            {hasNext ? <Badge variant="outline">More Available</Badge> : null}
          </>
        }
        title="Workspaces"
      />

      {error !== "" ? <ErrorBanner message={error} /> : null}

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>Workspace catalog</CardTitle>
            <CardDescription>
              Instantiated workspaces and the workflows they are bound to.
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
              description="Workspace instances will appear here after they are created."
              title="No workspaces"
            />
          ) : (
            <DashboardTable className="table-fixed">
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[26%]">Workspace</TableHead>
                  <TableHead>Workflow</TableHead>
                  <TableHead className="w-28 text-right">Parameters</TableHead>
                  <TableHead className="w-40">Created</TableHead>
                  <TableHead className="w-40 text-right">Updated</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((workspace) => (
                  <TableRow
                    className="cursor-pointer hover:bg-muted/40"
                    key={workspace.id}
                    onClick={() => openWorkspace(workspace.id)}
                    onKeyDown={(event) => handleRowKeyDown(event, workspace.id)}
                    role="link"
                    tabIndex={0}
                  >
                    <TableCell className="min-w-0">
                      <div className="min-w-0">
                        <div
                          className="truncate font-medium"
                          title={workspace.name}
                        >
                          {workspace.name}
                        </div>
                        <div className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
                          <span
                            className="truncate font-mono"
                            title={workspace.id}
                          >
                            {workspace.id}
                          </span>
                          <button
                            aria-label={`Copy workspace ID ${workspace.id}`}
                            className="shrink-0 rounded-sm hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            onClick={(event) =>
                              void copyWorkspaceID(event, workspace.id)
                            }
                            title="Copy workspace ID"
                            type="button"
                          >
                            {copiedID === workspace.id ? (
                              <Check className="size-3 shrink-0 text-emerald-600" />
                            ) : (
                              <Copy className="size-3 shrink-0" />
                            )}
                          </button>
                        </div>
                      </div>
                    </TableCell>
                    <TableCell
                      className="truncate"
                      title={workspace.workflow_id}
                    >
                      {workspace.workflow_id}
                    </TableCell>
                    <TableCell className="text-right">
                      {Object.keys(workspace.parameters ?? {}).length}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {formatDate(workspace.created_at)}
                    </TableCell>
                    <TableCell className="text-right text-sm text-muted-foreground">
                      {formatDate(workspace.updated_at)}
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

function isInteractiveTarget(target: EventTarget): boolean {
  return (
    target instanceof Element &&
    target.closest("a,button,input,select,textarea") !== null
  );
}
