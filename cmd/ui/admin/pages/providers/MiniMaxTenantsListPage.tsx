import { Plus, RefreshCw } from "lucide-react";
import type { KeyboardEvent, MouseEvent } from "react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { expectData, toMessage } from "../../components/api";
import { listMiniMaxTenants, syncMiniMaxTenantVoices, type MiniMaxTenant } from "@gizclaw/adminservice";

import { ErrorBanner } from "../../components/banners";
import { EmptyState } from "../../components/empty-state";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { useCursorListPage } from "../../hooks/useCursorListPage";
import { formatDate, formatValue } from "../../lib/format";

export function MiniMaxTenantsListPage(): JSX.Element {
  const navigate = useNavigate();
  const [syncing, setSyncing] = useState<Record<string, boolean>>({});
  const [syncError, setSyncError] = useState("");
  const { error, hasNext, items, loading, nextPage, pageNumber, prevPage, refresh } = useCursorListPage<MiniMaxTenant>(async (query) => {
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

  const handleRowKeyDown = (event: KeyboardEvent<HTMLTableRowElement>, name: string): void => {
    if (isInteractiveTarget(event.target)) {
      return;
    }
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    openTenant(name);
  };

  const syncTenant = async (event: MouseEvent<HTMLButtonElement>, name: string): Promise<void> => {
    event.stopPropagation();
    setSyncError("");
    setSyncing((current) => ({ ...current, [name]: true }));
    try {
      await expectData(syncMiniMaxTenantVoices({ path: { name } }));
      await refresh();
    } catch (err) {
      setSyncError(toMessage(err));
    } finally {
      setSyncing((current) => ({ ...current, [name]: false }));
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" variant="outline">
              <Link to="/resources?kind=MiniMaxTenant">
                <Plus className="size-4" />
                New MiniMax Tenant
              </Link>
            </Button>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" onClick={() => void refresh()} variant="outline">
              <RefreshCw className="size-4" />
              Refresh
            </Button>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { label: "MiniMax Tenants" }]}
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
      {syncError !== "" ? <ErrorBanner message={syncError} /> : null}

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>Tenant catalog</CardTitle>
            <CardDescription>Each tenant maps an app and group pair to a reusable credential.</CardDescription>
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
            <EmptyState description="MiniMax tenant records will appear here after they are created." title="No MiniMax tenants" />
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>App ID</TableHead>
                    <TableHead>Group ID</TableHead>
                    <TableHead>Credential</TableHead>
                    <TableHead>Base URL</TableHead>
                    <TableHead>Last Sync</TableHead>
                    <TableHead className="text-right">Updated</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
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
                      <TableCell className="font-medium">{tenant.name}</TableCell>
                      <TableCell className="font-mono text-xs">{tenant.app_id}</TableCell>
                      <TableCell className="font-mono text-xs">{tenant.group_id}</TableCell>
                      <TableCell>{tenant.credential_name}</TableCell>
                      <TableCell className="max-w-[18rem] truncate text-sm text-muted-foreground">{formatValue(tenant.base_url)}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">{formatDate(tenant.last_synced_at)}</TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground">{formatDate(tenant.updated_at)}</TableCell>
                      <TableCell className="text-right">
                        <Button
                          className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm"
                          disabled={syncing[tenant.name] === true}
                          onClick={(event) => void syncTenant(event, tenant.name)}
                          type="button"
                          variant="outline"
                        >
                          <RefreshCw className={`size-4 ${syncing[tenant.name] === true ? "animate-spin" : ""}`} />
                          Sync
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
