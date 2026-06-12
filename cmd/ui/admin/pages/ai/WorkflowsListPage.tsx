import { FileJson, Plus, RefreshCw } from "lucide-react";
import type { KeyboardEvent } from "react";
import { Link, useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { expectData } from "../../components/api";
import { listWorkflows, type FlowcraftWorkflow } from "@gizclaw/adminservice";

import { ErrorBanner } from "../../components/banners";
import { EmptyState } from "../../components/empty-state";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { useCursorListPage } from "../../hooks/useCursorListPage";

export function WorkflowsListPage(): JSX.Element {
  const navigate = useNavigate();
  const { error, hasNext, items, loading, nextPage, pageNumber, prevPage, refresh } = useCursorListPage<FlowcraftWorkflow>(
    async (query) => {
      const result = await expectData(listWorkflows({ query }));
      return {
        hasNext: result.has_next,
        items: result.items ?? [],
        nextCursor: result.next_cursor ?? null,
      };
    },
  );

  const openWorkflow = (name: string): void => {
    navigate(`/resources?kind=Workflow&name=${encodeURIComponent(name)}`);
  };

  const handleRowKeyDown = (event: KeyboardEvent<HTMLTableRowElement>, name: string): void => {
    if (isInteractiveTarget(event.target)) {
      return;
    }
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    openWorkflow(name);
  };

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" variant="outline">
              <Link to="/resources?kind=Workflow">
                <Plus className="size-4" />
                New Workflow
              </Link>
            </Button>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" onClick={() => void refresh()} variant="outline">
              <RefreshCw className="size-4" />
              Refresh
            </Button>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { label: "Workflows" }]}
      />

      <PageSummaryCard
        description="Declarative workflow documents that workspaces load when running agents."
        eyebrow="AI"
        meta={
          <>
            <Badge variant="outline">Page {pageNumber}</Badge>
            <Badge variant="secondary">{items.length} loaded</Badge>
            {hasNext ? <Badge variant="outline">More Available</Badge> : null}
          </>
        }
        title="Workflows"
      />

      {error !== "" ? <ErrorBanner message={error} /> : null}

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>Workflow catalog</CardTitle>
            <CardDescription>Workflow documents grouped by top-level kind and metadata.</CardDescription>
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
            <EmptyState description="Workflow documents will appear here after they are created." title="No workflows" />
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Kind</TableHead>
                    <TableHead>API Version</TableHead>
                    <TableHead>Description</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((workflow) => (
                    <TableRow
                      className="cursor-pointer hover:bg-muted/40"
                      key={workflow.metadata.name}
                      onClick={() => openWorkflow(workflow.metadata.name)}
                      onKeyDown={(event) => handleRowKeyDown(event, workflow.metadata.name)}
                      role="link"
                      tabIndex={0}
                    >
                      <TableCell className="font-medium">{workflow.metadata.name}</TableCell>
                      <TableCell>
                        <Badge variant="outline">{workflow.kind}</Badge>
                      </TableCell>
                      <TableCell>{workflow.apiVersion}</TableCell>
                      <TableCell className="max-w-[28rem] text-sm text-muted-foreground">{workflow.metadata.description?.trim() || "—"}</TableCell>
                      <TableCell className="text-right">
                        <Button asChild className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" onClick={(event) => event.stopPropagation()} variant="outline">
                          <Link to={`/resources?kind=Workflow&name=${encodeURIComponent(workflow.metadata.name)}`}>
                            <FileJson className="size-4" />
                            Resource
                          </Link>
                        </Button>
                      </TableCell>
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
