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
  name: string;
  slots: Record<SlotKey, FirmwareSlot>;
};

type FirmwareEditorProps = {
  autoSaveSlots?: boolean;
  form: FirmwareFormState;
  infoSaveLabel?: string;
  showName?: boolean;
  onChange: (form: FirmwareFormState) => void;
  onSave: (form: FirmwareFormState) => void;
  saveLabel: string;
  saving: boolean;
};

export function FirmwareEditor({
  autoSaveSlots = false,
  form,
  infoSaveLabel,
  showName = true,
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
              Name and operator-facing description.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {showName ? (
              <FormField label="Name">
                <Input
                  onChange={(event) =>
                    onChange({ ...form, name: event.target.value })
                  }
                  value={form.name}
                />
              </FormField>
            ) : null}
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
          <TableHead className="w-32 text-right">Artifact</TableHead>
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
              <TableCell className="text-right">
                <Badge variant="outline">
                  {slot.artifact == null ? "None" : "Uploaded"}
                </Badge>
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

  const submit = (): void => {
    onSubmit({
      description: optionalString(description),
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
              <CardTitle className="text-base">Artifact</CardTitle>
              <CardDescription>
                Artifacts are uploaded from the firmware detail summary after
                the slot exists.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Badge variant="outline">
                {slot.artifact == null ? "Not uploaded" : "Uploaded"}
              </Badge>
            </CardContent>
          </Card>
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
    name: "new-firmware",
    slots: emptySlots(),
  };
}

export function firmwareToForm(firmware: Firmware): FirmwareFormState {
  return {
    description: firmware.description ?? "",
    name: firmware.name,
    slots: normalizeSlots(firmware.slots),
  };
}

export function formToUpsert(form: FirmwareFormState): FirmwareUpsert {
  return {
    description: optionalString(form.description),
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
  };
}

function optionalString(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}
