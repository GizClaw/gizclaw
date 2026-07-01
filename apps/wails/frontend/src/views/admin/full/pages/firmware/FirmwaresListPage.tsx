import type { KeyboardEvent, MouseEvent } from "react";
import { useState } from "react";
import { Check, Copy, Plus, RefreshCw } from "lucide-react";
import { Link, useNavigate } from "react-router-dom";

import { listFirmwares, type Firmware } from "@gizclaw/gizclaw/admin";
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
  const [copiedName, setCopiedName] = useState("");
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
    if (isInteractiveTarget(event.target)) {
      return;
    }
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    openFirmware(name);
  };

  const copyFirmwareName = async (event: MouseEvent<HTMLButtonElement>, name: string): Promise<void> => {
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
        description="Release-line JSON documents with develop, beta, stable, and pending slots."
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
              <Table className="table-fixed">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[26%]">Firmware ID</TableHead>
                    <TableHead>Stable</TableHead>
                    <TableHead>Beta</TableHead>
                    <TableHead>Develop</TableHead>
                    <TableHead>Pending</TableHead>
                    <TableHead className="w-40 text-right">Updated</TableHead>
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
                      <TableCell className="min-w-0">
                        <div className="flex min-w-0 items-center gap-1.5">
                          <button
                            className="min-w-0 truncate rounded-sm text-left font-medium underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            onClick={(event) => {
                              event.stopPropagation();
                              openFirmware(firmware.name);
                            }}
                            title={firmware.name}
                            type="button"
                          >
                            {firmware.name}
                          </button>
                          <button
                            aria-label={`Copy firmware name ${firmware.name}`}
                            className="shrink-0 rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            onClick={(event) => void copyFirmwareName(event, firmware.name)}
                            title="Copy firmware name"
                            type="button"
                          >
                            {copiedName === firmware.name ? <Check className="size-3 shrink-0 text-emerald-600" /> : <Copy className="size-3 shrink-0" />}
                          </button>
                        </div>
                      </TableCell>
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
  const description = slot.description?.trim();
  const hasArtifact = slot.artifact != null && slot.artifact.tar_path.trim() !== "";
  if (!description && !hasArtifact) {
    return <span className="text-muted-foreground">-</span>;
  }
  return (
    <div className="flex items-center gap-2">
      <span className="text-xs">{description || "artifact-only"}</span>
      {hasArtifact ? <Badge variant="outline">artifact</Badge> : null}
    </div>
  );
}

function isInteractiveTarget(target: EventTarget): boolean {
  return target instanceof Element && target.closest("a,button,input,select,textarea") !== null;
}
