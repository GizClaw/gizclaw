import { ChevronLeft, RefreshCw } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";

import {
  getResource,
  getVoice,
  type Resource,
  type Voice,
} from "@gizclaw/gizclaw/admin";
import { expectData, toMessage } from "@/dashboard";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DetailBlock } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { ErrorBanner } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ResourceCliPanel } from "../../components/ResourceCliPanel";

export function VoiceDetailPage(): JSX.Element {
  const params = useParams();
  const voiceID = useMemo(() => decodeRouteParam(params.id ?? ""), [params.id]);
  const [voice, setVoice] = useState<Voice | null>(null);
  const [voiceResource, setVoiceResource] = useState<Resource | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = async (): Promise<void> => {
    if (voiceID === "") {
      setLoading(false);
      setError("Missing voice ID in the URL.");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const [nextVoice, nextResource] = await Promise.all([
        expectData(getVoice({ path: { id: voiceID } })),
        expectData(getResource({ path: { kind: "Voice", name: voiceID } })),
      ]);
      setVoice(nextVoice);
      setVoiceResource(nextResource);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, [voiceID]);

  if (voiceID === "") {
    return (
      <EmptyState
        description="Missing voice ID in the URL."
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
              <Link to="/ai/voices">
                <ChevronLeft className="size-4" />
                Back to list
              </Link>
            </Button>
            <Button
              className="min-w-fit shrink-0 whitespace-nowrap"
              onClick={() => void load()}
              size="sm"
              variant="outline"
            >
              <span className="inline-flex items-center gap-2 whitespace-nowrap">
                <RefreshCw className="size-4" />
                Reload
              </span>
            </Button>
          </>
        }
        items={[
          { href: "/overview", label: "Overview" },
          { href: "/ai/voices", label: "Voices" },
          { label: compactVoiceID(voiceID) },
        ]}
      />

      <PageSummaryCard
        description={
          <span className="break-all font-mono text-xs">{voiceID}</span>
        }
        eyebrow="AI"
        meta={
          voice ? (
            <Badge variant={voice.source === "sync" ? "secondary" : "outline"}>
              {voice.source}
            </Badge>
          ) : null
        }
        title={voice?.name?.trim() || compactVoiceID(voiceID)}
      />

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-80 w-full" />
        </div>
      ) : error !== "" ? (
        <ErrorBanner message={error} />
      ) : voice === null ? (
        <EmptyState
          description="This voice could not be loaded."
          title="Voice not found"
        />
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
                  ["Internal ID", voice.id],
                  ["Name", voice.name],
                  ["Description", voice.description],
                  ["Source", voice.source],
                  ["Provider", providerDisplayText(voice)],
                  ["Provider kind", voice.provider.kind],
                ]}
                title="Voice"
              />
              <DetailBlock
                items={[
                  ["Provider voice ID", providerDataString(voice, "voice_id")],
                  ["Resource ID", providerDataString(voice, "resource_id")],
                  ["State", providerDataString(voice, "state")],
                  ["Status", providerDataString(voice, "status")],
                  ["Synced at", voice.synced_at],
                  ["Created", voice.created_at],
                  ["Updated", voice.updated_at],
                ]}
                title="Provider Data"
              />
            </div>
          </TabsContent>

          <TabsContent className="space-y-4" value="cli">
            <ResourceCliPanel
              commands={cliCommands(voice)}
              resource={voiceResource}
              resourceDescription={voiceResourceDescription(voice)}
              resourceTitle="Voice Resource Spec"
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

function compactVoiceID(id: string): string {
  const trimmed = id.trim();
  if (trimmed === "") {
    return "Voice";
  }
  const parts = trimmed.split(":");
  return parts[parts.length - 1] ?? trimmed;
}

function providerDataString(voice: Voice, key: string): string | undefined {
  const providerData = voice.provider_data;
  if (
    typeof providerData !== "object" ||
    providerData === null ||
    Array.isArray(providerData)
  ) {
    return undefined;
  }
  const value = (providerData as Record<string, unknown>)[key];
  return typeof value === "string" && value.trim() !== "" ? value : undefined;
}

function providerDisplayText(voice: Voice): string {
  return `${providerPrefix(voice.provider.kind)}/${voice.provider.name}`;
}

function providerPrefix(kind: string): string {
  switch (kind) {
    case "minimax-tenant":
      return "minimax";
    case "volc-tenant":
      return "volc";
    default:
      return kind;
  }
}

function cliCommands(voice: Voice): string {
  const id = shellQuote(voice.id);
  const commands = [
    `# Show this voice resource`,
    `gizclaw admin --context <admin-cli-context> show Voice ${id}`,
  ];
  if (voice.source === "sync") {
    commands.push(
      ``,
      `# Synced voices are read-only; update them by syncing the provider tenant.`,
    );
  } else {
    commands.push(
      ``,
      `# Apply/update from a JSON file`,
      `gizclaw admin --context <admin-cli-context> apply -f voice.json`,
    );
  }
  commands.push(
    ``,
    `# Delete this voice resource`,
    `gizclaw admin --context <admin-cli-context> delete Voice ${id}`,
  );
  if (voice.provider.kind === "volc-tenant") {
    const tenantName = shellQuote(voice.provider.name);
    commands.push(
      ``,
      `# Show the Volcengine tenant that owns this voice`,
      `gizclaw admin --context <admin-cli-context> show VolcTenant ${tenantName}`,
      ``,
      `# Re-sync voices from this Volcengine tenant`,
      `gizclaw admin volc-tenants --context <admin-cli-context> sync-voices ${tenantName}`,
    );
  }
  return commands.join("\n");
}

function voiceResourceDescription(voice: Voice): string {
  if (voice.source === "sync") {
    return "JSON returned by the resource API. Synced voice resources are read-only; update them by syncing the provider tenant.";
  }
  return "JSON returned by the resource API and accepted by admin apply. The resource metadata name is the voice ID.";
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}
