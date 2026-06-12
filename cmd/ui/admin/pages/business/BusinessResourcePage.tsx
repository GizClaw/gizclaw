import { ChevronLeft, Download, FileJson, Medal, PawPrint, Plus, RefreshCw, Save, Trash2, Upload } from "lucide-react";
import type { ChangeEvent } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import {
  deleteResource,
  downloadBadgeIcon,
  downloadPetSpeciesPixa,
  getResource,
  listBadges,
  listPetSpecies,
  putResource,
  uploadBadgeIcon,
  uploadPetSpeciesPixa,
  type Badge,
  type BadgeSpec,
  type PetSpecies,
  type PetSpeciesSpec,
  type Resource,
} from "@gizclaw/adminservice";
import { parsePixa, type PixaAsset } from "@gizclaw/pixa";
import { Badge as BadgePill } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";

import { expectData, toMessage } from "../../components/api";
import { ErrorBanner, NoticeBanner } from "../../components/banners";
import { DeleteConfirmButton } from "../../components/delete-confirm-button";
import { DetailBlock } from "../../components/detail-block";
import { EmptyState } from "../../components/empty-state";
import { FormField } from "../../components/form-field";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { useCursorListPage } from "../../hooks/useCursorListPage";
import { formatDate } from "../../lib/format";

type BusinessResourceKind = "PetSpecies" | "Badge";

type BusinessResourceConfig = {
  assetDescription: string;
  assetLabel: string;
  assetName: string;
  collectionPath: string;
  description: string;
  detailPlaceholder: string;
  idLabel: string;
  kind: BusinessResourceKind;
  namePlaceholder: string;
  resourceTitle: string;
  title: string;
};

const petSpeciesConfig: BusinessResourceConfig = {
  assetDescription: "Upload or download the .pixa binary after the PetSpecies resource exists.",
  assetLabel: "PIXA asset",
  assetName: ".pixa",
  collectionPath: "/business/pet-species",
  description: "Admin-managed species metadata used by pet adoption and pet action flows.",
  detailPlaceholder: "rabbit",
  idLabel: "Species ID",
  kind: "PetSpecies",
  namePlaceholder: "Rabbit",
  resourceTitle: "Pet Species Resource",
  title: "Pet Species",
};

const badgeConfig: BusinessResourceConfig = {
  assetDescription: "Upload or download the badge icon after the Badge resource exists.",
  assetLabel: "Icon asset",
  assetName: "icon",
  collectionPath: "/business/badges",
  description: "Admin-managed achievement metadata that reward flows can grant to peers.",
  detailPlaceholder: "daily-checkin",
  idLabel: "Badge ID",
  kind: "Badge",
  namePlaceholder: "Daily Check-in",
  resourceTitle: "Badge Resource",
  title: "Badges",
};

export function PetSpeciesPage(): JSX.Element {
  return <BusinessResourceCollectionPage config={petSpeciesConfig} />;
}

export function PetSpeciesDetailPage(): JSX.Element {
  return <BusinessResourceDetailPage config={petSpeciesConfig} />;
}

export function BadgesPage(): JSX.Element {
  return <BusinessResourceCollectionPage config={badgeConfig} />;
}

export function BadgeDetailPage(): JSX.Element {
  return <BusinessResourceDetailPage config={badgeConfig} />;
}

