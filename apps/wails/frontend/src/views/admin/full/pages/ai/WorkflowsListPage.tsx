import { Check, Copy, Plus, RefreshCw } from "lucide-react";
import { DashboardActionButton } from "@/dashboard";
import { DashboardPager } from "@/dashboard";
import { DashboardTable } from "@/dashboard";
import type { KeyboardEvent, MouseEvent } from "react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { expectData } from "@/dashboard";
import { workflowLocale } from "@/lib/gizclaw/workflow_i18n";
import { listWorkflows, type Workflow, type WorkflowI18nCatalog } from "@gizclaw/gizclaw/admin";

import { ErrorBanner } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { useDashboardCursorPage as useCursorListPage } from "@/dashboard";

export function WorkflowsListPage(): JSX.Element {
  const navigate = useNavigate();
  const [copiedName, setCopiedName] = useState("");
  const { error, hasNext, items, loading, nextPage, pageNumber, prevPage, refresh } = useCursorListPage<Workflow>(
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

  const copyWorkflowName = async (event: MouseEvent<HTMLButtonElement>, name: string): Promise<void> => {
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
              <Link to="/resources?kind=Workflow">
                <Plus className="size-4" />
                New Workflow
              </Link>
            </DashboardActionButton>
            <DashboardActionButton onClick={() => void refresh()}>
              <RefreshCw className="size-4" />
              Refresh
            </DashboardActionButton>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { label: "Workflows" }]}
      />

      <PageSummaryCard
        description="Workflows that workspaces load when running agents."
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
            <CardDescription>Workflows grouped by driver and localized catalog.</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex justify-end">
            <DashboardPager canNext={hasNext} canPrevious={pageNumber > 1} loading={loading} onNext={nextPage} onPrevious={prevPage} onRefresh={() => void refresh()} pageIndex={pageNumber} />
          </div>

          {loading ? (
            <div className="space-y-3">
              {Array.from({ length: 6 }).map((_, index) => (
                <Skeleton className="h-14 w-full" key={index} />
              ))}
            </div>
          ) : items.length === 0 ? (
            <EmptyState description="Workflows will appear here after they are created." title="No workflows" />
          ) : (
            <DashboardTable className="table-fixed">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[28%]">Workflow ID</TableHead>
                    <TableHead className="w-36">Driver</TableHead>
                    <TableHead className="w-40">Spec</TableHead>
                    <TableHead>Description</TableHead>
                    <TableHead className="w-32 text-right">Updated</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((workflow) => (
                    <TableRow
                      className="cursor-pointer hover:bg-muted/40"
                      key={workflow.name}
                      onClick={() => openWorkflow(workflow.name)}
                      onKeyDown={(event) => handleRowKeyDown(event, workflow.name)}
                      role="link"
                      tabIndex={0}
                    >
                      <TableCell className="min-w-0">
                        <div className="flex min-w-0 items-center gap-1.5">
                          <button
                            className="min-w-0 truncate rounded-sm text-left font-medium underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            onClick={(event) => {
                              event.stopPropagation();
                              openWorkflow(workflow.name);
                            }}
                            title={workflow.name}
                            type="button"
                          >
                            {workflow.name}
                          </button>
                          <button
                            aria-label={`Copy workflow name ${workflow.name}`}
                            className="shrink-0 rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            onClick={(event) => void copyWorkflowName(event, workflow.name)}
                            title="Copy workflow name"
                            type="button"
                          >
                            {copiedName === workflow.name ? <Check className="size-3 shrink-0 text-emerald-600" /> : <Copy className="size-3 shrink-0" />}
                          </button>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline">{workflow.spec.driver}</Badge>
                      </TableCell>
                      <TableCell>{workflowSpecLabel(workflow)}</TableCell>
                      <TableCell className="truncate text-sm text-muted-foreground" title={workflowDescription(workflow) || "—"}>
                        {workflowDescription(workflow) || "—"}
                      </TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground">—</TableCell>
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
  return target instanceof Element && target.closest("a,button,input,select,textarea") !== null;
}

function workflowDescription(workflow: Workflow): string {
  const catalog = workflowCatalog(workflow, navigator.languages);
  return catalog?.description?.trim() ?? "";
}

function workflowCatalog(workflow: Workflow, localeTags: readonly string[]): WorkflowI18nCatalog | undefined {
  const i18n = workflow.i18n;
  if (!i18n) {
    return undefined;
  }

  const uiLocale = workflowLocale(localeTags[0] ?? "");
  const candidates: Array<"en" | "zh-CN"> = uiLocale === "" ? [] : [uiLocale];
  candidates.push(i18n.default_locale);
  for (const locale of candidates) {
    const catalog = locale === "zh-CN" ? i18n["zh-CN"] : i18n.en;
    if (typeof catalog === "object") {
      return catalog;
    }
  }
  return undefined;
}

function workflowSpecLabel(workflow: Workflow): string {
  if (workflow.spec.ast_translate !== undefined) {
    return "ast_translate";
  }
  if (workflow.spec.doubao_realtime !== undefined) {
    return "doubao_realtime";
  }
  if (workflow.spec.flowcraft !== undefined) {
    return "flowcraft";
  }
  if (workflow.spec.chatroom !== undefined) {
    return "chatroom";
  }
  return "—";
}
