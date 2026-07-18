import { useCallback, useEffect, useMemo, useState } from "react";
import { Ban, Check, ChevronLeft, RefreshCw, Save, Trash2 } from "lucide-react";
import { Link, useNavigate, useParams } from "react-router-dom";

import { expectData, toMessage } from "@/dashboard";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

import {
  approvePeer,
  blockPeer,
  deletePeer,
  getResource,
  putPeerInfo,
  refreshPeer,
  type DeviceInfo,
  type PeerRole,
  type Resource,
} from "@gizclaw/gizclaw/admin";

import { ResourceCliPanel } from "../../components/ResourceCliPanel";
import { DetailBlock } from "@/dashboard";
import { ErrorBanner, NoticeBanner } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { FormField } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { StatusBadge } from "@/dashboard";
import { usePeerDetail } from "../../hooks/usePeerDetail";
import { formatDate, formatShortKey, peerTitle } from "../../lib/format";
import { PeerTelemetryPanel } from "./PeerTelemetryPanel";

export function PeerDetailPage(): JSX.Element {
  const params = useParams();
  const navigate = useNavigate();
  const rawKey = params.publicKey ?? "";
  const publicKey = useMemo(() => {
    try {
      return decodeURIComponent(rawKey);
    } catch {
      return rawKey;
    }
  }, [rawKey]);

  const detail = usePeerDetail(publicKey === "" ? undefined : publicKey);
  const [peerNotice, setPeerNotice] = useState<{ message: string; tone: "error" | "success" } | null>(null);
  const [peerActionBusy, setPeerActionBusy] = useState<string | null>(null);
  const [approveRole, setApproveRole] = useState<PeerRole>("client");
  const [deviceName, setDeviceName] = useState("");
  const [deviceEmoji, setDeviceEmoji] = useState("");
  const [peerConfigResource, setPeerConfigResource] = useState<Resource | null>(null);

  const registration = detail.data?.registration ?? null;
  const isBlocked = registration?.status === "blocked";
  const isActive = registration?.status === "active";
  const isApproved = isActive && registration?.role !== "unspecified";

  useEffect(() => {
    if (detail.data?.registration?.role && detail.data.registration.role !== "unspecified") {
      setApproveRole(detail.data.registration.role);
    }
    setDeviceName(detail.data?.info?.name ?? "");
    setDeviceEmoji(detail.data?.info?.emoji ?? "");
  }, [detail.data?.info?.emoji, detail.data?.info?.name, detail.data?.registration?.role]);

  const loadPeerConfigResource = useCallback(async () => {
    if (publicKey === "") {
      setPeerConfigResource(null);
      return;
    }
    try {
      const resource = await expectData(getResource({ path: { kind: "PeerConfig", name: publicKey } }));
      setPeerConfigResource(resource);
    } catch {
      setPeerConfigResource(null);
    }
  }, [publicKey]);

  useEffect(() => {
    void loadPeerConfigResource();
  }, [loadPeerConfigResource]);

  const runPeerAction = useCallback(async (name: string, action: () => Promise<void>, successMessage: string) => {
    setPeerActionBusy(name);
    setPeerNotice(null);
    try {
      await action();
      setPeerNotice({ message: successMessage, tone: "success" });
    } catch (error) {
      setPeerNotice({ message: toMessage(error), tone: "error" });
    } finally {
      setPeerActionBusy(null);
    }
  }, []);

  const handleApprove = useCallback(async () => {
    if (publicKey === "") {
      return;
    }
    const nextRole = approveRole;
    await runPeerAction(
      "approve",
      async () => {
        await expectData(
          approvePeer({
            body: { role: nextRole },
            path: { publicKey },
          }),
        );
        await detail.reload();
      },
      isApproved ? `Peer role saved as ${nextRole}.` : `Peer approved as ${nextRole}.`,
    );
  }, [approveRole, detail, isApproved, publicKey, runPeerAction]);

  const handleUnblock = useCallback(async () => {
    if (publicKey === "") {
      return;
    }
    const nextRole = approveRole;
    await runPeerAction(
      "unblock",
      async () => {
        await expectData(
          approvePeer({
            body: { role: nextRole },
            path: { publicKey },
          }),
        );
        await detail.reload();
      },
      `Peer restored as ${nextRole}.`,
    );
  }, [approveRole, detail, publicKey, runPeerAction]);

  const handleBlock = useCallback(async () => {
    if (publicKey === "") {
      return;
    }
    await runPeerAction(
      "block",
      async () => {
        await expectData(blockPeer({ path: { publicKey } }));
        await detail.reload();
      },
      "Peer blocked.",
    );
  }, [detail, publicKey, runPeerAction]);

  const handleRefreshPeer = useCallback(async () => {
    if (publicKey === "") {
      return;
    }
    await runPeerAction(
      "refresh",
      async () => {
        await expectData(refreshPeer({ path: { publicKey } }));
        await detail.reload();
      },
      "Peer refreshed.",
    );
  }, [detail, publicKey, runPeerAction]);

  const handleDeletePeer = useCallback(async () => {
    if (publicKey === "") {
      return;
    }
    await runPeerAction(
      "delete",
      async () => {
        await expectData(deletePeer({ path: { publicKey } }));
        navigate("/peers");
      },
      "Peer deleted.",
    );
  }, [navigate, publicKey, runPeerAction]);

  const handleSaveInfo = useCallback(async () => {
    if (publicKey === "") {
      return;
    }
    await runPeerAction(
      "info",
      async () => {
        const trimmedName = deviceName.trim();
        const trimmedEmoji = deviceEmoji.trim();
        const nextInfo: DeviceInfo = {
          name: trimmedName,
          emoji: trimmedEmoji,
        };
        await expectData(
          putPeerInfo({
            body: nextInfo,
            path: { publicKey },
          }),
        );
        await detail.reload();
      },
      deviceName.trim() === "" ? "Peer name cleared." : `Peer renamed to ${deviceName.trim()}.`,
    );
  }, [detail, deviceEmoji, deviceName, publicKey, runPeerAction]);

  if (publicKey === "") {
    return <EmptyState description="Missing peer public key in the URL." title="Invalid route" />;
  }

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild size="sm" variant="outline">
              <Link to="/peers">
                <ChevronLeft className="size-4" />
                Back to list
              </Link>
            </Button>
            <Button className="min-w-fit shrink-0 whitespace-nowrap" onClick={() => void detail.reload()} size="sm" variant="outline">
              <span className="inline-flex items-center gap-2 whitespace-nowrap">
                <RefreshCw className="size-4" />
                Reload
              </span>
            </Button>
          </>
        }
        items={[
          { href: "/overview", label: "Overview" },
          { href: "/peers", label: "Peers" },
          { label: formatShortKey(publicKey) },
        ]}
      />

      <PageSummaryCard
        actions={
          registration ? (
            <Button disabled={peerActionBusy !== null} onClick={() => void handleRefreshPeer()} size="sm" type="button" variant="outline">
              <RefreshCw className="size-4" />
              Refresh Peer
            </Button>
          ) : null
        }
        description={<span className="break-all font-mono text-xs">{publicKey}</span>}
        eyebrow="Peers"
        meta={
          registration ? (
            <>
              <StatusBadge status={registration.status} />
              <Badge variant="outline">{registration.role}</Badge>
              {registration.auto_registered ? <Badge variant="secondary">Auto Registered</Badge> : null}
              {detail.data?.runtime?.online ? <Badge variant="success">Online</Badge> : <Badge variant="outline">Offline</Badge>}
            </>
          ) : null
        }
        title={registration ? peerTitle(detail.data?.info, registration.public_key) : "Peer"}
      />

      {detail.loading ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      ) : detail.error !== "" ? (
        <ErrorBanner message={detail.error} />
      ) : registration === null ? (
        <EmptyState description="This peer could not be loaded." title="Not found" />
      ) : (
        <div className="space-y-4">
          {peerNotice !== null ? <NoticeBanner message={peerNotice.message} tone={peerNotice.tone} /> : null}

          <Tabs className="space-y-4" defaultValue="info">
            <TabsList className="grid h-auto w-full grid-cols-4 lg:w-[34rem]">
              <TabsTrigger value="info">Info</TabsTrigger>
              <TabsTrigger value="edit">Edit</TabsTrigger>
              <TabsTrigger value="telemetry">Telemetry</TabsTrigger>
              <TabsTrigger value="cli">CLI</TabsTrigger>
            </TabsList>

            <TabsContent className="space-y-4" value="info">
              <div className="grid gap-4 xl:grid-cols-2">
                <DetailBlock
                  items={[
                    ["Name", detail.data?.info?.name],
                    ["Emoji", detail.data?.info?.emoji],
                    ["Serial", detail.data?.info?.identifiers?.sn],
                    ["Manufacturer", detail.data?.info?.hardware?.manufacturer],
                    ["Model", detail.data?.info?.hardware?.model],
                    ["Revision", detail.data?.info?.hardware?.hardware_revision],
                  ]}
                  title="Peer Info"
                />
                <DetailBlock
                  items={[
                    ["Public Key", registration.public_key],
                    ["Role", registration.role],
                    ["Status", registration.status],
                    ["Auto registered", registration.auto_registered ? "Yes" : "No"],
                    ["Created", registration.created_at],
                    ["Approved", registration.approved_at],
                    ["Updated", registration.updated_at],
                  ]}
                  title="Registration"
                />
                <DetailBlock
                  items={[
                    ["View", detail.data?.config?.view],
                    ["Resource kind", "PeerConfig"],
                    ["Resource name", registration.public_key],
                  ]}
                  title="Configuration"
                />
                <DetailBlock
                  items={[
                    ["Online", detail.data?.runtime?.online ? "Yes" : "No"],
                    ["Last Seen", formatDate(detail.data?.runtime?.last_seen_at)],
                    ["Last Address", detail.data?.runtime?.last_addr],
                    ["RX Bytes", formatBytes(detail.data?.runtime?.rx_bytes)],
                    ["TX Bytes", formatBytes(detail.data?.runtime?.tx_bytes)],
                  ]}
                  title="Runtime"
                />
              </div>

              <Card className="min-w-0">
                <CardHeader className="pb-3">
                  <CardTitle className="text-base">Raw Detail</CardTitle>
                  <CardDescription>Combined registration, info, config, and runtime payloads.</CardDescription>
                </CardHeader>
                <CardContent className="pt-6">
                  <pre className="max-h-[32rem] min-w-0 overflow-x-auto rounded-lg border bg-muted/50 p-4 text-xs leading-6 text-foreground">
                    {JSON.stringify(detail.data, null, 2)}
                  </pre>
                </CardContent>
              </Card>
            </TabsContent>

            <TabsContent className="space-y-4" value="edit">
              <div className="grid gap-4 xl:grid-cols-[1.2fr_0.8fr]">
                <Card>
                  <CardHeader className="pb-3">
                    <CardTitle className="text-base">Device Info</CardTitle>
                    <CardDescription>Set the operator-facing name shown in peer lists and detail headers.</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <FormField description="Leave blank to clear the stored device name." label="Name">
                      <Input
                        onChange={(event) => setDeviceName(event.target.value)}
                        placeholder="Living room display"
                        value={deviceName}
                      />
                    </FormField>
                    <FormField description="A platform-rendered Unicode emoji sequence, up to 64 UTF-8 bytes." label="Emoji">
                      <Input
                        onChange={(event) => setDeviceEmoji(event.target.value)}
                        placeholder="🧑‍🚀"
                        value={deviceEmoji}
                      />
                    </FormField>
                    <div className="flex justify-end border-t pt-4">
                      <Button disabled={peerActionBusy !== null} onClick={() => void handleSaveInfo()} type="button">
                        <Save className="size-4" />
                        Save Info
                      </Button>
                    </div>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader className="pb-3">
                    <CardTitle className="text-base">Peer Actions</CardTitle>
                    <CardDescription>Approve, restore, block, refresh, or reset this peer registration.</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <FormField
                      description={
                        isBlocked
                          ? "Choose the role to assign when this peer is restored."
                          : "Choose the role to assign when this peer moves into service, or block it from this same flow."
                      }
                      label={isBlocked ? "Restore role" : "Approval role"}
                    >
                      <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
                        <Select onValueChange={(value) => setApproveRole(value as PeerRole)} value={approveRole}>
                          <SelectTrigger id="approve-role">
                            <SelectValue placeholder="Select role" />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="client">client</SelectItem>
                            <SelectItem value="server">server</SelectItem>
                            <SelectItem value="edge-node">edge-node</SelectItem>
                            <SelectItem value="admin">admin</SelectItem>
                          </SelectContent>
                        </Select>
                        <div className="flex flex-wrap gap-2">
                          {isBlocked ? (
                            <Button className="w-full md:w-auto" disabled={peerActionBusy !== null} onClick={() => void handleUnblock()} type="button">
                              <Check className="size-4" />
                              Unblock
                            </Button>
                          ) : (
                            <>
                              <Button className="w-full md:w-auto" disabled={peerActionBusy !== null} onClick={() => void handleApprove()} type="button">
                                <Check className="size-4" />
                                {isApproved ? "Save Role" : "Approve"}
                              </Button>
                              <Button
                                className="w-full md:w-auto"
                                disabled={peerActionBusy !== null}
                                onClick={() => void handleBlock()}
                                type="button"
                                variant="outline"
                              >
                                <Ban className="size-4" />
                                Block
                              </Button>
                            </>
                          )}
                        </div>
                      </div>
                    </FormField>

                    <div className="space-y-3 rounded-lg border bg-muted/20 p-4">
                      <div className="space-y-1">
                        <div className="text-sm font-medium">Registration reset</div>
                        <p className="text-sm leading-6 text-muted-foreground">Reset the peer registration back to the unapproved state.</p>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        <Button disabled={peerActionBusy !== null} onClick={() => void handleDeletePeer()} type="button" variant="outline">
                          <Trash2 className="size-4" />
                          Reset
                        </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>

              </div>
            </TabsContent>

            <TabsContent className="space-y-4" value="telemetry">
              <PeerTelemetryPanel publicKey={registration.public_key} />
            </TabsContent>

            <TabsContent className="space-y-4" value="cli">
              <ResourceCliPanel
                commands={peerCliCommands(registration.public_key, registration.role)}
                resource={peerConfigResource}
                resourceDescription="JSON returned by the resource API and accepted by admin apply. PeerConfig manages desired peer configuration."
                resourceTitle="PeerConfig Resource Spec"
              />
            </TabsContent>
          </Tabs>
        </div>
      )}
    </div>
  );
}