function BusinessResourceCollectionPage({ config }: { config: BusinessResourceConfig }): JSX.Element {
  const navigate = useNavigate();
  const [listUnavailable, setListUnavailable] = useState(false);
  const { error, hasNext, items, loading, nextPage, pageNumber, prevPage, refresh } = useCursorListPage<Badge | PetSpecies>(async (query) => {
    try {
      if (config.kind === "PetSpecies") {
        const result = await expectData(listPetSpecies({ query }));
        setListUnavailable(false);
        return {
          hasNext: result.has_next,
          items: result.items ?? [],
          nextCursor: result.next_cursor ?? null,
        };
      }
      const result = await expectData(listBadges({ query }));
      setListUnavailable(false);
      return {
        hasNext: result.has_next,
        items: result.items ?? [],
        nextCursor: result.next_cursor ?? null,
      };
    } catch (err) {
      if (isMissingBusinessListEndpoint(err, config.kind)) {
        setListUnavailable(true);
        return {
          hasNext: false,
          items: [],
          nextCursor: null,
        };
      }
      throw err;
    }
  });

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" variant="outline">
              <Link to={`/resources?kind=${encodeURIComponent(config.kind)}`}>
                <Plus className="size-4" />
                New {config.kind === "PetSpecies" ? "Pet Species" : "Badge"}
              </Link>
            </Button>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" onClick={() => void refresh()} variant="outline">
              <RefreshCw className="size-4" />
              Refresh
            </Button>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { label: config.title }]}
      />

      <PageSummaryCard
        description={config.kind === "PetSpecies" ? "Create the metadata first. Upload the .pixa file from the species detail page." : "Create badge metadata first. Upload the icon from the badge detail page."}
        eyebrow="Business"
        meta={
          <>
            <BadgePill variant="secondary">{config.kind}</BadgePill>
            <BadgePill variant="outline">Page {pageNumber}</BadgePill>
            <BadgePill variant="secondary">{items.length} loaded</BadgePill>
          </>
        }
        title={config.title}
      />

      {error !== "" && !listUnavailable ? <ErrorBanner message={error} /> : null}

      <BusinessResourceList
        config={config}
        hasNext={hasNext}
        items={items}
        listUnavailable={listUnavailable}
        loading={loading}
        nextPage={nextPage}
        onOpen={(id) => navigate(resourceDetailPath(config, id))}
        pageNumber={pageNumber}
        prevPage={prevPage}
      />
    </div>
  );
}

