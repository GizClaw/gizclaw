import { ChevronLeft, Plus, RefreshCw } from "lucide-react";
import { DashboardPager } from "@/dashboard";
import { DashboardTable } from "@/dashboard";
import type { KeyboardEvent } from "react";
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import {
  getDashScopeTenant,
  getGeminiTenant,
  getOpenAiTenant,
  getResource,
  listDashScopeTenants,
  listGeminiTenants,
  listOpenAiTenants,
  type DashScopeTenant,
  type GeminiTenant,
  type OpenAiTenant,
  type Resource,
  type ResourceKind,
} from "@gizclaw/gizclaw/admin";
import { expectData, toMessage } from "@/dashboard";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { DetailBlock } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { ErrorBanner } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { ResourceCliPanel } from "../../components/ResourceCliPanel";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useDashboardCursorPage as useCursorListPage } from "@/dashboard";
import { formatDate, formatValue } from "../../lib/format";

type ProviderTenant = OpenAiTenant | GeminiTenant | DashScopeTenant;

type ProviderTenantConfig<T extends ProviderTenant> = {
  detailFields(tenant: T): Array<[string, string | undefined]>;
  emptyTitle: string;
  eyebrow: string;
  get(name: string): Promise<T>;
  kindBadge: string;
  list(query: { cursor?: string; limit?: number }): Promise<{ has_next: boolean; items: T[]; next_cursor?: string | null }>;
  listDescription: string;
  name: string;
  resourceKind: ResourceKind;
  routeBase: string;
  summaryDescription: string;
  title: string;
};

const openAIConfig: ProviderTenantConfig<OpenAiTenant> = {
  detailFields: (tenant) => [
    ["Name", tenant.name],
    ["Endpoint kind", tenant.kind],
    ["API mode", tenant.api_mode],
    ["Credential", tenant.credential_name],
    ["Base URL", tenant.base_url],
    ["Description", tenant.description],
    ["Created", tenant.created_at],
    ["Updated", tenant.updated_at],
  ],
  emptyTitle: "No OpenAI-compatible tenants",
  eyebrow: "Providers",
  get: async (name) => expectData(getOpenAiTenant({ path: { name } })),
  kindBadge: "OpenAI-compatible",
  list: async (query) => expectData(listOpenAiTenants({ query })),
  listDescription: "OpenAI-compatible runtime tenants used by model and voice provider bindings.",
  name: "OpenAI Tenant",
  resourceKind: "OpenAITenant",
  routeBase: "/providers/openai-tenants",
  summaryDescription: "OpenAI-compatible endpoint configuration and credential binding.",
  title: "OpenAI Tenants",
};

const geminiConfig: ProviderTenantConfig<GeminiTenant> = {
  detailFields: (tenant) => [
    ["Name", tenant.name],
    ["Credential", tenant.credential_name],
    ["Project ID", tenant.project_id],
    ["Location", tenant.location],
    ["Base URL", tenant.base_url],
    ["Description", tenant.description],
    ["Created", tenant.created_at],
    ["Updated", tenant.updated_at],
  ],
  emptyTitle: "No Gemini tenants",
  eyebrow: "Providers",
  get: async (name) => expectData(getGeminiTenant({ path: { name } })),
  kindBadge: "Gemini",
  list: async (query) => expectData(listGeminiTenants({ query })),
  listDescription: "Gemini tenant records that bind provider credentials to Gemini project settings.",
  name: "Gemini Tenant",
  resourceKind: "GeminiTenant",
  routeBase: "/providers/gemini-tenants",
  summaryDescription: "Gemini project and credential binding.",
  title: "Gemini Tenants",
};

