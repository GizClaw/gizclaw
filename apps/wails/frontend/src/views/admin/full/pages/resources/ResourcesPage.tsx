import { Download, FileJson, RefreshCw, Save, Search, Trash2, Upload } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { validatePixa, type PixaAsset } from "@gizclaw/pixa";

import {
  applyResource,
  deleteResource,
  downloadBadgeDefPixa,
  downloadPetDefPixa,
  getResource,
  putResource,
  uploadBadgeDefPixa,
  uploadPetDefPixa,
  type ApplyResult,
  type Resource,
  type ResourceKind,
} from "@gizclaw/gizclaw/admin";
import { PixaPreviewDialog } from "@/components/pixa/PixaPreviewDialog";
import { Badge as BadgePill } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { ErrorBanner, NoticeBanner } from "@/dashboard";
import { DashboardDeleteButton as DeleteConfirmButton } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { FormField } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { expectData, toMessage } from "@/dashboard";

const resourceKinds: ResourceKind[] = [
  "Credential",
  "ACLPolicyBinding",
  "ACLRole",
  "ACLView",
  "Firmware",
  "GameRuleset",
  "PetDef",
  "BadgeDef",
  "GameDef",
  "Model",
  "DashScopeTenant",
  "GeminiTenant",
  "MiniMaxTenant",
  "OpenAITenant",
  "VolcTenant",
  "Voice",
  "Tool",
  "Workflow",
  "Workspace",
  "PeerConfig",
  "ResourceList",
];

type AdminResourceJSON = {
  apiVersion: "gizclaw.admin/v1alpha1";
  kind: ResourceKind;
  metadata: {
    annotations?: Record<string, string>;
    labels?: Record<string, string>;
    name: string;
  };
  spec: unknown;
};

type PixaPreviewState = {
  asset: PixaAsset;
  blob?: Blob;
  mode: "petdef" | "badgedef";
  pendingUpload: boolean;
};

