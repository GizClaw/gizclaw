import { ChevronLeft, RefreshCw, RotateCcw, StepForward } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";

import { getFirmware, getResource, putFirmware, releaseFirmware, rollbackFirmware, type Firmware, type Resource } from "@gizclaw/adminservice";
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
                  ["Stable", slotVersion(firmware.slots.stable) || "-"],
                  ["Rollback", slotVersion(firmware.slots.rollback) || "-"],
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
                  <CardDescription>Current rollback, stable, beta, develop, and pending slot contents.</CardDescription>
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
                <SlotsTable firmware={firmware} />
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

function SlotsTable({ firmware }: { firmware: Firmware }): JSX.Element {
  const rows = [
    ["rollback", firmware.slots.rollback],
    ["stable", firmware.slots.stable],
    ["beta", firmware.slots.beta],
    ["develop", firmware.slots.develop],
    ["pending", firmware.slots.pending],
  ] as const;
  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-32">Slot</TableHead>
            <TableHead className="w-40">Version</TableHead>
            <TableHead>Description</TableHead>
            <TableHead className="text-right">Artifacts</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map(([name, slot]) => (
            <TableRow key={name}>
              <TableCell className="font-medium">{name}</TableCell>
              <TableCell className="font-mono text-xs">{slotVersion(slot) || "-"}</TableCell>
              <TableCell className="max-w-[26rem] text-sm text-muted-foreground">{slot.description?.trim() || "-"}</TableCell>
              <TableCell className="text-right">{slot.artifacts?.length ?? 0}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

function firmwareCliCommands(firmware: Firmware): string {
  const name = shellQuote(firmware.name);
  return [
    `gizclaw admin firmwares --context <admin-cli-context> get ${name}`,
    `gizclaw admin firmwares --context <admin-cli-context> put ${name} -f firmware.json`,
    `gizclaw admin firmwares --context <admin-cli-context> release ${name}`,
    `gizclaw admin firmwares --context <admin-cli-context> rollback ${name}`,
    `gizclaw admin --context <admin-cli-context> show Firmware ${name}`,
  ].join("\n");
}

function slotVersion(slot: Firmware["slots"]["stable"]): string {
  return slot.version?.trim() ?? "";
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
