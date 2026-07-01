import { ChevronLeft, RefreshCw } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";

import { getModel, getResource, type Model, type Resource } from "@gizclaw/gizclaw/admin";
import { expectData, toMessage } from "../../components/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DetailBlock } from "../../components/detail-block";
import { EmptyState } from "../../components/empty-state";
import { ErrorBanner } from "../../components/banners";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { ResourceCliPanel } from "../../components/ResourceCliPanel";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

export function ModelDetailPage(): JSX.Element {
  const params = useParams();
  const modelID = useMemo(() => decodeRouteParam(params.id ?? ""), [params.id]);
  const [model, setModel] = useState<Model | null>(null);
  const [resource, setResource] = useState<Resource | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = async (): Promise<void> => {
    if (modelID === "") {
      setLoading(false);
      setError("Missing model ID in the URL.");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const [nextModel, nextResource] = await Promise.all([
        expectData(getModel({ path: { id: modelID } })),
        expectData(getResource({ path: { kind: "Model", name: modelID } })),
      ]);
      setModel(nextModel);
      setResource(nextResource);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, [modelID]);

  if (modelID === "") {
    return <EmptyState description="Missing model ID in the URL." title="Invalid route" />;
  }

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild size="sm" variant="outline">
              <Link to="/ai/models">
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
        items={[
          { href: "/overview", label: "Overview" },
          { href: "/ai/models", label: "Models" },
          { label: compactModelID(modelID) },
        ]}
      />

      <PageSummaryCard
        description={<span className="break-all font-mono text-xs">{modelID}</span>}
        eyebrow="AI"
        meta={model ? <Badge variant={model.source === "sync" ? "secondary" : "outline"}>{model.source}</Badge> : null}
        title={model?.name?.trim() || compactModelID(modelID)}
      />

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-80 w-full" />
        </div>
      ) : error !== "" ? (
        <ErrorBanner message={error} />
      ) : model === null ? (
        <EmptyState description="This model could not be loaded." title="Model not found" />
      ) : (
        <Tabs defaultValue="summary">
          <TabsList>
            <TabsTrigger value="summary">Summary</TabsTrigger>
            <TabsTrigger value="cli">CLI</TabsTrigger>
          </TabsList>

          <TabsContent className="space-y-4" value="summary">
            <div className="grid gap-4 xl:grid-cols-2">
              <DetailBlock
                items={[
                  ["Internal ID", model.id],
                  ["Kind", model.kind],
                  ["Source", model.source],
                  ["Provider", `${model.provider.kind}/${model.provider.name}`],
                  ["Name", model.name],
                  ["Description", model.description],
                ]}
                title="Model"
              />
              <DetailBlock
                items={[
                  ["JSON output", boolText(model.capabilities?.json_output)],
                  ["Tool calls", boolText(model.capabilities?.tool_calls)],
                  ["Text only", boolText(model.capabilities?.text_only)],
                  ["System role", boolText(model.capabilities?.system_role)],
                  ["Temperature", boolText(model.capabilities?.temperature)],
                  ["Thinking", boolText(model.capabilities?.thinking?.supported)],
                  ["Synced at", model.synced_at],
                  ["Created", model.created_at],
                  ["Updated", model.updated_at],
                ]}
                title="Capabilities"
              />
            </div>
          </TabsContent>

          <TabsContent className="space-y-4" value="cli">
            <ResourceCliPanel
              commands={modelCliCommands(model)}
              resource={resource}
              resourceDescription="JSON returned by the resource API and accepted by admin apply."
              resourceTitle="Model Resource Spec"
            />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}

function decodeRouteParam(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function compactModelID(id: string): string {
  const trimmed = id.trim();
  if (trimmed === "") {
    return "Model";
  }
  const parts = trimmed.split(":");
  return parts[parts.length - 1] ?? trimmed;
}

function boolText(value: boolean | undefined): string | undefined {
  if (value === undefined) {
    return undefined;
  }
  return value ? "yes" : "no";
}

function modelCliCommands(model: Model): string {
  const id = shellQuote(model.id);
  return [
    `# Read this model through the model CLI`,
    `gizclaw admin models --context <admin-cli-context> get ${id}`,
    ``,
    `# Show this declarative model resource`,
    `gizclaw admin --context <admin-cli-context> show Model ${id}`,
    ``,
    `# Apply/update from a JSON file`,
    `gizclaw admin --context <admin-cli-context> apply -f model.json`,
    ``,
    `# Delete this model resource`,
    `gizclaw admin --context <admin-cli-context> delete Model ${id}`,
  ].join("\n");
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}
