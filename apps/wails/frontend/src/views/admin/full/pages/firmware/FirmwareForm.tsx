import { Edit, Save } from "lucide-react";
import { DashboardTable } from "@/dashboard";
import { useState } from "react";

import type {
  Firmware,
  FirmwareSlot,
  FirmwareUpsert,
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
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormField } from "@/dashboard";
import { Input } from "@/components/ui/input";
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";

const slotKeys = ["develop", "beta", "stable", "pending"] as const;

type SlotKey = (typeof slotKeys)[number];

export type FirmwareFormState = {
  description: string;
  id: string;
  name: string;
  slots: Record<SlotKey, FirmwareSlot>;
};

type FirmwareEditorProps = {
  autoSaveSlots?: boolean;
  form: FirmwareFormState;
  infoSaveLabel?: string;
  showID?: boolean;
  onChange: (form: FirmwareFormState) => void;
  onSave: (form: FirmwareFormState) => void;
  saveLabel: string;
  saving: boolean;
};

export function FirmwareEditor({
  autoSaveSlots = false,
  form,
  infoSaveLabel,
  showID = true,
  onChange,
  onSave,
  saveLabel,
  saving,
}: FirmwareEditorProps): JSX.Element {
  const [editingSlot, setEditingSlot] = useState<SlotKey | null>(null);

  const updateSlot = (slotName: SlotKey, slot: FirmwareSlot): void => {
    const nextForm = {
      ...form,
      slots: {
        ...form.slots,
        [slotName]: slot,
      },
    };
    onChange(nextForm);
    if (autoSaveSlots) {
      onSave(nextForm);
    }
  };

  return (
    <div className="space-y-4">
      <div className="space-y-4">
        <Card>
          <CardHeader>
            <CardTitle>Firmware Info</CardTitle>
            <CardDescription>
              Caller-defined immutable ID, peer-visible name, and
              operator-facing description.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {showID ? (
              <FormField label="ID">
                <Input
                  onChange={(event) =>
                    onChange({ ...form, id: event.target.value })
                  }
                  value={form.id}
                />
              </FormField>
            ) : null}
            <FormField label="Name">
              <Input
                onChange={(event) =>
                  onChange({ ...form, name: event.target.value })
                }
                value={form.name}
              />
            </FormField>
            <FormField label="Description">
              <Textarea
                className="min-h-28"
                onChange={(event) =>
                  onChange({ ...form, description: event.target.value })
                }
                value={form.description}
              />
            </FormField>
            {infoSaveLabel ? (
              <div className="flex justify-end border-t pt-4">
                <Button
                  disabled={saving}
                  onClick={() => onSave(form)}
                  type="button"
                >
                  <Save className="size-4" />
                  {saving ? "Saving..." : infoSaveLabel}
                </Button>
              </div>
            ) : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Firmware Slots</CardTitle>
            <CardDescription>
              Release channels are edited per slot and saved together.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <SlotsEditTable form={form} onEdit={setEditingSlot} />
          </CardContent>
        </Card>
      </div>

      {editingSlot !== null ? (
        <SlotEditDialog
          onClose={() => setEditingSlot(null)}
          onSubmit={(slot) => {
            updateSlot(editingSlot, slot);
            setEditingSlot(null);
          }}
          submitLabel={autoSaveSlots ? "Save Slot" : "Apply Slot"}
          slot={form.slots[editingSlot]}
          title={editingSlot}
        />
      ) : null}

      {!infoSaveLabel ? (
        <div className="flex justify-end border-t pt-4">
          <Button disabled={saving} onClick={() => onSave(form)} type="button">
            <Save className="size-4" />
            {saving ? "Saving..." : saveLabel}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

function SlotsEditTable({
  form,
  onEdit,
}: {
  form: FirmwareFormState;
  onEdit: (slot: SlotKey) => void;
}): JSX.Element {
  return (
    <DashboardTable>
      <TableHeader>
        <TableRow>
          <TableHead className="w-32">Slot</TableHead>
          <TableHead>Description</TableHead>
          <TableHead>HTTPS .tar.zlib URL</TableHead>
          <TableHead className="w-24 text-right">Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {slotKeys.map((slotName) => {
          const slot = form.slots[slotName];
          return (
            <TableRow key={slotName}>
              <TableCell className="font-medium">{slotName}</TableCell>
              <TableCell className="max-w-[26rem] text-sm text-muted-foreground">
                {slot.description?.trim() || "-"}
              </TableCell>
              <TableCell className="max-w-[30rem]">
                {slot.package == null ? (
                  <Badge variant="outline">Not configured</Badge>
                ) : (
                  <div
                    className="truncate font-mono text-xs"
                    title={slot.package.url}
                  >
                    {slot.package.url}
                  </div>
                )}
              </TableCell>
              <TableCell className="text-right">
                <Button
                  aria-label={`Edit ${slotName} slot`}
                  className="h-8 min-w-fit px-2 text-xs"
                  onClick={() => onEdit(slotName)}
                  type="button"
                  variant="outline"
                >
                  <Edit className="size-3.5" />
                  Edit
                </Button>
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </DashboardTable>
  );
}

function SlotEditDialog({
  onClose,
  onSubmit,
  submitLabel,
  slot,
  title,
}: {
  onClose: () => void;
  onSubmit: (slot: FirmwareSlot) => void;
  submitLabel: string;
  slot: FirmwareSlot;
  title: string;
}): JSX.Element {
  const [description, setDescription] = useState(slot.description ?? "");
  const [url, setURL] = useState(slot.package?.url ?? "");
  const [sha256, setSHA256] = useState(slot.package?.sha256 ?? "");
  const [size, setSize] = useState(
    slot.package == null ? "" : String(slot.package.size),
  );
  const [validationError, setValidationError] = useState("");

  const submit = (): void => {
    const packageURL = optionalString(url);
    const packageSHA256 = optionalString(sha256);
    const packageSize = optionalPositiveInteger(size);
    const hasPackageInput =
      packageURL != null || packageSHA256 != null || size.trim() !== "";
    if (utf8Length(description.trim()) > 1024) {
      setValidationError("Description must contain at most 1024 UTF-8 bytes.");
      return;
    }
    if (
      hasPackageInput &&
      (packageURL == null ||
        !isValidFirmwarePackageURL(packageURL) ||
        packageSHA256 == null ||
        !/^[0-9a-fA-F]{64}$/.test(packageSHA256) ||
        packageSize == null)
    ) {
      setValidationError(
        "Package requires a valid HTTPS URL, a 64-character SHA-256, and a positive integer size.",
      );
      return;
    }
    const firmwarePackage =
      packageURL != null && packageSHA256 != null && packageSize != null
        ? {
            sha256: packageSHA256.toLowerCase(),
            size: packageSize,
            url: packageURL,
          }
        : undefined;
    setValidationError("");
    onSubmit({
      description: optionalString(description),
      package: firmwarePackage,
    });
  };

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) {
          onClose();
        }
      }}
    >
      <DialogContent className="max-h-[90vh] w-[calc(100vw-2rem)] max-w-[calc(100vw-2rem)] overflow-x-hidden overflow-y-auto xl:max-w-6xl">
        <DialogHeader>
          <div className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            Firmware slot
          </div>
          <DialogTitle className="capitalize">{title}</DialogTitle>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          <div className="grid gap-3">
            <FormField label="Description">
              <Input
                onChange={(event) => setDescription(event.target.value)}
                value={description}
              />
            </FormField>
          </div>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">
                External .tar.zlib package
              </CardTitle>
              <CardDescription>
                The device downloads this HTTPS URL directly. SHA-256 and size
                describe the complete tar archive compressed as one zlib stream.
              </CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4">
              <FormField label="HTTPS URL">
                <Input
                  onChange={(event) => setURL(event.target.value)}
                  placeholder="https://downloads.example.com/firmware/stable.tar.zlib"
                  type="url"
                  value={url}
                />
              </FormField>
              <FormField label="SHA-256">
                <Input
                  className="font-mono"
                  maxLength={64}
                  onChange={(event) => setSHA256(event.target.value)}
                  placeholder="64 lowercase hexadecimal characters"
                  value={sha256}
                />
              </FormField>
              <FormField label="Compressed size (bytes)">
                <Input
                  min={1}
                  onChange={(event) => setSize(event.target.value)}
                  placeholder="1048576"
                  step={1}
                  type="number"
                  value={size}
                />
              </FormField>
            </CardContent>
          </Card>

          {validationError === "" ? null : (
            <div className="rounded-lg border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
              {validationError}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button onClick={onClose} type="button" variant="outline">
            Cancel
          </Button>
          <Button onClick={submit} type="button">
            {submitLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function emptyFirmwareForm(): FirmwareFormState {
  return {
    description: "Firmware release line",
    id: "new-firmware",
    name: "New firmware",
    slots: emptySlots(),
  };
}

export function firmwareToForm(firmware: Firmware): FirmwareFormState {
  return {
    description: firmware.description ?? "",
    id: firmware.id,
    name: firmware.name,
    slots: normalizeSlots(firmware.slots),
  };
}

export function formToUpsert(form: FirmwareFormState): FirmwareUpsert {
  return {
    description: optionalString(form.description),
    id: form.id,
    name: form.name,
    slots: {
      beta: slotToUpsert(form.slots.beta),
      develop: slotToUpsert(form.slots.develop),
      pending: slotToUpsert(form.slots.pending),
      stable: slotToUpsert(form.slots.stable),
    },
  };
}

function emptySlots(): FirmwareFormState["slots"] {
  return {
    beta: {},
    develop: {},
    pending: {},
    stable: {},
  };
}

function normalizeSlots(
  slots: FirmwareUpsert["slots"],
): FirmwareFormState["slots"] {
  return {
    beta: slots.beta ?? {},
    develop: slots.develop ?? {},
    pending: slots.pending ?? {},
    stable: slots.stable ?? {},
  };
}

function slotToUpsert(slot: FirmwareSlot): FirmwareSlot {
  return {
    description: slot.description,
    package: slot.package,
  };
}

function optionalString(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

function optionalPositiveInteger(value: string): number | undefined {
  const trimmed = value.trim();
  if (trimmed === "") {
    return undefined;
  }
  const parsed = Number(trimmed);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : undefined;
}

function isValidFirmwarePackageURL(value: string): boolean {
  if (utf8Length(value) > 2048) {
    return false;
  }
  try {
    const parsed = new URL(value);
    const authority = value.slice("https://".length).split(/[/?#]/, 1)[0];
    const port = parsed.port === "" ? undefined : Number(parsed.port);
    return (
      parsed.protocol === "https:" &&
      parsed.host !== "" &&
      parsed.hostname !== "" &&
      parsed.username === "" &&
      parsed.password === "" &&
      parsed.hash === "" &&
      !authority.endsWith(":") &&
      (port == null || (Number.isInteger(port) && port >= 1 && port <= 65535))
    );
  } catch {
    return false;
  }
}

function utf8Length(value: string): number {
  return new TextEncoder().encode(value).length;
}