function formatBytes(value: number | undefined): string {
  if (value === undefined) {
    return "—";
  }
  if (!Number.isFinite(value) || value <= 0) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = value;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  const precision = size >= 10 || unitIndex === 0 ? 0 : 1;
  return `${size.toFixed(precision)} ${units[unitIndex]}`;
}

function peerCliCommands(publicKey: string, role: PeerRole): string {
  const key = shellQuote(publicKey);
  const nextRole = shellQuote(role === "unspecified" ? "client" : role);
  return [
    `# Read this peer registration`,
    `gizclaw admin peers --context <admin-cli-context> get ${key}`,
    ``,
    `# Read peer snapshots`,
    `gizclaw admin peers --context <admin-cli-context> info ${key}`,
    `gizclaw admin peers --context <admin-cli-context> config ${key}`,
    `gizclaw admin peers --context <admin-cli-context> runtime ${key}`,
    ``,
    `# Refresh state from the device-side API`,
    `gizclaw admin peers --context <admin-cli-context> refresh ${key}`,
    ``,
    `# Approve or block this peer`,
    `gizclaw admin peers --context <admin-cli-context> approve ${key} ${nextRole}`,
    `gizclaw admin peers --context <admin-cli-context> block ${key}`,
    ``,
    `# Update desired configuration`,
    `gizclaw admin peers --context <admin-cli-context> put-config ${key} --file config.json`,
    ``,
    `# Show/apply the declarative PeerConfig resource`,
    `gizclaw admin --context <admin-cli-context> show PeerConfig ${key}`,
    `gizclaw admin --context <admin-cli-context> apply -f peer-config.json`,
    ``,
    `# Reset this peer registration`,
    `gizclaw admin peers --context <admin-cli-context> delete ${key}`,
  ].join("\n");
}

function shellQuote(value: string): string {
  return `'${value.replaceAll("'", "'\\''")}'`;
}
