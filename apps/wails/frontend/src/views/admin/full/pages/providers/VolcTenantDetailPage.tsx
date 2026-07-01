import { ChevronLeft, RefreshCw, Save } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";

import { getResource, getVolcTenant, listCredentials, putVolcTenant, syncVolcTenantVoices, type Credential, type Resource, type VolcTenant } from "@gizclaw/gizclaw/admin";
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

type VolcTenantForm = {
  credentialName: string;
  description: string;
  endpoint: string;
  region: string;
  resourceIDs: string;
};

export function VolcTenantDetailPage(): JSX.Element {
  const params = useParams();
  const tenantName = useMemo(() => decodeRouteParam(params.name ?? ""), [params.name]);
  const [tenant, setTenant] = useState<VolcTenant | null>(null);
  const [tenantResource, setTenantResource] = useState<Resource | null>(null);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [form, setForm] = useState<VolcTenantForm>(() => emptyForm());
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const load = async (): Promise<void> => {
    if (tenantName === "") {
      setLoading(false);
      setError("Missing Volcengine tenant name in the URL.");
      return;
    }
    setLoading(true);
    setError("");
    setNotice("");
    try {
      const [nextTenant, nextResource, credentialList] = await Promise.all([
        expectData(getVolcTenant({ path: { name: tenantName } })),
        expectData(getResource({ path: { kind: "VolcTenant", name: tenantName } })),
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
        putVolcTenant({
          body: {
            name: tenant.name,
            credential_name: form.credentialName.trim(),
            region: optionalString(form.region),
            endpoint: optionalString(form.endpoint),
            resource_ids: optionalStringList(form.resourceIDs),
            description: optionalString(form.description),
          },
          path: { name: tenant.name },
        }),
      );
      setTenant(updated);
      setForm(formFromTenant(updated));
      setTenantResource(await expectData(getResource({ path: { kind: "VolcTenant", name: updated.name } })));
      setNotice("Volcengine tenant saved.");
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
      const result = await expectData(syncVolcTenantVoices({ path: { name: tenant.name } }));
      await load();
      setNotice(`Synced voices: ${result.created_count} created, ${result.updated_count} updated, ${result.deleted_count} deleted.`);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setSyncing(false);
    }
  };

  if (tenantName === "") {
    return <EmptyState description="Missing Volcengine tenant name in the URL." title="Invalid route" />;
  }

  const credentialOptions = mergeCredentialOptions(credentials, form.credentialName);

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild size="sm" variant="outline">
              <Link to="/providers/volc-tenants">
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
          { href: "/providers/volc-tenants", label: "Volcengine Tenants" },
          { label: tenantName },
        ]}
      />

      <PageSummaryCard
        description="Volcengine speech tenant configuration and voice sync controls."
        eyebrow="Providers"
        meta={tenant ? <Badge variant="secondary">Volcengine</Badge> : null}
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
        <EmptyState description="This Volcengine tenant could not be loaded." title="Volcengine tenant not found" />
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
                  ["Last sync", tenant.last_synced_at],
                ]}
                title="Tenant"
              />
              <DetailBlock
                items={[
                  ["Region", tenant.region],
                  ["Endpoint", tenant.endpoint],
                  ["Resource IDs", tenant.resource_ids?.join(", ")],
                  ["Created", tenant.created_at],
                  ["Updated", tenant.updated_at],
                ]}
                title="Volcengine"
              />
            </div>
          </TabsContent>

          <TabsContent value="edit">
            <Card>
              <CardHeader>
                <CardTitle>Edit Volcengine Tenant</CardTitle>
                <CardDescription>Update tenant credential binding and speech sync options. The tenant name is the resource identity and is not editable here.</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid gap-4 lg:grid-cols-2">
                  <FormField description="Resource identity. Rename via resource replacement if needed." label="Name">
                    <Input disabled value={tenant.name} />
                  </FormField>
                  <FormField description="Stored credential used when syncing Volcengine voices." label="Credential">
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
                  <FormField description="Optional Volcengine region override." label="Region">
                    <Input onChange={(event) => setForm((current) => ({ ...current, region: event.target.value }))} placeholder="cn-north-1" value={form.region} />
                  </FormField>
                  <FormField description="Optional Volcengine endpoint override." label="Endpoint">
                    <Input onChange={(event) => setForm((current) => ({ ...current, endpoint: event.target.value }))} placeholder="https://..." value={form.endpoint} />
                  </FormField>
                  <FormField description="Comma or newline separated ResourceIDs for purchased or cloned voices. Leave empty to sync public timbres only." label="Resource IDs">
                    <Textarea
                      className="min-h-24 font-mono"
                      onChange={(event) => setForm((current) => ({ ...current, resourceIDs: event.target.value }))}
                      placeholder={"seed-tts-2.0"}
                      value={form.resourceIDs}
                    />
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
              commands={volcTenantCliCommands(tenant)}
              resource={tenantResource}
              resourceDescription="JSON returned by the resource API and accepted by admin apply. Voice synchronization remains a separate CLI action."
              resourceTitle="VolcTenant Resource Spec"
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

function emptyForm(): VolcTenantForm {
  return { credentialName: "", description: "", endpoint: "", region: "", resourceIDs: "" };
}

function formFromTenant(tenant: VolcTenant): VolcTenantForm {
  return {
    credentialName: tenant.credential_name,
    description: tenant.description ?? "",
    endpoint: tenant.endpoint ?? "",
    region: tenant.region ?? "",
    resourceIDs: tenant.resource_ids?.join("\n") ?? "",
  };
}

function optionalString(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

function optionalStringList(value: string): string[] | undefined {
  const items = value
    .split(/[,\n]/)
    .map((item) => item.trim())
    .filter((item) => item !== "");
  return items.length === 0 ? undefined : items;
}

function mergeCredentialOptions(credentials: Credential[], currentName: string): Credential[] {
  if (currentName === "" || credentials.some((credential) => credential.name === currentName)) {
    return credentials;
  }
  return [{ name: currentName, provider: "unknown", body: {}, created_at: "", updated_at: "" }, ...credentials];
}

function volcTenantCliCommands(tenant: VolcTenant): string {
  const name = shellQuote(tenant.name);
  return [
    `# Read this tenant through the Volcengine tenant CLI`,
    `gizclaw admin volc-tenants --context <admin-cli-context> get ${name}`,
    ``,
    `# Re-sync voices from this Volcengine tenant`,
    `gizclaw admin volc-tenants --context <admin-cli-context> sync-voices ${name}`,
    ``,
    `# Show this declarative tenant resource`,
    `gizclaw admin --context <admin-cli-context> show VolcTenant ${name}`,
    ``,
    `# Apply/update from a JSON file`,
    `gizclaw admin --context <admin-cli-context> apply -f volc-tenant.json`,
    ``,
    `# Delete this tenant resource`,
    `gizclaw admin --context <admin-cli-context> delete VolcTenant ${name}`,
  ].join("\n");
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}