function BusinessResourceList({
  config,
  hasNext,
  items,
  listUnavailable,
  loading,
  nextPage,
  onOpen,
  pageNumber,
  prevPage,
}: {
  config: BusinessResourceConfig;
  hasNext: boolean;
  items: Array<Badge | PetSpecies>;
  listUnavailable: boolean;
  loading: boolean;
  nextPage: () => void;
  onOpen: (id: string) => void;
  pageNumber: number;
  prevPage: () => void;
}): JSX.Element {
  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <CardTitle>{config.title} List</CardTitle>
          <CardDescription>{config.kind === "PetSpecies" ? "Open a species detail page to upload or replace its .pixa asset." : "Open a badge detail page to upload or replace its icon asset."}</CardDescription>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <BadgePill variant="outline">Page {pageNumber}</BadgePill>
          <BadgePill variant="secondary">{items.length}</BadgePill>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex justify-end gap-2">
          <Button
            className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm disabled:border-border disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100 disabled:shadow-none"
            disabled={loading || pageNumber === 1}
            onClick={prevPage}
            type="button"
            variant="outline"
          >
            Previous
          </Button>
          <Button
            className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm disabled:border-border disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100 disabled:shadow-none"
            disabled={loading || !hasNext}
            onClick={nextPage}
            type="button"
            variant="outline"
          >
            Next
          </Button>
        </div>

        {loading ? (
          <div className="flex flex-col gap-3">
            {Array.from({ length: 6 }).map((_, index) => (
              <Skeleton className="h-14 w-full" key={index} />
            ))}
          </div>
        ) : listUnavailable ? (
          <EmptyState
            description={`The connected admin service does not expose GET /${config.kind === "PetSpecies" ? "pet-species" : "badges"} yet. Create resources with the New button; the list will populate after the server is updated.`}
            title={`${config.title} list unavailable`}
          />
        ) : items.length === 0 ? (
          <EmptyState description={`Create a ${config.kind} resource to make it appear in this list.`} title={`No ${config.kind} resources`} />
        ) : (
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>Name</TableHead>
                  {config.kind === "Badge" ? <TableHead>Description</TableHead> : null}
                  <TableHead>{config.kind === "PetSpecies" ? "PIXA Path" : "Icon Path"}</TableHead>
                  <TableHead className="text-right">Updated</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((item) => (
                  <TableRow className="cursor-pointer hover:bg-muted/40" key={item.id} onClick={() => onOpen(item.id)}>
                    <TableCell className="max-w-48 font-mono text-xs font-medium">
                      <span className="block truncate">{item.id}</span>
                    </TableCell>
                    <TableCell className="max-w-60">
                      <span className="block truncate font-medium">{item.name}</span>
                    </TableCell>
                    {config.kind === "Badge" ? (
                      <TableCell className="max-w-72">
                        <span className="block truncate">{(item as Badge).description}</span>
                      </TableCell>
                    ) : null}
                    <TableCell className="max-w-72">
                      <span className="block truncate">{config.kind === "PetSpecies" ? (item as PetSpecies).pixa_path : (item as Badge).icon_path}</span>
                    </TableCell>
                    <TableCell className="text-right text-sm text-muted-foreground">{formatDate(item.updated_at)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function BusinessResourceDetailPage({ config }: { config: BusinessResourceConfig }): JSX.Element {
  const navigate = useNavigate();
  const params = useParams();
  const rawID = params.id ?? "";
  const id = useMemo(() => decodeRouteParam(rawID), [rawID]);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [assetPath, setAssetPath] = useState("");
  const [resource, setResource] = useState<Resource | null>(null);
  const [object, setObject] = useState<Badge | PetSpecies | null>(null);
  const [jsonText, setJSONText] = useState(() => JSON.stringify(resourceTemplate(config.kind, id), null, 2));
  const [pixaAsset, setPixaAsset] = useState<PixaAsset | null>(null);
  const [pixaError, setPixaError] = useState("");
  const [loading, setLoading] = useState(false);
  const [acting, setActing] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const load = useCallback(async (): Promise<void> => {
    if (id === "") {
      setError(`Missing ${config.idLabel.toLowerCase()} in route.`);
      return;
    }
    setLoading(true);
    setError("");
    setNotice("");
    try {
      const next = await expectData(getResource({ path: { kind: config.kind, name: id } }));
      setLoadedResource(config.kind, next, setResource, setObject, setName, setDescription, setAssetPath, setJSONText);
    } catch (err) {
      setError(toMessage(err));
      setResource(null);
      setObject(null);
      setJSONText(JSON.stringify(resourceTemplate(config.kind, id), null, 2));
    } finally {
      setLoading(false);
    }
  }, [config.idLabel, config.kind, id]);

  useEffect(() => {
    void load();
  }, [load]);

  const saveFields = async (): Promise<void> => {
    setActing("save-fields");
    setError("");
    setNotice("");
    try {
      const nextResource = resourceFromFields(config.kind, id, name, description, assetPath);
      const next = await expectData(putResource({ body: nextResource, path: { kind: config.kind, name: id } }));
      setLoadedResource(config.kind, next, setResource, setObject, setName, setDescription, setAssetPath, setJSONText);
      setNotice(`Saved ${config.kind} ${id}.`);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  const saveJSON = async (): Promise<void> => {
    setActing("save-json");
    setError("");
    setNotice("");
    try {
      const parsed = parseResourceJSON(config.kind, id, jsonText);
      const next = await expectData(putResource({ body: parsed, path: { kind: config.kind, name: id } }));
      setLoadedResource(config.kind, next, setResource, setObject, setName, setDescription, setAssetPath, setJSONText);
      setNotice(`Saved ${config.kind} ${id}.`);
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
    try {
      await expectData(deleteResource({ path: { kind: config.kind, name: id } }));
      navigate(config.collectionPath);
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
    try {
      let parsedPixa: PixaAsset | null = null;
      if (config.kind === "PetSpecies") {
        try {
          parsedPixa = parsePixa(await file.arrayBuffer());
        } catch (err) {
          setPixaAsset(null);
          setPixaError(toMessage(err));
          setError(toMessage(err));
          return;
        }
      }
      const nextObject =
        config.kind === "PetSpecies"
          ? await expectData(uploadPetSpeciesPixa({ body: file, path: { id } }))
          : await expectData(uploadBadgeIcon({ body: file, path: { id } }));
      setObject(nextObject);
      setPixaAsset(parsedPixa);
      setPixaError("");
      setNotice(`Uploaded ${config.assetName} for ${id}.`);
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
    try {
      const blob =
        config.kind === "PetSpecies"
          ? await expectData(downloadPetSpeciesPixa({ path: { id } }))
          : await expectData(downloadBadgeIcon({ path: { id } }));
      if (config.kind === "PetSpecies") {
        try {
          setPixaAsset(parsePixa(await blob.arrayBuffer()));
          setPixaError("");
        } catch (err) {
          setPixaAsset(null);
          setPixaError(toMessage(err));
        }
      }
      saveBlob(blob, config.kind === "PetSpecies" ? `${id}.pixa` : `${id}-icon`);
      setNotice(`Downloaded ${config.assetName} for ${id}.`);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setActing("");
    }
  };

  const cliCommands = useMemo(() => resourceCommands(config.kind, id), [config.kind, id]);

  if (id === "") {
    return <EmptyState description="Missing resource ID in the URL." title="Invalid route" />;
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        actions={
          <>
            <Button asChild className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" variant="outline">
              <Link to={config.collectionPath}>
                <ChevronLeft className="size-4" />
                Back
              </Link>
            </Button>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" disabled={loading} onClick={() => void load()} variant="outline">
              <RefreshCw className="size-4" />
              Reload
            </Button>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" disabled={acting !== ""} onClick={() => void saveFields()}>
              <Save className="size-4" />
              Save
            </Button>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { href: config.collectionPath, label: config.title }, { label: id }]}
      />

      <PageSummaryCard
        description={config.description}
        eyebrow="Business"
        meta={
          <>
            <BadgePill variant="secondary">{config.kind}</BadgePill>
            {resource === null ? <BadgePill variant="outline">Draft</BadgePill> : <BadgePill variant="outline">Loaded</BadgePill>}
            {assetPath.trim() !== "" ? <BadgePill variant="outline">Asset Path Set</BadgePill> : null}
          </>
        }
        title={id}
      />

      {notice !== "" ? <NoticeBanner message={notice} tone="success" /> : null}
      {error !== "" ? <ErrorBanner message={error} /> : null}

      <div className="grid gap-4 xl:grid-cols-[24rem_minmax(0,1fr)]">
        <AssetCard
          config={config}
          disabled={resource === null || acting !== ""}
          onDownload={() => void downloadAsset()}
          onUpload={(event) => void uploadAsset(event)}
          pixaAsset={pixaAsset}
          pixaError={pixaError}
        />

        <Tabs className="min-w-0" defaultValue="fields">
          <TabsList className="grid h-auto w-full grid-cols-2 lg:w-[20rem]">
            <TabsTrigger value="fields">Fields</TabsTrigger>
            <TabsTrigger value="json">JSON</TabsTrigger>
          </TabsList>

          <TabsContent className="mt-4 flex flex-col gap-4" value="fields">
            <Card>
              <CardHeader>
                <CardTitle>{config.resourceTitle}</CardTitle>
                <CardDescription>Fields map directly to the typed #22 resource spec.</CardDescription>
              </CardHeader>
              <CardContent className="flex flex-col gap-4">
                <FormField label="Name">
                  <Input onChange={(event) => setName(event.target.value)} placeholder={config.namePlaceholder} value={name} />
                </FormField>
                {config.kind === "Badge" ? (
                  <FormField label="Description">
                    <Textarea onChange={(event) => setDescription(event.target.value)} placeholder="Granted by reward claim flow." value={description} />
                  </FormField>
                ) : null}
                <FormField description="Normally set or updated by asset upload; edit only when repairing metadata." label={config.kind === "PetSpecies" ? "pixa_path" : "icon_path"}>
                  <Input onChange={(event) => setAssetPath(event.target.value)} placeholder={config.kind === "PetSpecies" ? `${id}.pixa` : `${id}/icon`} value={assetPath} />
                </FormField>
                <div className="flex flex-wrap justify-end gap-2 border-t pt-4">
                  <Button disabled={acting !== ""} onClick={() => void saveFields()} type="button">
                    <Save className="size-4" />
                    Save
                  </Button>
                  <DeleteConfirmButton disabled={acting !== ""} onConfirm={() => void remove()} title={`Delete ${config.kind}?`}>
                    <Trash2 className="size-4" />
                    Delete
                  </DeleteConfirmButton>
                </div>
              </CardContent>
            </Card>

            {object !== null ? <BusinessObjectDetail kind={config.kind} object={object} /> : <EmptyState description="Load or save this resource before uploading its asset." title={`No ${config.kind} loaded`} />}
          </TabsContent>

          <TabsContent className="mt-4 flex flex-col gap-4" value="json">
            <Card>
              <CardHeader className="flex flex-row items-start justify-between gap-4">
                <div className="flex flex-col gap-1">
                  <CardTitle>Declarative Resource JSON</CardTitle>
                  <CardDescription>Equivalent to `gizclaw admin apply` input for this resource.</CardDescription>
                </div>
                <Button className="min-w-fit shrink-0 whitespace-nowrap" disabled={acting !== ""} onClick={() => void saveJSON()} size="sm" type="button">
                  <FileJson className="size-4" />
                  Save JSON
                </Button>
              </CardHeader>
              <CardContent>
                <Textarea className="min-h-[30rem] resize-y font-mono text-xs leading-5" onChange={(event) => setJSONText(event.target.value)} spellCheck={false} value={jsonText} />
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>CLI Commands</CardTitle>
                <CardDescription>Use a CLI admin context for scripted resource setup.</CardDescription>
              </CardHeader>
              <CardContent>
                <pre className="overflow-auto rounded-md bg-muted p-4 text-xs leading-5">{cliCommands}</pre>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}

function AssetCard({
  config,
  disabled,
  onDownload,
  onUpload,
  pixaAsset,
  pixaError,
}: {
  config: BusinessResourceConfig;
  disabled: boolean;
  onDownload: () => void;
  onUpload: (event: ChangeEvent<HTMLInputElement>) => void;
  pixaAsset: PixaAsset | null;
  pixaError: string;
}): JSX.Element {
  const Icon = config.kind === "PetSpecies" ? PawPrint : Medal;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Icon className="size-5" />
          Asset
        </CardTitle>
        <CardDescription>{config.assetDescription}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <FormField description="The resource must be saved before its asset can be uploaded." label={config.assetLabel}>
          <div className="flex flex-wrap gap-2">
            <Button asChild className="min-w-fit shrink-0 whitespace-nowrap" disabled={disabled} type="button" variant="outline">
              <label>
                <Upload className="size-4" />
                Upload
                <input className="sr-only" disabled={disabled} onChange={onUpload} type="file" />
              </label>
            </Button>
            <Button className="min-w-fit shrink-0 whitespace-nowrap" disabled={disabled} onClick={onDownload} type="button" variant="outline">
              <Download className="size-4" />
              Download
            </Button>
          </div>
        </FormField>
        {config.kind === "PetSpecies" ? <PixaInspection asset={pixaAsset} error={pixaError} /> : null}
      </CardContent>
    </Card>
  );
}

function PixaInspection({ asset, error }: { asset: PixaAsset | null; error: string }): JSX.Element {
  if (error !== "") {
    return <div className="rounded-md border border-destructive/40 p-3 text-sm text-destructive">PIXA parse failed: {error}</div>;
  }
  if (asset === null) {
    return (
      <div className="rounded-md border p-3 text-sm text-muted-foreground">
        Select or download a PIXA asset to inspect its container metadata.
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-3 rounded-md border p-3 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <BadgePill variant="secondary">PIXA v{asset.version}</BadgePill>
        <BadgePill variant="outline">
          {asset.canvas.width} x {asset.canvas.height}
        </BadgePill>
        <BadgePill variant="outline">{asset.frameCount} frames</BadgePill>
      </div>
      <div className="grid gap-2 text-xs sm:grid-cols-2">
        <PixaStat label="Colors" value={String(asset.colorCount)} />
        <PixaStat label="Payload" value={formatByteCount(asset.payloadLength)} />
        <PixaStat label="Palette Offset" value={String(asset.paletteOffset)} />
        <PixaStat label="Clip Offset" value={String(asset.clipOffset)} />
        <PixaStat label="Frame Offset" value={String(asset.frameOffset)} />
        <PixaStat label="Payload Offset" value={String(asset.payloadOffset)} />
      </div>
      <div className="flex flex-col gap-1">
        <span className="text-xs font-medium text-muted-foreground">Clips</span>
        <div className="flex flex-wrap gap-1">
          {asset.clips.length === 0 ? (
            <BadgePill variant="outline">None</BadgePill>
          ) : (
            asset.clips.map((clip) => (
              <BadgePill key={`${clip.name}-${clip.firstFrame}`} variant="outline">
                {clip.name || "(unnamed)"} · {clip.frameCount}f · {clip.totalDurationMs}ms
              </BadgePill>
            ))
          )}
        </div>
      </div>
      <div className="flex flex-col gap-1">
        <span className="text-xs font-medium text-muted-foreground">Frames</span>
        <div className="flex flex-wrap gap-1">
          {asset.frames.slice(0, 6).map((frame, index) => (
            <BadgePill key={index} variant="secondary">
              #{index} {frame.type} {frame.durationMs}ms
            </BadgePill>
          ))}
          {asset.frames.length > 6 ? <BadgePill variant="outline">+{asset.frames.length - 6}</BadgePill> : null}
        </div>
      </div>
    </div>
  );
}

function PixaStat({ label, value }: { label: string; value: string }): JSX.Element {
  return (
    <div className="flex min-w-0 items-center justify-between gap-2 rounded-sm bg-muted px-2 py-1">
      <span className="truncate text-muted-foreground">{label}</span>
      <span className="shrink-0 font-mono">{value}</span>
    </div>
  );
}

function formatByteCount(value: number): string {
  if (value < 1024) {
    return `${value} B`;
  }
  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} KiB`;
  }
  return `${(value / (1024 * 1024)).toFixed(1)} MiB`;
}

function BusinessObjectDetail({ kind, object }: { kind: BusinessResourceKind; object: Badge | PetSpecies }): JSX.Element {
  if (kind === "PetSpecies") {
    const species = object as PetSpecies;
    return (
      <div className="grid gap-4 xl:grid-cols-2">
        <DetailBlock
          items={[
            ["ID", species.id],
            ["Name", species.name],
            ["PIXA Path", species.pixa_path],
            ["Created", formatDate(species.created_at)],
            ["Updated", formatDate(species.updated_at)],
          ]}
          title="Species"
        />
        <DetailBlock
          items={[
            ["Version", String(species.pixa_metadata?.version ?? "")],
            ["Canvas", `${species.pixa_metadata?.canvas_width ?? "?"} x ${species.pixa_metadata?.canvas_height ?? "?"}`],
            ["Colors", String(species.pixa_metadata?.color_count ?? "")],
            ["Frames", String(species.pixa_metadata?.frame_count ?? "")],
            ["Payload", String(species.pixa_metadata?.payload_bytes ?? "")],
            ["Clips", species.pixa_metadata?.clip_names?.join(", ")],
          ]}
          title="PIXA Metadata"
        />
      </div>
    );
  }
  const badge = object as Badge;
  return (
    <DetailBlock
      items={[
        ["ID", badge.id],
        ["Name", badge.name],
        ["Description", badge.description],
        ["Icon Path", badge.icon_path],
        ["Created", formatDate(badge.created_at)],
        ["Updated", formatDate(badge.updated_at)],
      ]}
      title="Badge"
    />
  );
}

function setLoadedResource(
  kind: BusinessResourceKind,
  next: Resource,
  setResource: (resource: Resource) => void,
  setObject: (object: Badge | PetSpecies | null) => void,
  setName: (value: string) => void,
  setDescription: (value: string) => void,
  setAssetPath: (value: string) => void,
  setJSONText: (value: string) => void,
): void {
  setResource(next);
  setJSONText(JSON.stringify(next, null, 2));
  const spec = resourceSpec(next);
  if (kind === "PetSpecies") {
    const petSpec = spec as Partial<PetSpeciesSpec>;
    setName(petSpec.name ?? "");
    setDescription("");
    setAssetPath(petSpec.pixa_path ?? "");
    setObject(resourceObject(next, "PetSpecies"));
    return;
  }
  const badgeSpec = spec as Partial<BadgeSpec>;
  setName(badgeSpec.name ?? "");
  setDescription(badgeSpec.description ?? "");
  setAssetPath(badgeSpec.icon_path ?? "");
  setObject(resourceObject(next, "Badge"));
}

function resourceFromFields(kind: BusinessResourceKind, id: string, name: string, description: string, assetPath: string): Resource {
  const spec =
    kind === "PetSpecies"
      ? {
          name: name.trim(),
          ...(assetPath.trim() === "" ? {} : { pixa_path: assetPath.trim() }),
        }
      : {
          name: name.trim(),
          description: description.trim(),
          ...(assetPath.trim() === "" ? {} : { icon_path: assetPath.trim() }),
        };
  return {
    apiVersion: "gizclaw.admin/v1alpha1",
    kind,
    metadata: { name: id },
    spec,
  } as Resource;
}

function resourceTemplate(kind: BusinessResourceKind, id: string): Resource {
  return resourceFromFields(kind, id, "", "", "");
}

function parseResourceJSON(kind: BusinessResourceKind, id: string, text: string): Resource {
  const parsed = JSON.parse(text) as unknown;
  if (!isRecord(parsed)) {
    throw new Error("Resource JSON must be an object.");
  }
  if (parsed.kind !== kind) {
    throw new Error(`Resource kind must be ${kind}.`);
  }
  const metadata = parsed.metadata;
  if (!isRecord(metadata) || metadata.name !== id) {
    throw new Error(`Resource metadata.name must be ${id}.`);
  }
  return parsed as Resource;
}

function resourceSpec(resource: Resource): unknown {
  return (resource as unknown as { spec?: unknown }).spec ?? {};
}

function resourceObject(resource: Resource, kind: BusinessResourceKind): Badge | PetSpecies | null {
  const value = (resource as unknown as { status?: unknown; object?: unknown }).status ?? (resource as unknown as { object?: unknown }).object;
  if (isRecord(value)) {
    return value as Badge | PetSpecies;
  }
  const metadata = (resource as unknown as { metadata?: unknown }).metadata;
  const spec = resourceSpec(resource);
  if (!isRecord(metadata) || !isRecord(spec)) {
    return null;
  }
  const id = typeof metadata.name === "string" ? metadata.name : "";
  if (kind === "PetSpecies") {
    return {
      id,
      name: typeof spec.name === "string" ? spec.name : "",
      pixa_path: typeof spec.pixa_path === "string" ? spec.pixa_path : "",
      pixa_metadata: { canvas_height: 0, canvas_width: 0, clip_count: 0, clip_names: [], color_count: 0, frame_count: 0, payload_bytes: 0, version: 0 },
      created_at: "",
      updated_at: "",
    };
  }
  return {
    id,
    name: typeof spec.name === "string" ? spec.name : "",
    description: typeof spec.description === "string" ? spec.description : "",
    icon_path: typeof spec.icon_path === "string" ? spec.icon_path : "",
    created_at: "",
    updated_at: "",
  };
}

function resourceCommands(kind: BusinessResourceKind, id: string): string {
  const name = id === "" ? `<${kind === "PetSpecies" ? "species-id" : "badge-id"}>` : shellQuote(id);
  const assetCommand =
    kind === "PetSpecies"
      ? `gizclaw admin --context <admin-cli-context> pet-species upload-pixa ${name} --file species.pixa`
      : `gizclaw admin --context <admin-cli-context> badges upload-icon ${name} --file icon.png`;
  return [
    `# Show the declarative resource`,
    `gizclaw admin --context <admin-cli-context> show ${kind} ${name}`,
    ``,
    `# Create or update metadata`,
    `gizclaw admin --context <admin-cli-context> apply -f ${kind === "PetSpecies" ? "pet-species" : "badge"}.json`,
    ``,
    `# Upload the binary asset from the detail page equivalent`,
    assetCommand,
  ].join("\n");
}

function resourceDetailPath(config: BusinessResourceConfig, id: string): string {
  return `${config.collectionPath}/${encodeURIComponent(id)}`;
}

function isMissingBusinessListEndpoint(error: unknown, kind: BusinessResourceKind): boolean {
  const message = toMessage(error);
  const path = kind === "PetSpecies" ? "/pet-species" : "/badges";
  return message.includes(`Cannot GET ${path}`);
}

function decodeRouteParam(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
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

function shellQuote(value: string): string {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
