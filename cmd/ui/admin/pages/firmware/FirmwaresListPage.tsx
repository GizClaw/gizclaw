import type { KeyboardEvent } from "react";
import { Plus, RefreshCw } from "lucide-react";
import { Link, useNavigate } from "react-router-dom";

import { listFirmwares, type Firmware } from "@gizclaw/adminservice";
import { expectData } from "../../components/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ErrorBanner } from "../../components/banners";
import { EmptyState } from "../../components/empty-state";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useCursorListPage } from "../../hooks/useCursorListPage";
import { formatDate } from "../../lib/format";

export function FirmwaresListPage(): JSX.Element {
  const navigate = useNavigate();
  const { error, hasNext, items, loading, nextPage, pageNumber, prevPage, refresh } = useCursorListPage<Firmware>(async (query) => {
    const result = await expectData(listFirmwares({ query }));
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });

  const openFirmware = (name: string): void => {
    navigate(`/firmwares/${encodeURIComponent(name)}`);
  };

  const handleRowKeyDown = (event: KeyboardEvent<HTMLTableRowElement>, name: string): void => {
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    openFirmware(name);
  };

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" variant="outline">
              <Link to="/firmwares/new">
                <Plus className="size-4" />
                Create
              </Link>
            </Button>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" onClick={() => void refresh()} variant="outline">
              <RefreshCw className="size-4" />
              Refresh
            </Button>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { label: "Firmwares" }]}
      />

      <PageSummaryCard
        description="Release-line JSON documents with rollback, stable, beta, develop, and pending slots."
        eyebrow="Devices"
        meta={
          <>
            <Badge variant="outline">Page {pageNumber}</Badge>
            <Badge variant="secondary">{items.length} loaded</Badge>
            {hasNext ? <Badge variant="outline">More Available</Badge> : null}
          </>
        }
        title="Firmwares"
      />

      {error !== "" ? <ErrorBanner message={error} /> : null}

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>Firmware catalog</CardTitle>
            <CardDescription>Stored firmware release lines and current slot versions.</CardDescription>
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
            <EmptyState description="Firmware release lines will appear here after they are created." title="No firmwares" />
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Stable</TableHead>
                    <TableHead>Beta</TableHead>
                    <TableHead>Develop</TableHead>
                    <TableHead>Pending</TableHead>
                    <TableHead className="text-right">Updated</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((firmware) => (
                    <TableRow
                      className="cursor-pointer hover:bg-muted/40"
                      key={firmware.name}
                      onClick={() => openFirmware(firmware.name)}
                      onKeyDown={(event) => handleRowKeyDown(event, firmware.name)}
                      role="link"
                      tabIndex={0}
                    >
                      <TableCell className="font-medium">{firmware.name}</TableCell>
                      <TableCell>{slotLabel(firmware.slots.stable)}</TableCell>
                      <TableCell>{slotLabel(firmware.slots.beta)}</TableCell>
                      <TableCell>{slotLabel(firmware.slots.develop)}</TableCell>
                      <TableCell>{slotLabel(firmware.slots.pending)}</TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground">{formatDate(firmware.updated_at)}</TableCell>
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

function slotLabel(slot: Firmware["slots"]["stable"]): JSX.Element {
  const version = slot.version?.trim();
  const count = slot.artifacts?.length ?? 0;
  if (!version && count === 0) {
    return <span className="text-muted-foreground">-</span>;
  }
  return (
    <div className="flex items-center gap-2">
      <span className="font-mono text-xs">{version || "artifact-only"}</span>
      {count > 0 ? <Badge variant="outline">{count}</Badge> : null}
    </div>
  );
}
