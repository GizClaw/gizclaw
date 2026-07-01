import type { ComponentType } from "react";
import type { KeyboardEvent } from "react";
import { AudioLines, Boxes, ChevronRight, Cpu, FolderKanban, KeyRound, Mic2, Server, ShieldCheck, Workflow } from "lucide-react";
import { Link, useNavigate } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

import { EmptyState } from "../../components/empty-state";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { StatusBadge } from "../../components/status-badge";
import { useOverviewData } from "../../hooks/useOverviewData";
import { formatShortKey } from "../../lib/format";

export function OverviewPage(): JSX.Element {
  const navigate = useNavigate();
  const dashboard = useOverviewData();
  const latestPeers = dashboard.peers.slice(0, 5);
  const autoCount = dashboard.peers.filter((peer) => peer.auto_registered).length;
  const openPath = (path: string) => {
    navigate(path);
  };
  const handleRowKeyDown = (event: KeyboardEvent<HTMLTableRowElement>, path: string) => {
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    openPath(path);
  };

  return (
    <div className="space-y-6">
      <PageHeader items={[{ label: "Overview" }]} />

      <PageSummaryCard
        description="Server health and a snapshot of peers on the first page."
        eyebrow="Overview"
        title="Dashboard"
      />

      {dashboard.error !== "" ? (
        <div className="rounded-lg border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">{dashboard.error}</div>
      ) : null}

      <section className="grid gap-4 md:grid-cols-3">
        <MetricCard
          description={formatShortKey(dashboard.serverInfo?.public_key)}
          icon={Server}
          label="Server Build"
          value={dashboard.serverInfo?.build_commit ?? "dev"}
        />
        <MetricCard
          description="First page snapshot"
          icon={Boxes}
          label="Peers This Page"
          value={String(dashboard.peers.length)}
        />
        <MetricCard
          description={`${dashboard.peers.length - autoCount} manual or approved on this page`}
          icon={ShieldCheck}
          label="Auto Registered"
          value={String(autoCount)}
        />
      </section>

      <section>
        <Card className="border-border/60 bg-background/90 shadow-sm">
          <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
            <div className="space-y-1">
              <CardTitle>Recent Peers</CardTitle>
              <CardDescription>Latest peers from the first page of results.</CardDescription>
            </div>
            <Button asChild size="sm" variant="outline">
              <Link to="/peers">
                Open Peers
                <ChevronRight className="size-4" />
              </Link>
            </Button>
          </CardHeader>
          <CardContent className="space-y-3">
            {dashboard.loading ? (
              <div className="space-y-3">
                {Array.from({ length: 4 }).map((_, index) => (
                  <Skeleton className="h-16 w-full" key={index} />
                ))}
              </div>
            ) : latestPeers.length === 0 ? (
              <EmptyState
                description="Registered peers will show up here as clickable detail entries."
                title="No peers yet"
              />
            ) : (
              <div className="rounded-md border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Peer</TableHead>
                      <TableHead>Role</TableHead>
                      <TableHead>Status</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {latestPeers.map((peer) => {
                      const path = `/peers/${encodeURIComponent(peer.public_key)}`;
                      return (
                        <TableRow
                          className="cursor-pointer hover:bg-muted/40"
                          key={peer.public_key}
                          onClick={() => openPath(path)}
                          onKeyDown={(event) => handleRowKeyDown(event, path)}
                          role="link"
                          tabIndex={0}
                        >
                          <TableCell className="font-medium">{formatShortKey(peer.public_key)}</TableCell>
                          <TableCell>{peer.role}</TableCell>
                          <TableCell>
                            <StatusBadge status={peer.status} />
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              </div>
            )}
          </CardContent>
        </Card>
      </section>

      <Card className="border-border/60 bg-background/90 shadow-sm">
        <CardHeader>
          <CardTitle>Shortcuts</CardTitle>
          <CardDescription>Jump to primary admin surfaces.</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          <Button asChild variant="outline">
            <Link to="/peers">
              <Boxes className="size-4" />
              Peers
            </Link>
          </Button>
          <Button asChild variant="outline">
            <Link to="/providers/credentials">
              <KeyRound className="size-4" />
              Credentials
            </Link>
          </Button>
          <Button asChild variant="outline">
            <Link to="/providers/minimax-tenants">
              <AudioLines className="size-4" />
              MiniMax Tenants
            </Link>
          </Button>
          <Button asChild variant="outline">
            <Link to="/providers/volc-tenants">
              <AudioLines className="size-4" />
              Volcengine Tenants
            </Link>
          </Button>
          <Button asChild variant="outline">
            <Link to="/ai/voices">
              <Mic2 className="size-4" />
              Voices
            </Link>
          </Button>
          <Button asChild variant="outline">
            <Link to="/ai/models">
              <Cpu className="size-4" />
              Models
            </Link>
          </Button>
          <Button asChild variant="outline">
            <Link to="/ai/workflows">
              <Workflow className="size-4" />
              Workflows
            </Link>
          </Button>
          <Button asChild variant="outline">
            <Link to="/ai/workspaces">
              <FolderKanban className="size-4" />
              Workspaces
            </Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}

function MetricCard({
  description,
  icon: Icon,
  label,
  value,
}: {
  description: string;
  icon: ComponentType<{ className?: string }>;
  label: string;
  value: string;
}): JSX.Element {
  return (
    <Card className="border-border/60 bg-background/90 shadow-sm">
      <CardHeader className="space-y-3">
        <div className="flex items-center justify-between">
          <CardDescription>{label}</CardDescription>
          <div className="rounded-lg border bg-primary/5 p-2 text-primary">
            <Icon className="size-4" />
          </div>
        </div>
        <div className="space-y-1">
          <CardTitle className="text-2xl">{value}</CardTitle>
          <div className="text-sm text-muted-foreground">{description}</div>
        </div>
      </CardHeader>
    </Card>
  );
}
