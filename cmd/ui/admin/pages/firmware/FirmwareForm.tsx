import { Edit, Plus, Save, Trash2 } from "lucide-react";
import { useState } from "react";

import type { Firmware, FirmwareArtifact, FirmwareSlot, FirmwareUpsert } from "@gizclaw/adminservice";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { FormField } from "../../components/form-field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";

const slotKeys = ["develop", "beta", "stable", "pending"] as const;

type SlotKey = (typeof slotKeys)[number];
type ArtifactForm = {
  kind: "app" | "data";
  name: string;
};

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
            <CardDescription>Name and operator-facing description.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {showName ? (
              <FormField label="Name">
                <Input onChange={(event) => onChange({ ...form, name: event.target.value })} value={form.name} />
              </FormField>
            ) : null}
            <FormField label="Description">
              <Textarea
                className="min-h-28"
                onChange={(event) => onChange({ ...form, description: event.target.value })}
                value={form.description}
              />
            </FormField>
            {infoSaveLabel ? (
              <div className="flex justify-end border-t pt-4">
                <Button disabled={saving} onClick={() => onSave(form)} type="button">
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
            <CardDescription>Release channels are edited per slot and saved together.</CardDescription>
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

function SlotsEditTable({ form, onEdit }: { form: FirmwareFormState; onEdit: (slot: SlotKey) => void }): JSX.Element {
  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-32">Slot</TableHead>
            <TableHead className="w-40">Version</TableHead>
            <TableHead>Description</TableHead>
            <TableHead className="w-32 text-right">Artifacts</TableHead>
            <TableHead className="w-24 text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {slotKeys.map((slotName) => {
            const slot = form.slots[slotName];
            return (
              <TableRow key={slotName}>
                <TableCell className="font-medium">{slotName}</TableCell>
                <TableCell className="font-mono text-xs">{slot.version?.trim() || "-"}</TableCell>
                <TableCell className="max-w-[26rem] text-sm text-muted-foreground">{slot.description?.trim() || "-"}</TableCell>
                <TableCell className="text-right">
                  <Badge variant="outline">{slot.artifacts?.length ?? 0}</Badge>
                </TableCell>
                <TableCell className="text-right">
                  <Button aria-label={`Edit ${slotName} slot`} className="h-8 min-w-fit px-2 text-xs" onClick={() => onEdit(slotName)} type="button" variant="outline">
                    <Edit className="size-3.5" />
                    Edit
                  </Button>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
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
  const [version, setVersion] = useState(slot.version ?? "");
  const [description, setDescription] = useState(slot.description ?? "");
  const [artifacts, setArtifacts] = useState<ArtifactForm[]>(() => (slot.artifacts ?? []).map(artifactToForm));

  const updateArtifact = (index: number, patch: Partial<ArtifactForm>): void => {
    setArtifacts((current) => current.map((artifact, itemIndex) => (itemIndex === index ? { ...artifact, ...patch } : artifact)));
  };

  const submit = (): void => {
    onSubmit({
      artifacts: artifacts.map(formToArtifact).filter((artifact) => artifact.name.trim() !== ""),
      description: optionalString(description),
      version: optionalString(version),
    });
  };

  return (
    <Dialog open onOpenChange={(open) => {
      if (!open) {
        onClose();
      }
    }}>
      <DialogContent className="max-h-[90vh] w-[calc(100vw-2rem)] max-w-[calc(100vw-2rem)] overflow-x-hidden overflow-y-auto xl:max-w-6xl">
        <DialogHeader>
          <div className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Firmware slot</div>
          <DialogTitle className="capitalize">{title}</DialogTitle>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          <div className="grid gap-3 md:grid-cols-2">
            <FormField label="Version">
              <Input onChange={(event) => setVersion(event.target.value)} value={version} />
            </FormField>
            <FormField label="Description">
              <Input onChange={(event) => setDescription(event.target.value)} value={description} />
            </FormField>
          </div>

          <Card>
            <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
              <div className="space-y-1">
                <CardTitle className="text-base">Artifacts</CardTitle>
                <CardDescription>Device-managed artifact entries for this slot.</CardDescription>
              </div>
              <Button
                className="min-w-fit shrink-0"
                onClick={() => setArtifacts((current) => [...current, emptyArtifactForm()])}
                size="sm"
                type="button"
                variant="outline"
              >
                <Plus className="size-4" />
                Add
              </Button>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              {artifacts.length === 0 ? <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">No artifacts.</div> : null}
              {artifacts.map((artifact, index) => (
                <div className="grid min-w-0 gap-3 rounded-md border p-3 md:grid-cols-[minmax(0,1fr)_8rem_auto]" key={index}>
                  <Input aria-label={`Artifact ${index + 1} name`} className="min-w-0" onChange={(event) => updateArtifact(index, { name: event.target.value })} placeholder="name" value={artifact.name} />
                  <Select onValueChange={(value) => updateArtifact(index, { kind: value as ArtifactForm["kind"] })} value={artifact.kind}>
                    <SelectTrigger aria-label={`Artifact ${index + 1} kind`}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="app">app</SelectItem>
                      <SelectItem value="data">data</SelectItem>
                    </SelectContent>
                  </Select>
                  <Button
                    aria-label={`Remove artifact ${index + 1}`}
                    className="h-9 w-9 p-0"
                    onClick={() => setArtifacts((current) => current.filter((_, itemIndex) => itemIndex !== index))}
                    type="button"
                    variant="ghost"
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              ))}
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
    slots: normalizeSlots(form.slots),
  };
}

function emptySlots(): FirmwareFormState["slots"] {
  return {
    beta: {},
    develop: {},
    pending: {},
    stable: {
      version: "1.0.0",
    },
  };
}

function normalizeSlots(slots: FirmwareUpsert["slots"]): FirmwareFormState["slots"] {
  return {
    beta: slots.beta ?? {},
    develop: slots.develop ?? {},
    pending: slots.pending ?? {},
    stable: slots.stable ?? {},
  };
}

function artifactToForm(artifact: FirmwareArtifact): ArtifactForm {
  return {
    kind: artifact.kind === "data" ? "data" : "app",
    name: artifact.name,
  };
}

function formToArtifact(form: ArtifactForm): FirmwareArtifact {
  return {
    kind: form.kind,
    name: form.name,
  };
}

function emptyArtifactForm(): ArtifactForm {
  return {
    kind: "app",
    name: "",
  };
}

function optionalString(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}
