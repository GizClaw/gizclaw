import { ChevronLeft, Download, FileText, Folder, RefreshCw, RotateCcw, StepForward, Trash2, Upload } from "lucide-react";
import { DashboardTable } from "@/dashboard";
import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";

import {
  deleteFirmwareArtifact,
  downloadFirmwareArtifact,
  downloadFirmwareArtifactEntry,
  getFirmware,
  getResource,
  putFirmware,
  releaseFirmware,
  rollbackFirmware,
  statFirmwareArtifactEntry,
  treeFirmwareArtifactEntries,
  uploadFirmwareArtifact,
  type Firmware,
  type FirmwareArtifact,
  type FirmwareArtifactEntry,
  type FirmwareArtifactStats,
  type Resource,
} from "@gizclaw/gizclaw/admin";
import { expectData, toMessage } from "@/dashboard";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { DetailBlock } from "@/dashboard";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { EmptyState } from "@/dashboard";
import { ErrorBanner } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { ResourceCliPanel } from "../../components/ResourceCliPanel";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { FirmwareEditor, type FirmwareFormState, firmwareToForm, formToUpsert } from "./FirmwareForm";

export function FirmwareDetailPage(): JSX.Element {
  const params = useParams();
  const firmwareName = useMemo(() => decodeRouteParam(params.name ?? ""), [params.name]);
  const [firmware, setFirmware] = useState<Firmware | null>(null);
  const [resource, setResource] = useState<Resource | null>(null);
  const [form, setForm] = useState<FirmwareFormState | null>(null);
  const [artifactEntries, setArtifactEntries] = useState<Partial<Record<SlotKey, FirmwareArtifactEntry[]>>>({});
  const [inspection, setInspection] = useState<ArtifactInspection | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [acting, setActing] = useState("");
  const [error, setError] = useState("");

  const load = async (): Promise<void> => {
    if (firmwareName === "") {
      setLoading(false);
      setError("Missing firmware name in the URL.");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const [nextFirmware, nextResource] = await Promise.all([
        expectData(getFirmware({ path: { name: firmwareName } })),
        expectData(getResource({ path: { kind: "Firmware", name: firmwareName } })),
      ]);
      setFirmware(nextFirmware);
      setResource(nextResource);
      setForm(firmwareToForm(nextFirmware));
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, [firmwareName]);

  const save = async (nextForm = form): Promise<void> => {
    setSaving(true);
    setError("");
    try {
      if (nextForm == null) {
        throw new Error("Firmware form is not loaded.");
      }
      const body = formToUpsert({ ...nextForm, name: firmwareName });
      const next = await expectData(putFirmware({ body, path: { name: firmwareName } }));
      setFirmware(next);
      setForm(firmwareToForm(next));
      const nextResource = await expectData(getResource({ path: { kind: "Firmware", name: firmwareName } }));
      setResource(nextResource);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const runAction = async (action: "release" | "rollback"): Promise<void> => {
    setActing(action);
    setError("");
    try {
      const next = await expectData(action === "release" ? releaseFirmware({ path: { name: firmwareName } }) : rollbackFirmware({ path: { name: firmwareName } }));
      setFirmware(next);
      setForm(firmwareToForm(next));
      const nextResource = await expectData(getResource({ path: { kind: "Firmware", name: firmwareName } }));
      setResource(nextResource);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  const uploadArtifact = async (channel: SlotKey, file: File): Promise<void> => {
    const action = `upload:${channel}`;
    setActing(action);
    setError("");
    try {
      const next = await expectData(uploadFirmwareArtifact({ body: file, path: { name: firmwareName, channel } }));
      setFirmware(next);
      setForm(firmwareToForm(next));
      setArtifactEntries((current) => ({ ...current, [channel]: undefined }));
      const nextResource = await expectData(getResource({ path: { kind: "Firmware", name: firmwareName } }));
      setResource(nextResource);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  const removeArtifact = async (channel: SlotKey): Promise<void> => {
    const action = `delete:${channel}`;
    setActing(action);
    setError("");
    try {
      const next = await expectData(deleteFirmwareArtifact({ path: { name: firmwareName, channel } }));
      setFirmware(next);
      setForm(firmwareToForm(next));
      setArtifactEntries((current) => ({ ...current, [channel]: undefined }));
      const nextResource = await expectData(getResource({ path: { kind: "Firmware", name: firmwareName } }));
      setResource(nextResource);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  const loadArtifactFiles = async (channel: SlotKey): Promise<void> => {
    const action = `files:${channel}`;
    setActing(action);
    setError("");
    try {
      const tree = await expectData(treeFirmwareArtifactEntries({ path: { name: firmwareName, channel } }));
      setArtifactEntries((current) => ({ ...current, [channel]: tree.items }));
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  const downloadArtifactTarFile = async (channel: SlotKey): Promise<void> => {
    const action = `download-tar:${channel}`;
    setActing(action);
    setError("");
    try {
      const blob = await expectData(downloadFirmwareArtifact({ path: { name: firmwareName, channel } }));
      saveBlob(blob, `${firmwareName}-${channel}-artifact.tar`);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  const downloadArtifactEntryFile = async (channel: SlotKey, entryPath: string): Promise<void> => {
    const action = `download-file:${channel}:${entryPath}`;
    setActing(action);
    setError("");
    try {
      const blob = await expectData(downloadFirmwareArtifactEntry({ path: { name: firmwareName, channel }, query: { path: entryPath } }));
      saveBlob(blob, entryPath.split("/").pop() || "artifact-file");
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  const inspectArtifactEntry = async (channel: SlotKey, entryPath: string): Promise<void> => {
    const action = `stat-file:${channel}:${entryPath}`;
    setActing(action);
    setError("");
    try {
      const stats = await expectData(statFirmwareArtifactEntry({ path: { name: firmwareName, channel }, query: { path: entryPath } }));
      setInspection({ channel, stats });
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  if (firmwareName === "") {
    return <EmptyState description="Missing firmware name in the URL." title="Invalid route" />;
  }

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild size="sm" variant="outline">
              <Link to="/firmwares">
                <ChevronLeft className="size-4" />
                Back to list
              </Link>
            </Button>
            <Button className="min-w-fit shrink-0 whitespace-nowrap" onClick={() => void load()} size="sm" variant="outline">
              <RefreshCw className="size-4" />
              Reload
            </Button>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { href: "/firmwares", label: "Firmwares" }, { label: firmwareName }]}
      />

      <PageSummaryCard
        description="Firmware release slots and declarative resource state."
        eyebrow="Devices"
        meta={firmware ? <Badge variant="secondary">{slotVersion(firmware.slots.stable) || "no stable version"}</Badge> : null}
        title={firmware?.name ?? firmwareName}
      />

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-80 w-full" />
        </div>
      ) : error !== "" && firmware === null ? (
        <ErrorBanner message={error} />
      ) : firmware === null ? (
        <EmptyState description="This firmware could not be loaded." title="Firmware not found" />
      ) : (
        <Tabs defaultValue="summary">
          <TabsList>
            <TabsTrigger value="summary">Summary</TabsTrigger>
            <TabsTrigger value="edit">Edit</TabsTrigger>
            <TabsTrigger value="cli">CLI</TabsTrigger>
          </TabsList>

          {error !== "" ? <ErrorBanner message={error} /> : null}

          <TabsContent className="space-y-4" value="summary">
            <div className="grid gap-4 xl:grid-cols-2">
              <DetailBlock
                items={[
                  ["Name", firmware.name],
                  ["Description", firmware.description],
                  ["Created", firmware.created_at],
                  ["Updated", firmware.updated_at],
                ]}
                title="Firmware"
              />
              <DetailBlock
                items={[
                  ["Develop", slotVersion(firmware.slots.develop) || "-"],
                  ["Beta", slotVersion(firmware.slots.beta) || "-"],
                  ["Stable", slotVersion(firmware.slots.stable) || "-"],
                  ["Pending", slotVersion(firmware.slots.pending) || "-"],
                  ["Resource kind", "Firmware"],
                ]}
                title="Release State"
              />
            </div>

            <Card>
              <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
                <div className="space-y-1">
                  <CardTitle>Slots</CardTitle>
                  <CardDescription>Current develop, beta, stable, and pending slot contents.</CardDescription>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Button disabled={acting !== ""} onClick={() => void runAction("release")} size="sm" type="button" variant="outline">
                    <StepForward className="size-4" />
                    Release
                  </Button>
                  <Button disabled={acting !== ""} onClick={() => void runAction("rollback")} size="sm" type="button" variant="outline">
                    <RotateCcw className="size-4" />
                    Rollback
                  </Button>
                </div>
              </CardHeader>
              <CardContent>
                <SlotsTable
                  artifactEntries={artifactEntries}
                  disabled={acting !== "" || saving}
                  firmware={firmware}
                  onDelete={(channel) => void removeArtifact(channel)}
                  onDownloadEntry={(channel, entryPath) => void downloadArtifactEntryFile(channel, entryPath)}
                  onDownloadTar={(channel) => void downloadArtifactTarFile(channel)}
                  onInspectEntry={(channel, entryPath) => void inspectArtifactEntry(channel, entryPath)}
                  onLoadFiles={(channel) => void loadArtifactFiles(channel)}
                  onUpload={(channel, file) => void uploadArtifact(channel, file)}
                />
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent className="space-y-4" value="edit">
            {form == null ? null : (
              <FirmwareEditor
                autoSaveSlots
                form={form}
                infoSaveLabel="Save Info"
                onChange={setForm}
                onSave={(nextForm) => void save(nextForm)}
                saveLabel="Save"
                saving={saving}
                showName={false}
              />
            )}
          </TabsContent>

          <TabsContent className="space-y-4" value="cli">
            <ResourceCliPanel
              commands={firmwareCliCommands(firmware)}
              resource={resource}
              resourceDescription="JSON returned by the resource API and accepted by admin apply."
              resourceTitle="Firmware Resource Spec"
            />
          </TabsContent>
        </Tabs>
      )}
      <ArtifactStatsDialog disabled={acting !== ""} inspection={inspection} onClose={() => setInspection(null)} onDownload={(channel, entryPath) => void downloadArtifactEntryFile(channel, entryPath)} />
    </div>
  );
}

const slotKeys = ["develop", "beta", "stable", "pending"] as const;
type SlotKey = (typeof slotKeys)[number];

function SlotsTable({
  artifactEntries,
  disabled,
  firmware,
  onDelete,
  onDownloadEntry,
  onDownloadTar,
  onInspectEntry,
  onLoadFiles,
  onUpload,
}: {
  artifactEntries: Partial<Record<SlotKey, FirmwareArtifactEntry[]>>;
  disabled: boolean;
  firmware: Firmware;
  onDelete: (channel: SlotKey) => void;
  onDownloadEntry: (channel: SlotKey, path: string) => void;
  onDownloadTar: (channel: SlotKey) => void;
  onInspectEntry: (channel: SlotKey, path: string) => void;
  onLoadFiles: (channel: SlotKey) => void;
  onUpload: (channel: SlotKey, file: File) => void;
}): JSX.Element {
  const rows = [
    ["develop", firmware.slots.develop],
    ["beta", firmware.slots.beta],
    ["stable", firmware.slots.stable],
    ["pending", firmware.slots.pending],
  ] as const;
  return (
    <DashboardTable>
        <TableHeader>
          <TableRow>
            <TableHead className="w-32">Slot</TableHead>
            <TableHead className="w-40">Version</TableHead>
            <TableHead>Metadata</TableHead>
            <TableHead className="w-64 text-right">Artifact</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.flatMap(([name, slot]) => {
            const entries = artifactEntries[name] ?? [];
            return [
              <TableRow key={name}>
                <TableCell className="font-medium">{name}</TableCell>
                <TableCell className="font-mono text-xs">{slotVersion(slot) || "-"}</TableCell>
                <TableCell>
                  {slot.artifact == null ? (
                    <span className="text-sm text-muted-foreground">{slot.description?.trim() || "No artifact uploaded."}</span>
                  ) : (
                    <ArtifactMetadata artifact={slot.artifact} />
                  )}
                </TableCell>
                <TableCell className="text-right">
                  <div className="flex flex-wrap justify-end gap-2">
                    {slot.artifact == null ? (
                      <Button asChild className="h-8 min-w-fit px-2 text-xs" disabled={disabled} variant="outline">
                        <label>
                          <Upload className="size-3.5" />
                          Upload tar
                          <input
                            className="sr-only"
                            disabled={disabled}
                            onChange={(event) => {
                              const file = event.target.files?.[0];
                              event.currentTarget.value = "";
                              if (file != null) {
                                onUpload(name, file);
                              }
                            }}
                            type="file"
                          />
                        </label>
                      </Button>
                    ) : (
                      <>
                        <Button className="h-8 min-w-fit px-2 text-xs" disabled={disabled} onClick={() => onLoadFiles(name)} type="button" variant="outline">
                          <Folder className="size-3.5" />
                          Files
                        </Button>
                        <Button className="h-8 min-w-fit px-2 text-xs" disabled={disabled} onClick={() => onDownloadTar(name)} type="button" variant="outline">
                          <Download className="size-3.5" />
                          Tar
                        </Button>
                        <Button className="h-8 min-w-fit px-2 text-xs" disabled={disabled} onClick={() => onDelete(name)} type="button" variant="outline">
                          <Trash2 className="size-3.5" />
                          Delete
                        </Button>
                      </>
                    )}
                  </div>
                </TableCell>
              </TableRow>,
              ...entries.map((entry) => (
                <TableRow key={`${name}:${entry.path}`}>
                  <TableCell className="font-medium">{name}</TableCell>
                  <TableCell>
                    <Badge variant="outline">{entry.type}</Badge>
                  </TableCell>
                  <TableCell>
                    <div className="flex min-w-0 items-center gap-2">
                      {entry.type === "dir" ? <Folder className="size-3.5 shrink-0 text-muted-foreground" /> : <FileText className="size-3.5 shrink-0 text-muted-foreground" />}
                      <div className="min-w-0">
                        <div className="truncate font-mono text-xs" title={entry.path}>
                          {entry.path}
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {entry.type === "file" ? formatBytes(entry.size) : "-"} · {entry.content_type ?? "-"} · {entry.mod_time}
                        </div>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-2">
                      <Button className="h-8 min-w-fit px-2 text-xs" disabled={disabled} onClick={() => onInspectEntry(name, entry.path)} type="button" variant="outline">
                        Info
                      </Button>
                      {entry.type === "file" ? (
                        <Button className="h-8 min-w-fit px-2 text-xs" disabled={disabled} onClick={() => onDownloadEntry(name, entry.path)} type="button" variant="outline">
                          <Download className="size-3.5" />
                          Download
                        </Button>
                      ) : null}
                    </div>
                  </TableCell>
                </TableRow>
              )),
            ];
          })}
        </TableBody>
      </DashboardTable>
  );
}

type ArtifactInspection = {
  channel: SlotKey;
  stats: FirmwareArtifactStats;
};

function ArtifactStatsDialog({
  disabled,
  inspection,
  onClose,
  onDownload,
}: {
  disabled: boolean;
  inspection: ArtifactInspection | null;
  onClose: () => void;
  onDownload: (channel: SlotKey, path: string) => void;
}): JSX.Element {
  const entry = inspection?.stats.entry;
  return (
    <Dialog
      open={inspection != null}
      onOpenChange={(open) => {
        if (!open) {
          onClose();
        }
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Artifact entry</DialogTitle>
          <DialogDescription>Firmware artifact file or directory metadata.</DialogDescription>
        </DialogHeader>
        {inspection == null ? null : (
          <div className="grid gap-3 text-sm">
            <DetailBlock
              items={[
                ["Firmware", inspection.stats.firmware_id],
                ["Channel", inspection.stats.channel],
                ["Path", inspection.stats.path || "/"],
                ["Files", String(inspection.stats.files_count)],
                ["Total size", formatBytes(inspection.stats.total_size)],
                ["Artifact", inspection.stats.artifact.tar_path],
              ]}
              title="Entry Stats"
            />
            {entry == null ? null : (
              <DetailBlock
                items={[
                  ["Type", entry.type],
                  ["Path", entry.path],
                  ["Size", formatBytes(entry.size)],
                  ["Content type", entry.content_type],
                  ["Mode", String(entry.mode)],
                  ["Modified", entry.mod_time],
                ]}
                title="Selected Entry"
              />
            )}
          </div>
        )}
        <DialogFooter>
          <Button onClick={onClose} type="button" variant="outline">
            Close
          </Button>
          {inspection != null && entry?.type === "file" ? (
            <Button disabled={disabled} onClick={() => onDownload(inspection.channel, entry.path)} type="button" variant="outline">
              <Download className="size-4" />
              Download
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ArtifactMetadata({ artifact }: { artifact: FirmwareArtifact }): JSX.Element {
  if (artifact.tar_path.trim() === "") {
    return <span className="text-sm text-muted-foreground">Not uploaded</span>;
  }
  return (
    <div className="grid gap-1 text-xs">
      <div className="break-all font-mono text-foreground">{artifact.tar_path}</div>
      <div className="flex flex-wrap gap-x-3 gap-y-1 text-muted-foreground">
        <span>{formatBytes(artifact.size)}</span>
        <span>{artifact.content_type}</span>
        <span>{artifact.uploaded_at}</span>
      </div>
      {artifact.sha256.trim() !== "" ? <div className="break-all font-mono text-muted-foreground">sha256:{artifact.sha256}</div> : null}
    </div>
  );
}

function firmwareCliCommands(firmware: Firmware): string {
  const name = shellQuote(firmware.name);
  return [
    `gizclaw admin firmwares --context <admin-cli-context> get ${name}`,
    `gizclaw admin firmwares --context <admin-cli-context> put ${name} -f firmware.json`,
    `gizclaw admin firmwares --context <admin-cli-context> upload-artifact ${name} --channel stable -f artifact.tar`,
    `gizclaw admin firmwares --context <admin-cli-context> artifact tree ${name} --channel stable`,
    `gizclaw admin firmwares --context <admin-cli-context> release ${name}`,
    `gizclaw admin firmwares --context <admin-cli-context> rollback ${name}`,
    `gizclaw admin --context <admin-cli-context> show Firmware ${name}`,
  ].join("\n");
}

function slotVersion(slot: Firmware["slots"]["stable"]): string {
  return slot.description?.trim() ?? "";
}

function formatBytes(value: number | undefined): string {
  if (value == null || !Number.isFinite(value)) {
    return "- bytes";
  }
  if (value < 1024) {
    return `${value} bytes`;
  }
  const units = ["KiB", "MiB", "GiB"];
  let next = value / 1024;
  for (const unit of units) {
    if (next < 1024) {
      return `${next.toFixed(next < 10 ? 1 : 0)} ${unit}`;
    }
    next /= 1024;
  }
  return `${next.toFixed(0)} TiB`;
}

function saveBlob(blob: Blob | File, fileName: string): void {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = fileName;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

function decodeRouteParam(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}
