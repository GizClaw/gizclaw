import dayjs from "dayjs";
import { DashboardActionButton } from "@/dashboard";
import relativeTime from "dayjs/plugin/relativeTime";
import type { KeyboardEvent, MouseEvent } from "react";
import { Check, Copy, RefreshCw, Search } from "lucide-react";
import { useCallback, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

import type { DeviceInfo, Runtime } from "@gizclaw/gizclaw/admin";
import { EmptyState } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { StatusBadge } from "@/dashboard";
import { usePeersPage } from "../../hooks/usePeersPage";
import { formatDate } from "../../lib/format";

dayjs.extend(relativeTime);

interface NamedRegistration {
  name?: string | null;
  device?: {
    name?: string | null;
  } | null;
}

export function PeersListPage(): JSX.Element {
  const navigate = useNavigate();
  const [copiedPublicKey, setCopiedPublicKey] = useState<string | null>(null);
  const {
    dashboard,
    peerList,
    peerPageNumber,
    filter,
    filteredPeers,
    nextPage,
    prevPage,
    refreshDashboard,
    setFilter,
  } = usePeersPage();

  const handleCopyPublicKey = useCallback(async (event: MouseEvent<HTMLButtonElement>, publicKey: string) => {
    event.stopPropagation();
    try {
      await navigator.clipboard.writeText(publicKey);
      setCopiedPublicKey(publicKey);
      window.setTimeout(() => setCopiedPublicKey((current) => (current === publicKey ? null : current)), 1200);
    } catch {
      setCopiedPublicKey(null);
    }
  }, []);

  const openPeer = useCallback((publicKey: string) => {
    navigate(`/peers/${encodeURIComponent(publicKey)}`);
  }, [navigate]);

  const handlePeerRowKeyDown = useCallback((event: KeyboardEvent<HTMLTableRowElement>, publicKey: string) => {
    if (isInteractiveTarget(event.target)) {
      return;
    }
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    openPeer(publicKey);
  }, [openPeer]);

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <DashboardActionButton onClick={() => void refreshDashboard()}>
            <span className="inline-flex items-center gap-2 whitespace-nowrap">
              <RefreshCw className="size-4" />
              Refresh
            </span>
          </DashboardActionButton>
        }
        items={[{ href: "/overview", label: "Overview" }, { label: "Peers" }]}
      />

      <PageSummaryCard
        description="Browse paged inventory, filter the current page, and open a peer into its own detail route."
        eyebrow="Inventory"
        meta={
          <>
            <Badge variant="outline">Page {peerPageNumber}</Badge>
            <Badge variant="secondary">{dashboard.peers.length} loaded</Badge>
            {peerList.hasNext ? <Badge variant="outline">More Available</Badge> : null}
          </>
        }
        title="Peers"
      />

      {dashboard.error !== "" ? (
        <div className="rounded-lg border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">{dashboard.error}</div>
      ) : null}

      <Card>
        <CardContent className="p-6">
          <div className="rounded-md border">
            <div className="flex flex-col gap-3 border-b px-4 py-4 lg:flex-row lg:items-start lg:justify-between">
              <div className="space-y-1">
                <CardTitle>Peer Inventory</CardTitle>
                <CardDescription>Browse paged peer results and open a row to inspect details.</CardDescription>
              </div>
            </div>

            <div className="flex flex-col gap-3 border-b px-4 py-4 lg:flex-row lg:items-center lg:justify-between">
              <div className="relative lg:max-w-sm lg:flex-1">
                <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  className="h-8 pl-9"
                  onChange={(event) => setFilter(event.target.value)}
                  placeholder="Filter current page by key, role, or status"
                  value={filter}
                />
              </div>
              <div className="flex gap-2">
                <DashboardActionButton
                  disabled={dashboard.loading || peerList.history.length === 0}
                  onClick={prevPage}
                  type="button"
                >
                  Previous
                </DashboardActionButton>
                <DashboardActionButton
                  disabled={dashboard.loading || !peerList.hasNext || peerList.nextCursor === null}
                  onClick={nextPage}
                  type="button"
                >
                  Next
                </DashboardActionButton>
              </div>
            </div>

            {dashboard.loading ? (
              <div className="space-y-3 p-4">
                {Array.from({ length: 6 }).map((_, index) => (
                  <Skeleton className="h-16 w-full" key={index} />
                ))}
              </div>
            ) : filteredPeers.length === 0 ? (
              <div className="p-4">
                <EmptyState
                  description={filter.trim() === "" ? "Peers will appear here as soon as they are registered." : "No peers on this page match the current filter."}
                  title="No matching peers"
                />
              </div>
            ) : (
              <Table className="table-fixed">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-64">Peer</TableHead>
                    <TableHead className="w-56">Registration</TableHead>
                    <TableHead className="w-56">Runtime</TableHead>
                    <TableHead className="w-48">Endpoint</TableHead>
                    <TableHead className="text-right">Updated</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredPeers.map((peer) => {
                    const name = peerDisplayName(peer, dashboard.infos[peer.public_key]);
                    const copied = copiedPublicKey === peer.public_key;
                    const runtime = dashboard.runtimes[peer.public_key];

                    return (
                      <TableRow
                        className="cursor-pointer hover:bg-muted/40"
                        key={peer.public_key}
                        onClick={() => openPeer(peer.public_key)}
                        onKeyDown={(event) => handlePeerRowKeyDown(event, peer.public_key)}
                        role="link"
                        tabIndex={0}
                      >
                        <TableCell className="w-64 max-w-64">
                          <div className="flex min-w-0 items-center gap-1.5">
                            <div className="min-w-0 flex-1">
                              {name !== "" ? (
                                <div className="block truncate font-medium" title={name}>
                                  {name}
                                </div>
                              ) : null}
                              <button
                                className="block w-full truncate text-left font-mono text-xs text-muted-foreground hover:text-foreground"
                                onClick={(event) => void handleCopyPublicKey(event, peer.public_key)}
                                title="Copy public key"
                                type="button"
                              >
                                {peer.public_key}
                              </button>
                            </div>
                            <button
                              aria-label={`Copy public key ${peer.public_key}`}
                              className="shrink-0 rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                              onClick={(event) => void handleCopyPublicKey(event, peer.public_key)}
                              title="Copy public key"
                              type="button"
                            >
                              {copied ? <Check className="size-3 shrink-0 text-emerald-600" /> : <Copy className="size-3 shrink-0" />}
                            </button>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex justify-start gap-1.5">
                            <Badge title={peer.auto_registered ? "Auto registered" : "Manual registration"} variant="secondary">
                              {peer.auto_registered ? "A" : "M"}
                            </Badge>
                            <StatusBadge status={peer.status} />
                            <Badge title={`Role: ${peer.role}`} variant="outline">
                              {peer.role}
                            </Badge>
                          </div>
                        </TableCell>
                        <TableCell>
                          <PeerRuntime runtime={runtime} />
                        </TableCell>
                        <TableCell>
                          <PeerEndpoint runtime={runtime} />
                        </TableCell>
                        <TableCell className="text-right text-sm text-muted-foreground">{formatDate(peer.updated_at)}</TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            )}

            <div className="flex items-center justify-between border-t px-4 py-4 text-sm text-muted-foreground">
              <span>
                Showing {filteredPeers.length} of {dashboard.peers.length} peers on page {peerPageNumber}
              </span>
              <span>{peerList.hasNext ? "Next page available" : "End of results"}</span>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function peerDisplayName(peer: NamedRegistration, info: DeviceInfo | null | undefined): string {
  return peer.name?.trim() || peer.device?.name?.trim() || info?.name?.trim() || "";
}

function PeerRuntime({ runtime }: { runtime: Runtime | null | undefined }): JSX.Element {
  if (runtime === undefined || runtime === null) {
    return <span className="text-sm text-muted-foreground">Unavailable</span>;
  }
  const lastSeen = formatRuntimeLastSeen(runtime.last_seen_at);

  return (
    <div className="max-w-56 space-y-1">
      <div className="flex items-center gap-2">
        <Badge className={runtime.online ? undefined : "border-transparent bg-muted text-muted-foreground hover:bg-muted"} variant={runtime.online ? "success" : "outline"}>
          {runtime.online ? "Online" : "Offline"}
        </Badge>
        <span className="whitespace-nowrap text-xs text-muted-foreground" title={lastSeen.title}>
          {lastSeen.label}
        </span>
      </div>
    </div>
  );
}

function PeerEndpoint({ runtime }: { runtime: Runtime | null | undefined }): JSX.Element {
  if (runtime === undefined || runtime === null) {
    return <span className="text-sm text-muted-foreground">Unavailable</span>;
  }
  const endpoint = runtime.last_addr?.trim() ?? "";
  if (endpoint === "") {
    return <span className="text-sm text-muted-foreground">No endpoint</span>;
  }
  return (
    <div className="max-w-48 truncate font-mono text-xs text-muted-foreground" title={endpoint}>
      {endpoint}
    </div>
  );
}

function formatRuntimeLastSeen(value: string | undefined): { label: string; title: string } {
  if (value === undefined || value === "" || value.startsWith("0001-01-01")) {
    return { label: "Never seen", title: "Never seen" };
  }
  const seenAt = dayjs(value);
  if (!seenAt.isValid()) {
    return { label: value, title: value };
  }
  return { label: `Seen ${seenAt.fromNow()}`, title: seenAt.toDate().toLocaleString() };
}

function isInteractiveTarget(target: EventTarget): boolean {
  return target instanceof Element && target.closest("a,button,input,select,textarea") !== null;
}
