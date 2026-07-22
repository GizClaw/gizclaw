import { ChevronLeft, Eye, EyeOff, RefreshCw } from "lucide-react";
import { DashboardTable } from "@/dashboard";
import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";

import {
  getCredential,
  getResource,
  type Credential,
  type Resource,
} from "@gizclaw/gizclaw/admin";
import { expectData, toMessage } from "@/dashboard";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { DetailBlock } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { ErrorBanner } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { Skeleton } from "@/components/ui/skeleton";
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ResourceCliPanel } from "../../components/ResourceCliPanel";

export function CredentialDetailPage(): JSX.Element {
  const params = useParams();
  const credentialName = useMemo(
    () => decodeRouteParam(params.name ?? ""),
    [params.name],
  );
  const [credential, setCredential] = useState<Credential | null>(null);
  const [credentialResource, setCredentialResource] = useState<Resource | null>(
    null,
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [revealed, setRevealed] = useState(false);

  const load = async (): Promise<void> => {
    if (credentialName === "") {
      setLoading(false);
      setError("Missing credential name in the URL.");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const [nextCredential, nextResource] = await Promise.all([
        expectData(getCredential({ path: { name: credentialName } })),
        expectData(
          getResource({ path: { kind: "Credential", name: credentialName } }),
        ),
      ]);
      setCredential(nextCredential);
      setCredentialResource(nextResource);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, [credentialName]);

  if (credentialName === "") {
    return (
      <EmptyState
        description="Missing credential name in the URL."
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
              <Link to="/providers/credentials">
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
              <RefreshCw className="size-4" />
              Reload
            </Button>
          </>
        }
        items={[
          { href: "/overview", label: "Overview" },
          { href: "/providers/credentials", label: "Credentials" },
          { label: credentialName },
        ]}
      />

      <PageSummaryCard
        description="Provider credential details and declarative resource access."
        eyebrow="Providers"
        meta={
          credential ? (
            <Badge variant="secondary">{credential.provider}</Badge>
          ) : null
        }
        title={credential?.name ?? credentialName}
      />

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-80 w-full" />
        </div>
      ) : error !== "" ? (
        <ErrorBanner message={error} />
      ) : credential === null ? (
        <EmptyState
          description="This credential could not be loaded."
          title="Credential not found"
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
                  ["Name", credential.name],
                  ["Provider", credential.provider],
                  ["Auth", credentialAuthSummary(credential)],
                  ["Description", credential.description],
                  ["Created", credential.created_at],
                  ["Updated", credential.updated_at],
                ]}
                title="Credential"
              />
              <DetailBlock
                items={[
                  ["Body keys", String(Object.keys(credential.body).length)],
                  ["Resource kind", "Credential"],
                  ["Resource name", credential.name],
                ]}
                title="Resource Identity"
              />
            </div>

            <Card>
              <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
                <div className="space-y-1">
                  <CardTitle>Credential Body</CardTitle>
                  <CardDescription>
                    Stored authentication fields returned by the admin API.
                  </CardDescription>
                </div>
                <Button
                  className="min-w-fit shrink-0 whitespace-nowrap"
                  onClick={() => setRevealed((value) => !value)}
                  size="sm"
                  type="button"
                  variant="outline"
                >
                  {revealed ? (
                    <EyeOff className="size-4" />
                  ) : (
                    <Eye className="size-4" />
                  )}
                  {revealed ? "Hide values" : "Show values"}
                </Button>
              </CardHeader>
              <CardContent>
                <CredentialBodyTable
                  credential={credential}
                  revealed={revealed}
                />
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent className="space-y-4" value="cli">
            <ResourceCliPanel
              commands={credentialCliCommands(credential)}
              resource={credentialResource}
              resourceDescription="JSON returned by the resource API and accepted by admin apply. Credential body values are included."
              resourceTitle="Credential Resource Spec"
            />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}

function CredentialBodyTable({
  credential,
  revealed,
}: {
  credential: Credential;
  revealed: boolean;
}): JSX.Element {
  const entries = Object.entries(credential.body);
  if (entries.length === 0) {
    return (
      <EmptyState
        description="This credential has an empty body."
        title="No body keys"
      />
    );
  }
  return (
    <DashboardTable>
      <TableHeader>
        <TableRow>
          <TableHead className="w-56">Key</TableHead>
          <TableHead>Value</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {entries.map(([key, value]) => {
          const formatted = formatCredentialBodyValue(value);
          return (
            <TableRow key={key}>
              <TableCell className="font-mono text-xs font-medium">
                {key}
              </TableCell>
              <TableCell className="break-all font-mono text-xs">
                {revealed ? formatted : maskCredentialBodyValue(formatted)}
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </DashboardTable>
  );
}

function credentialCliCommands(credential: Credential): string {
  const name = shellQuote(credential.name);
  return [
    `# Read this credential through the credential CLI`,
    `gizclaw admin credentials --context <admin-cli-context> get ${name}`,
    ``,
    `# Show this declarative credential resource`,
    `gizclaw admin --context <admin-cli-context> show Credential ${name}`,
    ``,
    `# Apply/update from a JSON file`,
    `gizclaw admin --context <admin-cli-context> apply -f credential.json`,
    ``,
    `# Delete this credential resource`,
    `gizclaw admin --context <admin-cli-context> delete Credential ${name}`,
  ].join("\n");
}

function credentialAuthSummary(credential: Credential): string {
  const keys = Object.keys(credential.body).filter((key) => {
    const value = (credential.body as Record<string, unknown>)[key];
    return value !== undefined && value !== "";
  });
  return keys.length === 0 ? "empty body" : keys.join(", ");
}

function decodeRouteParam(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function formatCredentialBodyValue(value: unknown): string {
  if (typeof value === "string") {
    return value;
  }
  if (value == null) {
    return "";
  }
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function maskCredentialBodyValue(value: string): string {
  if (value === "") {
    return "-";
  }
  if (value.length <= 2) {
    return "**";
  }
  if (value.length <= 8) {
    return `${value.slice(0, 1)}****${value.slice(-1)}`;
  }
  return `${value.slice(0, 6)}******${value.slice(-4)}`;
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}
