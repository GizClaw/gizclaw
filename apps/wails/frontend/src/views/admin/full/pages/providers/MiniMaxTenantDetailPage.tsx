import { ChevronLeft, RefreshCw, Save } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";

import {
  getResource,
  getMiniMaxTenant,
  listCredentials,
  putMiniMaxTenant,
  syncMiniMaxTenantVoices,
  type Credential,
  type MiniMaxTenant,
  type Resource,
} from "@gizclaw/gizclaw/admin";
import { expectData, toMessage } from "@/dashboard";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { DetailBlock } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { ErrorBanner, NoticeBanner } from "@/dashboard";
import { FormField } from "@/dashboard";
import { Input } from "@/components/ui/input";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { ResourceCliPanel } from "../../components/ResourceCliPanel";

type MiniMaxTenantForm = {
  appID: string;
  baseURL: string;
  credentialName: string;
  description: string;
  groupID: string;
};

export function MiniMaxTenantDetailPage(): JSX.Element {
  const params = useParams();
  const tenantName = useMemo(() => decodeRouteParam(params.name ?? ""), [params.name]);
  const [tenant, setTenant] = useState<MiniMaxTenant | null>(null);
  const [tenantResource, setTenantResource] = useState<Resource | null>(null);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [form, setForm] = useState<MiniMaxTenantForm>(() => emptyForm());
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const load = async (): Promise<void> => {
    if (tenantName === "") {
      setLoading(false);
      setError("Missing MiniMax tenant name in the URL.");
      return;
    }
    setLoading(true);
    setError("");
    setNotice("");
    try {
      const [nextTenant, nextResource, credentialList] = await Promise.all([
        expectData(getMiniMaxTenant({ path: { name: tenantName } })),
        expectData(getResource({ path: { kind: "MiniMaxTenant", name: tenantName } })),
        expectData(listCredentials({ query: { limit: 200 } })),
      ]);
      setTenant(nextTenant);
      setTenantResource(nextResource);
      setForm(formFromTenant(nextTenant));
      setCredentials(credentialList.items ?? []);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, [tenantName]);

  const save = async (): Promise<void> => {
    if (tenant === null) {
      return;
    }
    setSaving(true);
    setError("");
    setNotice("");
    try {
      const updated = await expectData(
        putMiniMaxTenant({
          body: {
            name: tenant.name,
            app_id: form.appID.trim(),
            group_id: form.groupID.trim(),
            credential_name: form.credentialName.trim(),
            base_url: optionalString(form.baseURL),
            description: optionalString(form.description),
          },
          path: { name: tenant.name },
        }),
      );
      setTenant(updated);
      setForm(formFromTenant(updated));
      setTenantResource(await expectData(getResource({ path: { kind: "MiniMaxTenant", name: updated.name } })));
      setNotice("MiniMax tenant saved.");
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const syncVoices = async (): Promise<void> => {
    if (tenant === null) {
      return;
    }
    setSyncing(true);
    setError("");
    setNotice("");
    try {
      const result = await expectData(syncMiniMaxTenantVoices({ path: { name: tenant.name } }));
      await load();
      setNotice(`Synced voices: ${result.created_count} created, ${result.updated_count} updated, ${result.deleted_count} deleted.`);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setSyncing(false);
    }
  };

  if (tenantName === "") {
    return <EmptyState description="Missing MiniMax tenant name in the URL." title="Invalid route" />;
  }

  const credentialOptions = mergeCredentialOptions(credentials, form.credentialName);

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild size="sm" variant="outline">
              <Link to="/providers/minimax-tenants">
                <ChevronLeft className="size-4" />
                Back to list
              </Link>
            </Button>
            <Button className="min-w-fit shrink-0 whitespace-nowrap" onClick={() => void load()} size="sm" variant="outline">
              <RefreshCw className="size-4" />
              Reload
            </Button>
            <Button className="min-w-fit shrink-0 whitespace-nowrap" disabled={tenant === null || syncing} onClick={() => void syncVoices()} size="sm" variant="outline">
              <RefreshCw className={`size-4 ${syncing ? "animate-spin" : ""}`} />
              Sync voices
            </Button>
          </>
        }
        items={[
          { href: "/overview", label: "Overview" },
          { href: "/providers/minimax-tenants", label: "MiniMax Tenants" },
          { label: tenantName },
        ]}
      />

      <PageSummaryCard
        description="MiniMax tenant configuration and voice sync controls."
        eyebrow="Providers"
        meta={tenant ? <Badge variant="secondary">MiniMax</Badge> : null}
        title={tenant?.name ?? tenantName}
      />

      {notice !== "" ? <NoticeBanner message={notice} tone="success" /> : null}
      {error !== "" ? <ErrorBanner message={error} /> : null}

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-80 w-full" />
        </div>
      ) : tenant === null ? (
        <EmptyState description="This MiniMax tenant could not be loaded." title="MiniMax tenant not found" />
      ) : (
        <Tabs defaultValue="summary">
          <TabsList>
            <TabsTrigger value="summary">Summary</TabsTrigger>
            <TabsTrigger value="edit">Edit</TabsTrigger>
            <TabsTrigger value="cli">CLI</TabsTrigger>
          </TabsList>

          <TabsContent className="space-y-4" value="summary">
            <div className="grid gap-4 xl:grid-cols-2">
              <DetailBlock
                items={[
                  ["Name", tenant.name],
                  ["Credential", tenant.credential_name],
                  ["Description", tenant.description],
                  ["Base URL", tenant.base_url],
                ]}
                title="Tenant"
              />
              <DetailBlock
                items={[
                  ["App ID", tenant.app_id],
                  ["Group ID", tenant.group_id],
                  ["Last sync", tenant.last_synced_at],
                  ["Created", tenant.created_at],
                  ["Updated", tenant.updated_at],
                ]}
                title="MiniMax"
              />
            </div>
          </TabsContent>

          <TabsContent value="edit">
            <Card>
              <CardHeader>
                <CardTitle>Edit MiniMax Tenant</CardTitle>
                <CardDescription>Update tenant routing and credential binding. The tenant name is the resource identity and is not editable here.</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid gap-4 lg:grid-cols-2">
                  <FormField description="Resource identity. Rename via resource replacement if needed." label="Name">
                    <Input disabled value={tenant.name} />
                  </FormField>
                  <FormField description="Stored credential used when syncing MiniMax voices." label="Credential">
                    <Select disabled={saving || credentialOptions.length === 0} onValueChange={(value) => setForm((current) => ({ ...current, credentialName: value }))} value={form.credentialName}>
                      <SelectTrigger>
                        <SelectValue placeholder="Select credential" />
                      </SelectTrigger>
                      <SelectContent>
                        {credentialOptions.map((credential) => (
                          <SelectItem key={credential.name} value={credential.name}>
                            {credential.name} · {credential.provider}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </FormField>
                  <FormField description="MiniMax app identifier." label="App ID">
                    <Input onChange={(event) => setForm((current) => ({ ...current, appID: event.target.value }))} value={form.appID} />
                  </FormField>
                  <FormField description="MiniMax group identifier." label="Group ID">
                    <Input onChange={(event) => setForm((current) => ({ ...current, groupID: event.target.value }))} value={form.groupID} />
                  </FormField>
                  <FormField description="Optional MiniMax API base URL override." label="Base URL">
                    <Input onChange={(event) => setForm((current) => ({ ...current, baseURL: event.target.value }))} placeholder="https://api.minimax.io" value={form.baseURL} />
                  </FormField>
                  <FormField description="Human-readable note for operators." label="Description">
                    <Textarea
                      className="min-h-24"
                      onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))}
                      value={form.description}
                    />
                  </FormField>
                </div>
                <div className="flex justify-end border-t pt-4">
                  <Button disabled={saving} onClick={() => void save()} type="button">
                    <Save className="size-4" />
                    {saving ? "Saving..." : "Save"}
                  </Button>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent className="space-y-4" value="cli">
            <ResourceCliPanel
              commands={miniMaxTenantCliCommands(tenant)}
              resource={tenantResource}
              resourceDescription="JSON returned by the resource API and accepted by admin apply. Voice synchronization remains a separate CLI action."
              resourceTitle="MiniMaxTenant Resource Spec"
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

function emptyForm(): MiniMaxTenantForm {
  return { appID: "", baseURL: "", credentialName: "", description: "", groupID: "" };
}

function formFromTenant(tenant: MiniMaxTenant): MiniMaxTenantForm {
  return {
    appID: tenant.app_id,
    baseURL: tenant.base_url ?? "",
    credentialName: tenant.credential_name,
    description: tenant.description ?? "",
    groupID: tenant.group_id,
  };
}

function optionalString(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

function mergeCredentialOptions(credentials: Credential[], currentName: string): Credential[] {
  if (currentName === "" || credentials.some((credential) => credential.name === currentName)) {
    return credentials;
  }
  return [{ name: currentName, provider: "unknown", body: {}, created_at: "", updated_at: "" }, ...credentials];
}

function miniMaxTenantCliCommands(tenant: MiniMaxTenant): string {
  const name = shellQuote(tenant.name);
  return [
    `# Read this tenant through the MiniMax tenant CLI`,
    `gizclaw admin minimax-tenants --context <admin-cli-context> get ${name}`,
    ``,
    `# Re-sync voices from this MiniMax tenant`,
    `gizclaw admin minimax-tenants --context <admin-cli-context> sync-voices ${name}`,
    ``,
    `# Show this declarative tenant resource`,
    `gizclaw admin --context <admin-cli-context> show MiniMaxTenant ${name}`,
    ``,
    `# Apply/update from a JSON file`,
    `gizclaw admin --context <admin-cli-context> apply -f minimax-tenant.json`,
    ``,
    `# Delete this tenant resource`,
    `gizclaw admin --context <admin-cli-context> delete MiniMaxTenant ${name}`,
  ].join("\n");
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}
