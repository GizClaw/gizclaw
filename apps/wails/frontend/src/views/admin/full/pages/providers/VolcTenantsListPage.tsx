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
import { listVolcTenants, type VolcTenant } from "@gizclaw/gizclaw/admin";

import { ErrorBanner } from "../../components/banners";
import { EmptyState } from "../../components/empty-state";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { useCursorListPage } from "../../hooks/useCursorListPage";
import { formatDate, formatValue } from "../../lib/format";

export function VolcTenantsListPage(): JSX.Element {
  const navigate = useNavigate();
  const [copiedName, setCopiedName] = useState("");
  const { error, hasNext, items, loading, nextPage, pageNumber, prevPage, refresh } = useCursorListPage<VolcTenant>(async (query) => {
    const result = await expectData(listVolcTenants({ query }));
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });

  const openTenant = (name: string): void => {
    navigate(`/providers/volc-tenants/${encodeURIComponent(name)}`);
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

  const copyTenantName = async (event: MouseEvent<HTMLButtonElement>, name: string): Promise<void> => {
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
              <Link to="/resources?kind=VolcTenant">
                <Plus className="size-4" />
                New Volcengine Tenant
              </Link>
            </Button>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" onClick={() => void refresh()} variant="outline">
              <RefreshCw className="size-4" />
              Refresh
            </Button>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { label: "Volcengine Tenants" }]}
      />

      <PageSummaryCard
        description="Volcengine speech configurations bound to stored credentials and used for Doubao voice synchronization."
        eyebrow="Providers"
        meta={
          <>
            <Badge variant="outline">Page {pageNumber}</Badge>
            <Badge variant="secondary">{items.length} loaded</Badge>
            {hasNext ? <Badge variant="outline">More Available</Badge> : null}
          </>
        }
        title="Volcengine Tenants"
      />

      {error !== "" ? <ErrorBanner message={error} /> : null}

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>Tenant catalog</CardTitle>
            <CardDescription>Each tenant maps a Volcengine speech AppID and credential to purchased voice synchronization.</CardDescription>
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
            <EmptyState description="Volcengine tenant records will appear here after they are created." title="No Volcengine tenants" />
          ) : (
            <div className="rounded-md border">
              <Table className="table-fixed">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[20%]">Tenant ID</TableHead>
                    <TableHead className="w-40">Credential</TableHead>
                    <TableHead className="w-28">Region</TableHead>
                    <TableHead>Endpoint</TableHead>
                    <TableHead>Resource IDs</TableHead>
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
                            onClick={(event) => void copyTenantName(event, tenant.name)}
                            title="Copy tenant name"
                            type="button"
                          >
                            {copiedName === tenant.name ? <Check className="size-3 shrink-0 text-emerald-600" /> : <Copy className="size-3 shrink-0" />}
                          </button>
                        </div>
                      </TableCell>
                      <TableCell className="truncate" title={tenant.credential_name}>{tenant.credential_name}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">{formatValue(tenant.region)}</TableCell>
                      <TableCell className="truncate text-sm text-muted-foreground" title={formatValue(tenant.endpoint)}>{formatValue(tenant.endpoint)}</TableCell>
                      <TableCell className="truncate font-mono text-xs" title={tenant.resource_ids?.join(", ") ?? "public only"}>{tenant.resource_ids?.join(", ") ?? "public only"}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">{formatDate(tenant.last_synced_at)}</TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground">{formatDate(tenant.updated_at)}</TableCell>
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
