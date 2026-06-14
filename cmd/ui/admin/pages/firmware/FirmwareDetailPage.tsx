import { ChevronLeft, RefreshCw, RotateCcw, StepForward, Upload } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";

import { getFirmware, getResource, putFirmware, releaseFirmware, rollbackFirmware, uploadFirmwareBin, type Firmware, type FirmwareArtifact, type Resource } from "@gizclaw/adminservice";
import { expectData, toMessage } from "../../components/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { DetailBlock } from "../../components/detail-block";
import { EmptyState } from "../../components/empty-state";
import { ErrorBanner } from "../../components/banners";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
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

  const uploadBin = async (channel: SlotKey, bin: string, file: File): Promise<void> => {
    const action = `upload:${channel}:${bin}`;
    setActing(action);
    setError("");
    try {
      const next = await expectData(uploadFirmwareBin({ body: file, path: { name: firmwareName, channel, bin } }));
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
                <SlotsTable disabled={acting !== "" || saving} firmware={firmware} onUpload={(channel, bin, file) => void uploadBin(channel, bin, file)} />
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
    </div>
  );
}

const slotKeys = ["develop", "beta", "stable", "pending"] as const;
type SlotKey = (typeof slotKeys)[number];

function SlotsTable({
  disabled,
  firmware,
  onUpload,
}: {
  disabled: boolean;
  firmware: Firmware;
  onUpload: (channel: SlotKey, bin: string, file: File) => void;
}): JSX.Element {
  const rows = [
    ["develop", firmware.slots.develop],
    ["beta", firmware.slots.beta],
    ["stable", firmware.slots.stable],
    ["pending", firmware.slots.pending],
  ] as const;
  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-32">Slot</TableHead>
            <TableHead className="w-40">Version</TableHead>
            <TableHead className="w-40">Bin</TableHead>
            <TableHead className="w-24">Kind</TableHead>
            <TableHead>Metadata</TableHead>
            <TableHead className="w-28 text-right">Upload</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.flatMap(([name, slot]) => {
            const artifacts = slot.artifacts ?? [];
            if (artifacts.length === 0) {
              return [
                <TableRow key={name}>
                  <TableCell className="font-medium">{name}</TableCell>
                  <TableCell className="font-mono text-xs">{slotVersion(slot) || "-"}</TableCell>
                  <TableCell colSpan={4} className="text-sm text-muted-foreground">
                    {slot.description?.trim() || "No artifacts."}
                  </TableCell>
                </TableRow>,
              ];
            }
            return artifacts.map((artifact) => (
              <TableRow key={`${name}:${artifact.name}`}>
                <TableCell className="font-medium">{name}</TableCell>
                <TableCell className="font-mono text-xs">{slotVersion(slot) || "-"}</TableCell>
                <TableCell className="font-mono text-xs">{artifact.name}</TableCell>
                <TableCell>
                  <Badge variant="outline">{artifact.kind}</Badge>
                </TableCell>
                <TableCell>
                  <ArtifactMetadata artifact={artifact} />
                </TableCell>
                <TableCell className="text-right">
                  <Button asChild className="h-8 min-w-fit px-2 text-xs" disabled={disabled} variant="outline">
                    <label>
                      <Upload className="size-3.5" />
                      Upload
                      <input
                        className="sr-only"
                        disabled={disabled}
                        onChange={(event) => {
                          const file = event.target.files?.[0];
                          event.currentTarget.value = "";
                          if (file != null) {
                            onUpload(name, artifact.name, file);
                          }
                        }}
                        type="file"
                      />
                    </label>
                  </Button>
                </TableCell>
              </TableRow>
            ));
          })}
        </TableBody>
      </Table>
    </div>
  );
}

function ArtifactMetadata({ artifact }: { artifact: FirmwareArtifact }): JSX.Element {
  if (artifact.path == null || artifact.path.trim() === "") {
    return <span className="text-sm text-muted-foreground">Not uploaded</span>;
  }
  return (
    <div className="grid gap-1 text-xs">
      <div className="break-all font-mono text-foreground">{artifact.path}</div>
      <div className="flex flex-wrap gap-x-3 gap-y-1 text-muted-foreground">
        <span>{formatBytes(artifact.size)}</span>
        <span>{artifact.content_type ?? "application/octet-stream"}</span>
        <span>{artifact.uploaded_at ?? "-"}</span>
      </div>
      {artifact.sha256 != null && artifact.sha256.trim() !== "" ? <div className="break-all font-mono text-muted-foreground">sha256:{artifact.sha256}</div> : null}
    </div>
  );
}

function firmwareCliCommands(firmware: Firmware): string {
  const name = shellQuote(firmware.name);
  return [
    `gizclaw admin firmwares --context <admin-cli-context> get ${name}`,
    `gizclaw admin firmwares --context <admin-cli-context> put ${name} -f firmware.json`,
    `gizclaw admin firmwares --context <admin-cli-context> upload-bin ${name} --channel stable --bin app -f app.bin`,
    `gizclaw admin firmwares --context <admin-cli-context> release ${name}`,
    `gizclaw admin firmwares --context <admin-cli-context> rollback ${name}`,
    `gizclaw admin --context <admin-cli-context> show Firmware ${name}`,
  ].join("\n");
}

function slotVersion(slot: Firmware["slots"]["stable"]): string {
  return slot.version?.trim() ?? "";
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
