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
import { listMiniMaxTenants, type MiniMaxTenant } from "@gizclaw/gizclaw/admin";

import { ErrorBanner } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { useDashboardCursorPage as useCursorListPage } from "@/dashboard";
import { formatDate, formatValue } from "../../lib/format";

export function MiniMaxTenantsListPage(): JSX.Element {
  const navigate = useNavigate();
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
  } = useCursorListPage<MiniMaxTenant>(async (query) => {
    const result = await expectData(listMiniMaxTenants({ query }));
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });

  const openTenant = (name: string): void => {
    navigate(`/providers/minimax-tenants/${encodeURIComponent(name)}`);
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
    openTenant(name);
  };

  const copyTenantName = async (
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
              <Link to="/resources?kind=MiniMaxTenant">
                <Plus className="size-4" />
                New MiniMax Tenant
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
          { label: "MiniMax Tenants" },
        ]}
      />

      <PageSummaryCard
        description="Multi-tenant MiniMax configurations bound to stored credentials and used for voice synchronization."
        eyebrow="Providers"
        meta={
          <>
            <Badge variant="outline">Page {pageNumber}</Badge>
            <Badge variant="secondary">{items.length} loaded</Badge>
            {hasNext ? <Badge variant="outline">More Available</Badge> : null}
          </>
        }
        title="MiniMax Tenants"
      />

      {error !== "" ? <ErrorBanner message={error} /> : null}

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>Tenant catalog</CardTitle>
            <CardDescription>
              Each tenant maps an app and group pair to a reusable credential.
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
              description="MiniMax tenant records will appear here after they are created."
              title="No MiniMax tenants"
            />
          ) : (
            <DashboardTable className="table-fixed">
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[20%]">Tenant ID</TableHead>
                  <TableHead className="w-36">App ID</TableHead>
                  <TableHead className="w-36">Group ID</TableHead>
                  <TableHead className="w-40">Credential</TableHead>
                  <TableHead>Base URL</TableHead>
                  <TableHead className="w-40">Last Sync</TableHead>
                  <TableHead className="w-40 text-right">Updated</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((tenant) => (
                  <TableRow
                    className="cursor-pointer hover:bg-muted/40"
                    key={tenant.name}
                    onClick={() => openTenant(tenant.name)}
                    onKeyDown={(event) => handleRowKeyDown(event, tenant.name)}
                    role="link"
                    tabIndex={0}
                  >
                    <TableCell className="min-w-0">
                      <div className="flex min-w-0 items-center gap-1.5">
                        <button
                          className="min-w-0 truncate rounded-sm text-left font-medium underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          onClick={(event) => {
                            event.stopPropagation();
                            openTenant(tenant.name);
                          }}
                          title={tenant.name}
                          type="button"
                        >
                          {tenant.name}
                        </button>
                        <button
                          aria-label={`Copy tenant name ${tenant.name}`}
                          className="shrink-0 rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          onClick={(event) =>
                            void copyTenantName(event, tenant.name)
                          }
                          title="Copy tenant name"
                          type="button"
                        >
                          {copiedName === tenant.name ? (
                            <Check className="size-3 shrink-0 text-emerald-600" />
                          ) : (
                            <Copy className="size-3 shrink-0" />
                          )}
                        </button>
                      </div>
                    </TableCell>
                    <TableCell
                      className="truncate font-mono text-xs"
                      title={tenant.app_id}
                    >
                      {tenant.app_id}
                    </TableCell>
                    <TableCell
                      className="truncate font-mono text-xs"
                      title={tenant.group_id}
                    >
                      {tenant.group_id}
                    </TableCell>
                    <TableCell
                      className="truncate"
                      title={tenant.credential_name}
                    >
                      {tenant.credential_name}
                    </TableCell>
                    <TableCell
                      className="truncate text-sm text-muted-foreground"
                      title={formatValue(tenant.base_url)}
                    >
                      {formatValue(tenant.base_url)}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {formatDate(tenant.last_synced_at)}
                    </TableCell>
                    <TableCell className="text-right text-sm text-muted-foreground">
                      {formatDate(tenant.updated_at)}
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