export function ResourcesPage(): JSX.Element {
  const [searchParams, setSearchParams] = useSearchParams();
  const [kind, setKind] = useState<ResourceKind>(() => parseResourceKind(searchParams.get("kind")) ?? "ResourceList");
  const [name, setName] = useState(() => searchParams.get("name") ?? "");
  const [resource, setResource] = useState<Resource | null>(null);
  const [resourceText, setResourceText] = useState(() => JSON.stringify(resourceTemplate(kind, name), null, 2));
  const [applyResult, setApplyResult] = useState<ApplyResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [acting, setActing] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [pixaPreview, setPixaPreview] = useState<PixaPreviewState | null>(null);

  const canAddressResource = name.trim() !== "" && kind !== "ResourceList";

  const syncURL = useCallback(
    (nextKind: ResourceKind, nextName: string) => {
      const next = new URLSearchParams();
      next.set("kind", nextKind);
      if (nextName.trim() !== "") {
        next.set("name", nextName.trim());
      }
      setSearchParams(next, { replace: true });
    },
    [setSearchParams],
  );

  const resetTemplate = useCallback((nextKind: ResourceKind, nextName: string) => {
    setResource(null);
    setApplyResult(null);
    setResourceText(JSON.stringify(resourceTemplate(nextKind, nextName), null, 2));
  }, []);

  const handleKindChange = (value: string): void => {
    const nextKind = parseResourceKind(value) ?? "ResourceList";
    setKind(nextKind);
    resetTemplate(nextKind, name);
    syncURL(nextKind, name);
  };

  const handleNameChange = (value: string): void => {
    setName(value);
    if (resource === null) {
      setResourceText(JSON.stringify(resourceTemplate(kind, value), null, 2));
    }
  };

  const load = useCallback(async (): Promise<void> => {
    if (!canAddressResource) {
      setError(kind === "ResourceList" ? "ResourceList is applied as a bundle; it cannot be fetched by kind/name." : "Enter a resource name first.");
      return;
    }
    setLoading(true);
    setError("");
    setNotice("");
    setApplyResult(null);
    try {
      const next = await expectData(getResource({ path: { kind, name: name.trim() } }));
      setResource(next);
      setResourceText(JSON.stringify(next, null, 2));
      syncURL(kind, name);
      setNotice(`Loaded ${kind} ${name.trim()}.`);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  }, [canAddressResource, kind, name]);

  useEffect(() => {
    const nextKind = parseResourceKind(searchParams.get("kind")) ?? "ResourceList";
    const nextName = searchParams.get("name") ?? "";
    setKind(nextKind);
    setName(nextName);
    if (nextName.trim() !== "" && nextKind !== "ResourceList") {
      void (async () => {
        setLoading(true);
        setError("");
        try {
          const next = await expectData(getResource({ path: { kind: nextKind, name: nextName.trim() } }));
          setResource(next);
          setResourceText(JSON.stringify(next, null, 2));
        } catch {
          setResource(null);
          setResourceText(JSON.stringify(resourceTemplate(nextKind, nextName), null, 2));
        } finally {
          setLoading(false);
        }
      })();
      return;
    }
    resetTemplate(nextKind, nextName);
  }, [resetTemplate, searchParams]);

  const parseResourceText = (): AdminResourceJSON => {
    const parsed = JSON.parse(resourceText) as unknown;
    if (!isRecord(parsed)) {
      throw new Error("Resource JSON must be an object.");
    }
    if (parsed.kind !== kind) {
      throw new Error(`Resource kind must be ${kind}.`);
    }
    if (!isRecord(parsed.metadata) || typeof parsed.metadata.name !== "string" || parsed.metadata.name.trim() === "") {
      throw new Error("Resource metadata.name is required.");
    }
    if (kind !== "ResourceList" && parsed.metadata.name !== name.trim()) {
      throw new Error(`Resource metadata.name must match ${name.trim()}.`);
    }
    return parsed as AdminResourceJSON;
  };

  const applyJSON = async (): Promise<void> => {
    setActing("apply");
    setError("");
    setNotice("");
    setApplyResult(null);
    try {
      const body = parseResourceText();
      const result = await expectData(applyResource({ body: body as Resource }));
      setApplyResult(result);
      setNotice(`${result.kind} ${result.name} ${result.action}.`);
      if (body.kind !== "ResourceList") {
        setKind(body.kind);
        setName(body.metadata.name);
        syncURL(body.kind, body.metadata.name);
      }
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  const putJSON = async (): Promise<void> => {
    setActing("put");
    setError("");
    setNotice("");
    setApplyResult(null);
    try {
      if (kind === "ResourceList") {
        throw new Error("Use Apply for ResourceList bundles.");
      }
      const body = parseResourceText();
      const next = await expectData(putResource({ body: body as Resource, path: { kind, name: name.trim() } }));
      setResource(next);
      setResourceText(JSON.stringify(next, null, 2));
      syncURL(kind, name);
      setNotice(`Saved ${kind} ${name.trim()}.`);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  const remove = async (): Promise<void> => {
    setActing("delete");
    setError("");
    setNotice("");
    setApplyResult(null);
    try {
      if (!canAddressResource) {
        throw new Error("Enter a resource name first.");
      }
      await expectData(deleteResource({ path: { kind, name: name.trim() } }));
      setResource(null);
      setResourceText(JSON.stringify(resourceTemplate(kind, name), null, 2));
      setNotice(`Deleted ${kind} ${name.trim()}.`);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  const readPixaFile = async (file: File): Promise<void> => {
    const mode = kind === "PetDef" ? "petdef" : kind === "BadgeDef" ? "badgedef" : null;
    if (mode == null) {
      return;
    }
    setError("");
    setNotice("");
    try {
      const buffer = await file.arrayBuffer();
      const asset = validateResourcePixa(buffer, mode, currentPetDefPixaMetadata());
      setPixaPreview({ asset, blob: new Blob([buffer], { type: "application/octet-stream" }), mode, pendingUpload: true });
    } catch (err) {
      setError(toMessage(err));
    }
  };

  const uploadPreviewPixa = async (): Promise<void> => {
    if (pixaPreview?.blob == null || !canAddressResource) {
      return;
    }
    setActing("pixa-upload");
    setError("");
    setNotice("");
    try {
      if (pixaPreview.mode === "petdef") {
        await expectData(uploadPetDefPixa({ body: pixaPreview.blob, path: { id: name.trim() } }));
      } else {
        await expectData(uploadBadgeDefPixa({ body: pixaPreview.blob, path: { id: name.trim() } }));
      }
      setPixaPreview(null);
      setNotice(`${kind} ${name.trim()} pixa uploaded.`);
      await load();
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  const previewSavedPixa = async (): Promise<void> => {
    if (!canAddressResource || (kind !== "PetDef" && kind !== "BadgeDef")) {
      return;
    }
    const mode = kind === "PetDef" ? "petdef" : "badgedef";
    setActing("pixa-download");
    setError("");
    setNotice("");
    try {
      const blob = await expectData(kind === "PetDef" ? downloadPetDefPixa({ path: { id: name.trim() } }) : downloadBadgeDefPixa({ path: { id: name.trim() } }));
      const asset = validateResourcePixa(await blob.arrayBuffer(), mode, currentPetDefPixaMetadata());
      setPixaPreview({ asset, mode, pendingUpload: false });
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  const selectedSummary = useMemo(() => resourceSummary(kind), [kind]);
  const supportsPixa = kind === "PetDef" || kind === "BadgeDef";

  const currentPetDefPixaMetadata = (): PetDefPixaMetadata | null => {
    if (kind !== "PetDef") {
      return null;
    }
    let source: unknown;
    try {
      source = JSON.parse(resourceText) as unknown;
    } catch (err) {
      throw new Error(`PetDef JSON is invalid: ${toMessage(err)}`);
    }
    return readPetDefPixaMetadata(source);
  };

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        actions={
          <>
            <Button className="min-w-fit shrink-0 whitespace-nowrap" disabled={loading} onClick={() => void load()} size="sm" type="button" variant="outline">
              <RefreshCw className="size-4" />
              Load
            </Button>
            <Button className="min-w-fit shrink-0 whitespace-nowrap" disabled={acting !== ""} onClick={() => void applyJSON()} size="sm" type="button">
              <FileJson className="size-4" />
              Apply
            </Button>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { label: "Resources" }]}
      />

      <PageSummaryCard
        description="Direct declarative resource editor for admin resources that do not yet need a specialized page."
        eyebrow="Settings"
        meta={
          <>
            <BadgePill variant="secondary">{kind}</BadgePill>
            {resource === null ? <BadgePill variant="outline">Draft</BadgePill> : <BadgePill variant="outline">Loaded</BadgePill>}
          </>
        }
        title="Resources"
      />

      {notice !== "" ? <NoticeBanner message={notice} tone="success" /> : null}
      {error !== "" ? <ErrorBanner message={error} /> : null}

      <div className="grid gap-4 xl:grid-cols-[24rem_minmax(0,1fr)]">
        <Card>
          <CardHeader>
            <CardTitle>Target</CardTitle>
            <CardDescription>{selectedSummary}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <FormField description="ResourceList bundles can only be applied." label="Kind">
              <Select onValueChange={handleKindChange} value={kind}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {resourceKinds.map((item) => (
                    <SelectItem key={item} value={item}>
                      {item}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormField>
            <FormField description="For PeerConfig this is the peer public key." label="Name">
              <div className="flex gap-2">
                <Input onChange={(event) => handleNameChange(event.target.value)} placeholder="resource-name" value={name} />
                <Button disabled={!canAddressResource || loading} onClick={() => void load()} type="button" variant="outline">
                  <Search className="size-4" />
                </Button>
              </div>
            </FormField>

            {supportsPixa ? (
              <div className="rounded-md border p-3">
                <div className="mb-3 flex items-center justify-between gap-3">
                  <div>
                    <p className="text-sm font-medium">Pixa</p>
                    <p className="text-xs text-muted-foreground">{kind === "PetDef" ? "Requires clips listed in visual.pixa.metadata." : "Requires a single-frame icon clip."}</p>
                  </div>
                  <Button disabled={!canAddressResource || acting !== ""} onClick={() => void previewSavedPixa()} size="icon" type="button" variant="outline">
                    <Download className="size-4" />
                  </Button>
                </div>
                <label className="flex cursor-pointer items-center justify-center gap-2 rounded-md border border-dashed px-3 py-4 text-sm text-muted-foreground hover:bg-muted/50">
                  <Upload className="size-4" />
                  Upload pixa
                  <input
                    accept=".pixa,application/octet-stream"
                    className="sr-only"
                    disabled={!canAddressResource || acting !== ""}
                    onChange={(event) => {
                      const file = event.target.files?.[0];
                      event.target.value = "";
                      if (file != null) {
                        void readPixaFile(file);
                      }
                    }}
                    type="file"
                  />
                </label>
              </div>
            ) : null}

          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
            <div className="flex flex-col gap-1">
              <CardTitle>Resource JSON</CardTitle>
              <CardDescription>Edit the same JSON accepted by `gizclaw admin apply`.</CardDescription>
            </div>
            <div className="flex flex-wrap justify-end gap-2">
              <Button className="min-w-fit shrink-0 whitespace-nowrap" disabled={acting !== ""} onClick={() => void putJSON()} size="sm" type="button" variant="outline">
                <Save className="size-4" />
                Put
              </Button>
              <Button className="min-w-fit shrink-0 whitespace-nowrap" disabled={acting !== ""} onClick={() => void applyJSON()} size="sm" type="button">
                <FileJson className="size-4" />
                Apply
              </Button>
              <DeleteConfirmButton disabled={!canAddressResource || acting !== ""} onConfirm={() => void remove()} size="sm" title={`Delete ${kind}?`}>
                <Trash2 className="size-4" />
                Delete
              </DeleteConfirmButton>
            </div>
          </CardHeader>
          <CardContent>
            <Textarea
              className="min-h-[34rem] resize-y font-mono text-xs leading-5"
              onChange={(event) => setResourceText(event.target.value)}
              spellCheck={false}
              value={resourceText}
            />
          </CardContent>
        </Card>
      </div>

      {applyResult !== null ? (
        <Card>
          <CardHeader>
            <CardTitle>Apply Result</CardTitle>
            <CardDescription>Server result returned by the declarative apply endpoint.</CardDescription>
          </CardHeader>
          <CardContent>
            <pre className="max-h-[24rem] overflow-auto rounded-md bg-muted p-4 text-xs leading-5">{JSON.stringify(applyResult, null, 2)}</pre>
          </CardContent>
        </Card>
      ) : null}

      {resource === null && !loading ? (
        <EmptyState description="Load an existing resource or edit the draft JSON and apply it." title="No resource loaded" />
      ) : null}
      {pixaPreview != null ? (
        <PixaPreviewDialog
          asset={pixaPreview.asset}
          confirmLabel="Upload Pixa"
          description={`${pixaPreview.mode === "petdef" ? "PetDef" : "BadgeDef"} pixa preview`}
          onClose={() => setPixaPreview(null)}
          onConfirm={pixaPreview.pendingUpload ? () => void uploadPreviewPixa() : undefined}
          title={pixaPreview.pendingUpload ? "Preview Upload" : "Saved Pixa"}
        />
      ) : null}
    </div>
  );
}

type PetDefPixaMetadata = {
  canvas: {
    height: number;
    width: number;
  };
  clips: Array<{
    pixa_clip_name: string;
  }>;
};

function validateResourcePixa(input: ArrayBuffer | ArrayBufferView, mode: "petdef" | "badgedef", metadata: PetDefPixaMetadata | null): PixaAsset {
  if (mode === "badgedef") {
    return validatePixa(input, "badgedef");
  }
  const asset = validatePixa(input);
  if (asset.clipCount === 0 || asset.frameCount === 0) {
    throw new Error("PetDef PIXA must contain at least one clip and one frame.");
  }
  if (metadata == null) {
    throw new Error("PetDef visual.pixa.metadata is required to validate PIXA uploads.");
  }
  if (asset.canvas.width !== metadata.canvas.width || asset.canvas.height !== metadata.canvas.height) {
    throw new Error(`PetDef PIXA canvas is ${asset.canvas.width}x${asset.canvas.height}, expected ${metadata.canvas.width}x${metadata.canvas.height}.`);
  }
  for (const clip of metadata.clips) {
    if (!asset.clips.some((candidate) => candidate.name === clip.pixa_clip_name)) {
      throw new Error(`PetDef PIXA is missing clip "${clip.pixa_clip_name}".`);
    }
  }
  return asset;
}

function readPetDefPixaMetadata(value: unknown): PetDefPixaMetadata | null {
  if (!isRecord(value) || value.kind !== "PetDef" || !isRecord(value.spec)) {
    return null;
  }
  const visual = value.spec.visual;
  if (!isRecord(visual) || !isRecord(visual.pixa) || !isRecord(visual.pixa.metadata)) {
    return null;
  }
  const metadata = visual.pixa.metadata;
  if (!isRecord(metadata.canvas) || !Array.isArray(metadata.clips)) {
    return null;
  }
  const width = metadata.canvas.width;
  const height = metadata.canvas.height;
  if (typeof width !== "number" || typeof height !== "number") {
    return null;
  }
  const clips = metadata.clips.flatMap((clip) => {
    if (!isRecord(clip) || typeof clip.pixa_clip_name !== "string") {
      return [];
    }
    return [{ pixa_clip_name: clip.pixa_clip_name }];
  });
  return { canvas: { height, width }, clips };
}

function parseResourceKind(value: string | null): ResourceKind | null {
  if (value === null) {
    return null;
  }
  return resourceKinds.includes(value as ResourceKind) ? (value as ResourceKind) : null;
}

function resourceTemplate(kind: ResourceKind, name: string): AdminResourceJSON {
  const resourceName = name.trim() || resourceNamePlaceholder(kind);
  return {
    apiVersion: "gizclaw.admin/v1alpha1",
    kind,
    metadata: { name: resourceName },
    spec: resourceSpecTemplate(kind),
  };
}

function resourceNamePlaceholder(kind: ResourceKind): string {
  if (kind === "ResourceList") {
    return "bundle";
  }
  if (kind === "PeerConfig") {
    return "<peer-public-key>";
  }
  return kind.replace(/([a-z])([A-Z])/g, "$1-$2").toLowerCase();
}

function resourceSpecTemplate(kind: ResourceKind): unknown {
  switch (kind) {
    case "ResourceList":
      return { items: [] };
    case "PeerConfig":
      return {};
    case "Tool":
      return {
        source: "admin",
        enabled: true,
        input_schema: { type: "object", properties: {} },
        executor: { kind: "builtin", name: "tool.example" },
      };
    case "GameRuleset":
      return {
        enabled: true,
        points: { initial_balance: 100 },
        pet_pool: [{ petdef_id: "petdef-basic", weight: 100, adoption_cost: 10 }],
        badge_def_ids: ["badge-basic"],
        game_def_ids: ["game-basic"],
        drive: {
          game_rewards: { "game-basic": { points_delta: 10, pet_exp_delta: 20, badge_exp_delta: { "badge-basic": 100 } } },
        },
      };
    case "PetDef":
      return {
        default_locale: "en",
        attr: {
          life: { hunger: { initial: 100 }, clean: { initial: 100 } },
          progression: { xp: { initial: 0 } },
        },
        character: { prompt: "Small friendly pixel pet." },
        voice: { voice_id: "gizclaw-soft", prompt: "Soft and curious." },
        drive: {
          actions: [
            {
              id: "idle",
              cost: 0,
              visual_clip_id: "idle",
            },
            {
              id: "bath",
              cost: 5,
              visual_clip_id: "bath",
              effect: { attr_delta: { life: { clean: 10 } }, pet_exp_delta: 10 },
            },
          ],
        },
        visual: {
          refs: { images: [], videos: [] },
          pixa: {
            asset_ref: "asset://pets/starter/pet.pixa",
            metadata: {
              version: "1",
              canvas: { width: 60, height: 60 },
              clips: [
                { id: "idle", action_id: "idle", pixa_clip_name: "idle" },
                { id: "bath", action_id: "bath", pixa_clip_name: "bath" },
              ],
            },
          },
        },
        i18n: {
          en: {
            display_name: "Starter Pet",
            description: "Starter pet for gameplay resource editing.",
            attr: {
              life: { hunger: { display_name: "Hunger" }, clean: { display_name: "Clean" } },
              progression: { xp: { display_name: "XP" } },
            },
            drive: { actions: { idle: { display_name: "Idle" }, bath: { display_name: "Bath" } } },
          },
        },
      };
    case "BadgeDef":
      return { display_name: "Starter Badge" };
    case "GameDef":
      return { display_name: "Starter Game", outcomes: ["win", "lose"] };
    default:
      return {};
  }
}

function resourceSummary(kind: ResourceKind): string {
  switch (kind) {
    case "ResourceList":
      return "Apply multiple resources in one request. The server rejects get/delete for ResourceList.";
    case "PeerConfig":
      return "Desired peer configuration keyed by peer public key.";
    case "Tool":
      return "Admin-managed executable capability with typed input schema and builtin or device RPC execution.";
    case "GameRuleset":
      return "Admin-managed gameplay ruleset for pet pools, point costs, drive rewards, and game rewards.";
    case "PetDef":
      return "Admin-managed pet definition used by ruleset adoption pools.";
    case "BadgeDef":
      return "Admin-managed badge definition that peer badge progress references.";
    case "GameDef":
      return "Admin-managed playable game definition used by pet drive game results.";
    default:
      return "Generic declarative resource backed by the admin resource API.";
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