const dashScopeConfig: ProviderTenantConfig<DashScopeTenant> = {
  detailFields: (tenant) => [
    ["Name", tenant.name],
    ["Credential", tenant.credential_name],
    ["Base URL", tenant.base_url],
    ["Description", tenant.description],
    ["Created", tenant.created_at],
    ["Updated", tenant.updated_at],
  ],
  emptyTitle: "No DashScope tenants",
  eyebrow: "Providers",
  get: async (name) => expectData(getDashScopeTenant({ path: { name } })),
  kindBadge: "DashScope",
  list: async (query) => expectData(listDashScopeTenants({ query })),
  listDescription: "DashScope tenant records that bind credentials to Aliyun-compatible provider endpoints.",
  name: "DashScope Tenant",
  resourceKind: "DashScopeTenant",
  routeBase: "/providers/dashscope-tenants",
  summaryDescription: "DashScope endpoint and credential binding.",
  title: "DashScope Tenants",
};

export function OpenAITenantsListPage(): JSX.Element {
  return <ProviderTenantsListPage config={openAIConfig} />;
}

export function OpenAITenantDetailPage(): JSX.Element {
  return <ProviderTenantDetailPage config={openAIConfig} />;
}

export function GeminiTenantsListPage(): JSX.Element {
  return <ProviderTenantsListPage config={geminiConfig} />;
}

export function GeminiTenantDetailPage(): JSX.Element {
  return <ProviderTenantDetailPage config={geminiConfig} />;
}

export function DashScopeTenantsListPage(): JSX.Element {
  return <ProviderTenantsListPage config={dashScopeConfig} />;
}

export function DashScopeTenantDetailPage(): JSX.Element {
  return <ProviderTenantDetailPage config={dashScopeConfig} />;
}

