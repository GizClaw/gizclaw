import { ChevronLeft, RefreshCw, RotateCcw, StepForward } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";

import {
  getFirmware,
  getResource,
  putFirmware,
  releaseFirmware,
  rollbackFirmware,
  type Firmware,
  type FirmwarePackage,
  type Resource,
} from "@gizclaw/gizclaw/admin";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  DashboardTable,
  DetailBlock,
  EmptyState,
  ErrorBanner,
  PageHeader,
  PageSummaryCard,
  expectData,
  toMessage,
} from "@/dashboard";
import { ResourceCliPanel } from "../../components/ResourceCliPanel";
import {
  FirmwareEditor,
  type FirmwareFormState,
  firmwareToForm,
  formToUpsert,
} from "./FirmwareForm";

export function FirmwareDetailPage(): JSX.Element {
  const params = useParams();
  const firmwareID = useMemo(
    () => decodeRouteParam(params.id ?? ""),
    [params.id],
  );
  const [firmware, setFirmware] = useState<Firmware | null>(null);
  const [resource, setResource] = useState<Resource | null>(null);
  const [form, setForm] = useState<FirmwareFormState | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [acting, setActing] = useState("");
  const [error, setError] = useState("");

  const load = async (): Promise<void> => {
    if (firmwareID === "") {
      setLoading(false);
      setError("Missing firmware ID in the URL.");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const [nextFirmware, nextResource] = await Promise.all([
        expectData(getFirmware({ path: { id: firmwareID } })),
        expectData(getResource({ path: { kind: "Firmware", id: firmwareID } })),
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
  }, [firmwareID]);

  const save = async (nextForm = form): Promise<void> => {
    setSaving(true);
    setError("");
    try {
      if (nextForm == null) {
        throw new Error("Firmware form is not loaded.");
      }
      const next = await expectData(
        putFirmware({
          body: formToUpsert({ ...nextForm, id: firmwareID }),
          path: { id: firmwareID },
        }),
      );
      setFirmware(next);
      setForm(firmwareToForm(next));
      setResource(
        await expectData(
          getResource({ path: { kind: "Firmware", id: firmwareID } }),
        ),
      );
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
      const next = await expectData(
        action === "release"
          ? releaseFirmware({ path: { id: firmwareID } })
          : rollbackFirmware({ path: { id: firmwareID } }),
      );
      setFirmware(next);
      setForm(firmwareToForm(next));
      setResource(
        await expectData(
          getResource({ path: { kind: "Firmware", id: firmwareID } }),
        ),
      );
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  if (firmwareID === "") {
    return (
      <EmptyState
        description="Missing firmware ID in the URL."
        title="Invalid route"
      />
    );
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
            <Button onClick={() => void load()} size="sm" variant="outline">
              <RefreshCw className="size-4" />
              Reload
            </Button>
          </>
        }
        items={[
          { href: "/overview", label: "Overview" },
          { href: "/firmwares", label: "Firmwares" },
          { label: firmwareID },
        ]}
      />

      <PageSummaryCard
        description="External .tar.zlib package configuration for each release channel."
        eyebrow="Devices"
        meta={
          firmware ? (
            <Badge variant="secondary">
              {slotLabel(firmware.slots.stable)}
            </Badge>
          ) : null
        }
        title={firmware?.id ?? firmwareID}
      />

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-80 w-full" />
        </div>
      ) : error !== "" && firmware === null ? (
        <ErrorBanner message={error} />
      ) : firmware === null ? (
        <EmptyState
          description="This firmware could not be loaded."
          title="Firmware not found"
        />
      ) : (
        <Tabs defaultValue="summary">
          <TabsList>
            <TabsTrigger value="summary">Summary</TabsTrigger>
            <TabsTrigger value="edit">Edit</TabsTrigger>
            <TabsTrigger value="cli">CLI</TabsTrigger>
          </TabsList>

          {error !== "" ? <ErrorBanner message={error} /> : null}

          <TabsContent className="space-y-4" value="summary">
            <DetailBlock
              items={[
                ["ID", firmware.id],
                ["Description", firmware.description],
                ["Created", firmware.created_at],
                ["Updated", firmware.updated_at],
              ]}
              title="Firmware"
            />

            <Card>
              <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
                <div className="space-y-1">
                  <CardTitle>Channels</CardTitle>
                  <CardDescription>
                    Devices fetch the configured HTTPS URL directly and verify
                    the complete .tar.zlib archive with SHA-256 and size.
                  </CardDescription>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    disabled={acting !== ""}
                    onClick={() => void runAction("release")}
                    size="sm"
                    type="button"
                    variant="outline"
                  >
                    <StepForward className="size-4" />
                    Release
                  </Button>
                  <Button
                    disabled={acting !== ""}
                    onClick={() => void runAction("rollback")}
                    size="sm"
                    type="button"
                    variant="outline"
                  >
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
                showID={false}
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
    ["develop", firmware.slots.develop],
    ["beta", firmware.slots.beta],
    ["stable", firmware.slots.stable],
    ["pending", firmware.slots.pending],
  ] as const;
  return (
    <DashboardTable>
      <TableHeader>
        <TableRow>
          <TableHead className="w-28">Channel</TableHead>
          <TableHead>Description</TableHead>
          <TableHead>HTTPS .tar.zlib URL</TableHead>
          <TableHead className="w-28">Size</TableHead>
          <TableHead className="w-48">SHA-256</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map(([name, slot]) => (
          <TableRow key={name}>
            <TableCell className="font-medium">{name}</TableCell>
            <TableCell>{slot.description?.trim() || "-"}</TableCell>
            <TableCell>
              <PackageURL firmwarePackage={slot.package} />
            </TableCell>
            <TableCell>{formatBytes(slot.package?.size)}</TableCell>
            <TableCell
              className="max-w-48 truncate font-mono text-xs"
              title={slot.package?.sha256 ?? ""}
            >
              {slot.package?.sha256 ?? "-"}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </DashboardTable>
  );
}

function PackageURL({
  firmwarePackage,
}: {
  firmwarePackage?: FirmwarePackage;
}): JSX.Element {
  if (firmwarePackage == null) {
    return <span className="text-muted-foreground">Not configured</span>;
  }
  return (
    <a
      className="break-all font-mono text-xs text-primary hover:underline"
      href={firmwarePackage.url}
      rel="noreferrer"
      target="_blank"
    >
      {firmwarePackage.url}
    </a>
  );
}

function firmwareCliCommands(firmware: Firmware): string {
  const id = shellQuote(firmware.id);
  return [
    `gizclaw admin firmwares --context <admin-cli-context> get ${id}`,
    `gizclaw admin firmwares --context <admin-cli-context> put ${id} -f firmware.json`,
    `gizclaw admin firmwares --context <admin-cli-context> release ${id}`,
    `gizclaw admin firmwares --context <admin-cli-context> rollback ${id}`,
    `gizclaw admin --context <admin-cli-context> show Firmware ${id}`,
  ].join("\n");
}

function slotLabel(slot: Firmware["slots"]["stable"]): string {
  if (slot.package != null) {
    return slot.description?.trim() || "stable configured";
  }
  return slot.description?.trim() || "no stable package";
}

function formatBytes(value: number | undefined): string {
  if (value == null || !Number.isFinite(value)) {
    return "-";
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
