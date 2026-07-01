import { Download, FileJson, RefreshCw, Save, Search, Trash2, Upload } from "lucide-react";
import type { ChangeEvent } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";

import {
  applyResource,
  deleteResource,
  downloadBadgeIcon,
  downloadPetSpeciesPixa,
  getResource,
  putResource,
  uploadBadgeIcon,
  uploadPetSpeciesPixa,
  type ApplyResult,
  type Badge,
  type PetSpecies,
  type Resource,
  type ResourceKind,
} from "@gizclaw/gizclaw/admin";
import { Badge as BadgePill } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { ErrorBanner, NoticeBanner } from "../../components/banners";
import { DeleteConfirmButton } from "../../components/delete-confirm-button";
import { EmptyState } from "../../components/empty-state";
import { FormField } from "../../components/form-field";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { expectData, toMessage } from "../../components/api";

const resourceKinds: ResourceKind[] = [
  "Credential",
  "ACLPolicyBinding",
  "ACLRole",
  "ACLView",
  "Firmware",
  "Model",
  "DashScopeTenant",
  "GeminiTenant",
  "MiniMaxTenant",
  "OpenAITenant",
  "VolcTenant",
  "Voice",
  "Workflow",
  "Workspace",
  "PeerConfig",
  "PetSpecies",
  "Badge",
  "ResourceList",
];

const assetKinds = new Set<ResourceKind>(["PetSpecies", "Badge"]);

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

export function ResourcesPage(): JSX.Element {
  const [searchParams, setSearchParams] = useSearchParams();
  const [kind, setKind] = useState<ResourceKind>(() => parseResourceKind(searchParams.get("kind")) ?? "ResourceList");
  const [name, setName] = useState(() => searchParams.get("name") ?? "");
  const [resource, setResource] = useState<Resource | null>(null);
  const [resourceText, setResourceText] = useState(() => JSON.stringify(resourceTemplate(kind, name), null, 2));
  const [applyResult, setApplyResult] = useState<ApplyResult | null>(null);
  const [assetResult, setAssetResult] = useState<Badge | PetSpecies | null>(null);
  const [loading, setLoading] = useState(false);
  const [acting, setActing] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const canAddressResource = name.trim() !== "" && kind !== "ResourceList";
  const assetEnabled = assetKinds.has(kind) && name.trim() !== "";

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
    setAssetResult(null);
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
    setAssetResult(null);
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
    setAssetResult(null);
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
    setAssetResult(null);
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
    setAssetResult(null);
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

  const uploadAsset = async (event: ChangeEvent<HTMLInputElement>): Promise<void> => {
    const file = event.target.files?.[0] ?? null;
    event.target.value = "";
    if (file === null) {
      return;
    }
    setActing("asset-upload");
    setError("");
    setNotice("");
    setAssetResult(null);
    try {
      const next =
        kind === "PetSpecies"
          ? await expectData(uploadPetSpeciesPixa({ body: file, path: { id: name.trim() } }))
          : await expectData(uploadBadgeIcon({ body: file, path: { id: name.trim() } }));
      setAssetResult(next);
      setNotice(`Uploaded ${kind === "PetSpecies" ? ".pixa" : "icon"} for ${name.trim()}.`);
      await load();
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  const downloadAsset = async (): Promise<void> => {
    setActing("asset-download");
    setError("");
    setNotice("");
    setAssetResult(null);
    try {
      const blob =
        kind === "PetSpecies"
          ? await expectData(downloadPetSpeciesPixa({ path: { id: name.trim() } }))
          : await expectData(downloadBadgeIcon({ path: { id: name.trim() } }));
      saveBlob(blob, `${name.trim()}${kind === "PetSpecies" ? ".pixa" : ".asset"}`);
      setNotice(`Downloaded ${kind === "PetSpecies" ? ".pixa" : "icon"} for ${name.trim()}.`);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  const selectedSummary = useMemo(() => resourceSummary(kind), [kind]);

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

            {assetKinds.has(kind) ? (
              <FormField description="Assets require an existing PetSpecies or Badge resource." label={kind === "PetSpecies" ? "PIXA asset" : "Badge icon"}>
                <div className="flex flex-wrap gap-2">
                  <Button asChild className="min-w-fit shrink-0 whitespace-nowrap" disabled={!assetEnabled || acting !== ""} type="button" variant="outline">
                    <label>
                      <Upload className="size-4" />
                      Upload
                      <input className="sr-only" disabled={!assetEnabled || acting !== ""} onChange={(event) => void uploadAsset(event)} type="file" />
                    </label>
                  </Button>
                  <Button className="min-w-fit shrink-0 whitespace-nowrap" disabled={!assetEnabled || acting !== ""} onClick={() => void downloadAsset()} type="button" variant="outline">
                    <Download className="size-4" />
                    Download
                  </Button>
                </div>
              </FormField>
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

      {assetResult !== null ? (
        <Card>
          <CardHeader>
            <CardTitle>Asset Result</CardTitle>
            <CardDescription>Resource metadata returned after the asset upload.</CardDescription>
          </CardHeader>
          <CardContent>
            <pre className="max-h-[24rem] overflow-auto rounded-md bg-muted p-4 text-xs leading-5">{JSON.stringify(assetResult, null, 2)}</pre>
          </CardContent>
        </Card>
      ) : resource === null && !loading ? (
        <EmptyState description="Load an existing resource or edit the draft JSON and apply it." title="No resource loaded" />
      ) : null}
    </div>
  );
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
    case "Badge":
    case "PetSpecies":
      return { name: "" };
    case "ResourceList":
      return { items: [] };
    case "PeerConfig":
      return {};
    default:
      return {};
  }
}

function resourceSummary(kind: ResourceKind): string {
  switch (kind) {
    case "ResourceList":
      return "Apply multiple resources in one request. The server rejects get/delete for ResourceList.";
    case "PetSpecies":
      return "Pet species metadata plus .pixa asset upload/download.";
    case "Badge":
      return "Badge metadata plus icon upload/download.";
    case "PeerConfig":
      return "Desired peer configuration keyed by peer public key.";
    default:
      return "Generic declarative resource backed by the admin resource API.";
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function saveBlob(blob: Blob | File, filename: string): void {
  const url = window.URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.URL.revokeObjectURL(url);
}