function ProviderTenantsListPage<T extends ProviderTenant>({ config }: { config: ProviderTenantConfig<T> }): JSX.Element {
  const navigate = useNavigate();
  const { error, hasNext, items, loading, nextPage, pageNumber, prevPage, refresh } = useCursorListPage<T>(async (query) => {
    const result = await config.list(query);
    return {
      hasNext: result.has_next,
      items: result.items ?? [],
      nextCursor: result.next_cursor ?? null,
    };
  });

  const openTenant = (name: string): void => {
    navigate(`${config.routeBase}/${encodeURIComponent(name)}`);
  };

  const handleRowKeyDown = (event: KeyboardEvent<HTMLTableRowElement>, name: string): void => {
    if (isInteractiveTarget(event.target)) {
      return;
    }
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    openTenant(name);
  };

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" variant="outline">
              <Link to={`/resources?kind=${config.resourceKind}`}>
                <Plus className="size-4" />
                New {config.name}
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
        description={config.listDescription}
        eyebrow={config.eyebrow}
        meta={
          <>
            <Badge variant="outline">Page {pageNumber}</Badge>
            <Badge variant="secondary">{items.length} loaded</Badge>
            {hasNext ? <Badge variant="outline">More Available</Badge> : null}
          </>
        }
        title={config.title}
      />

      {error !== "" ? <ErrorBanner message={error} /> : null}

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>Tenant catalog</CardTitle>
            <CardDescription>{config.listDescription}</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex justify-end">
            <DashboardPager canNext={hasNext} canPrevious={pageNumber > 1} loading={loading} onNext={nextPage} onPrevious={prevPage} onRefresh={() => void refresh()} pageIndex={pageNumber} />
          </div>

          {loading ? (
            <div className="space-y-3">
              {Array.from({ length: 6 }).map((_, index) => (
                <Skeleton className="h-14 w-full" key={index} />
              ))}
            </div>
          ) : items.length === 0 ? (
            <EmptyState description="Tenant records will appear here after they are created." title={config.emptyTitle} />
          ) : (
            <DashboardTable>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Credential</TableHead>
                    <TableHead>Endpoint</TableHead>
                    <TableHead>Description</TableHead>
                    <TableHead className="text-right">Updated</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((tenant) => (
                    <TableRow
                      className="cursor-pointer hover:bg-muted/40"
                      key={tenant.name}
                      onClick={() => openTenant(tenant.name)}
                      onKeyDown={(event) => handleRowKeyDown(event, tenant.name)}
                      role="link"
                      tabIndex={0}
                    >
                      <TableCell className="font-medium">{tenant.name}</TableCell>
                      <TableCell>{tenant.credential_name}</TableCell>
                      <TableCell className="max-w-[18rem] truncate text-sm text-muted-foreground">{formatValue(tenant.base_url)}</TableCell>
                      <TableCell className="max-w-[24rem] truncate text-sm text-muted-foreground">{formatValue(tenant.description)}</TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground">{formatDate(tenant.updated_at)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </DashboardTable>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function ProviderTenantDetailPage<T extends ProviderTenant>({ config }: { config: ProviderTenantConfig<T> }): JSX.Element {
  const params = useParams();
  const tenantName = useMemo(() => decodeRouteParam(params.name ?? ""), [params.name]);
  const [tenant, setTenant] = useState<T | null>(null);
  const [resource, setResource] = useState<Resource | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = async (): Promise<void> => {
    if (tenantName === "") {
      setLoading(false);
      setError(`Missing ${config.name} name in the URL.`);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const [nextTenant, nextResource] = await Promise.all([
        config.get(tenantName),
        expectData(getResource({ path: { kind: config.resourceKind, name: tenantName } })),
      ]);
      setTenant(nextTenant);
      setResource(nextResource);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, [tenantName]);

  if (tenantName === "") {
    return <EmptyState description={`Missing ${config.name} name in the URL.`} title="Invalid route" />;
  }

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild size="sm" variant="outline">
              <Link to={config.routeBase}>
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
          { href: config.routeBase, label: config.title },
          { label: tenantName },
        ]}
      />

      <PageSummaryCard
        description={config.summaryDescription}
        eyebrow={config.eyebrow}
        meta={tenant ? <Badge variant="secondary">{config.kindBadge}</Badge> : null}
        title={tenant?.name ?? tenantName}
      />

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-80 w-full" />
        </div>
      ) : error !== "" ? (
        <ErrorBanner message={error} />
      ) : tenant === null ? (
        <EmptyState description={`This ${config.name} could not be loaded.`} title={`${config.name} not found`} />
      ) : (
        <Tabs defaultValue="summary">
          <TabsList>
            <TabsTrigger value="summary">Summary</TabsTrigger>
            <TabsTrigger value="cli">CLI</TabsTrigger>
          </TabsList>

          <TabsContent className="space-y-4" value="summary">
            <DetailBlock items={config.detailFields(tenant)} title={config.name} />
          </TabsContent>

          <TabsContent className="space-y-4" value="cli">
            <ResourceCliPanel
              commands={tenantCliCommands(config, tenant)}
              resource={resource}
              resourceDescription="JSON returned by the resource API and accepted by admin apply."
              resourceTitle={`${config.resourceKind} Resource Spec`}
            />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}

function tenantCliCommands(config: ProviderTenantConfig<ProviderTenant>, tenant: ProviderTenant): string {
  const name = shellQuote(tenant.name);
  return [
    `# Read this tenant through the provider CLI`,
    `gizclaw admin ${cliTenantCommand(config.resourceKind)} --context <admin-cli-context> get ${name}`,
    ``,
    `# Show this declarative tenant resource`,
    `gizclaw admin --context <admin-cli-context> show ${config.resourceKind} ${name}`,
    ``,
    `# Apply/update from a JSON file`,
    `gizclaw admin --context <admin-cli-context> apply -f tenant.json`,
    ``,
    `# Delete this tenant resource`,
    `gizclaw admin --context <admin-cli-context> delete ${config.resourceKind} ${name}`,
  ].join("\n");
}

function cliTenantCommand(kind: ResourceKind): string {
  switch (kind) {
    case "OpenAITenant":
      return "openai-tenants";
    case "GeminiTenant":
      return "gemini-tenants";
    case "DashScopeTenant":
      return "dashscope-tenants";
    default:
      return "show";
  }
}

function decodeRouteParam(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}

function isInteractiveTarget(target: EventTarget): boolean {
  return target instanceof Element && target.closest("a,button,input,select,textarea") !== null;
}
