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
import { listWorkspaces, type Workspace } from "@gizclaw/adminservice";

import { ErrorBanner } from "../../components/banners";
import { EmptyState } from "../../components/empty-state";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { useCursorListPage } from "../../hooks/useCursorListPage";
import { formatDate } from "../../lib/format";

export function WorkspacesListPage(): JSX.Element {
  const navigate = useNavigate();
  const [copiedName, setCopiedName] = useState("");
  const { error, hasNext, items, loading, nextPage, pageNumber, prevPage, refresh } = useCursorListPage<Workspace>(async (query) => {
    const result = await expectData(listWorkspaces({ query }));
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });

  const openWorkspace = (name: string): void => {
    navigate(`/resources?kind=Workspace&name=${encodeURIComponent(name)}`);
  };

  const handleRowKeyDown = (event: KeyboardEvent<HTMLTableRowElement>, name: string): void => {
    if (isInteractiveTarget(event.target)) {
      return;
    }
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    openWorkspace(name);
  };

  const copyWorkspaceName = async (event: MouseEvent<HTMLButtonElement>, name: string): Promise<void> => {
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
            <Button asChild className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" variant="outline">
              <Link to="/resources?kind=Workspace">
                <Plus className="size-4" />
                New Workspace
              </Link>
            </Button>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" onClick={() => void refresh()} variant="outline">
              <RefreshCw className="size-4" />
              Refresh
            </Button>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { label: "Workspaces" }]}
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
            <CardDescription>Instantiated workspaces and the workflows they are bound to.</CardDescription>
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
            <EmptyState description="Workspace instances will appear here after they are created." title="No workspaces" />
          ) : (
            <div className="rounded-md border">
              <Table className="table-fixed">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[26%]">Workspace ID</TableHead>
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
                      key={workspace.name}
                      onClick={() => openWorkspace(workspace.name)}
                      onKeyDown={(event) => handleRowKeyDown(event, workspace.name)}
                      role="link"
                      tabIndex={0}
                    >
                      <TableCell className="min-w-0">
                        <div className="flex min-w-0 items-center gap-1.5">
                          <span className="min-w-0 truncate font-medium" title={workspace.name}>
                            {workspace.name}
                          </span>
                          <button
                            aria-label={`Copy workspace name ${workspace.name}`}
                            className="shrink-0 rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            onClick={(event) => void copyWorkspaceName(event, workspace.name)}
                            title="Copy workspace name"
                            type="button"
                          >
                            {copiedName === workspace.name ? <Check className="size-3 shrink-0 text-emerald-600" /> : <Copy className="size-3 shrink-0" />}
                          </button>
                        </div>
                      </TableCell>
                      <TableCell className="truncate" title={workspace.workflow_name}>{workspace.workflow_name}</TableCell>
                      <TableCell className="text-right">{Object.keys(workspace.parameters ?? {}).length}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">{formatDate(workspace.created_at)}</TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground">{formatDate(workspace.updated_at)}</TableCell>
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

function isInteractiveTarget(target: EventTarget): boolean {
  return target instanceof Element && target.closest("a,button,input,select,textarea") !== null;
}
