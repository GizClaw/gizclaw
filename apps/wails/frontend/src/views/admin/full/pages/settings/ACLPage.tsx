import { Check, Copy, Plus, RefreshCw } from "lucide-react";
import type { KeyboardEvent, MouseEvent, ReactNode } from "react";
import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";

import {
  createAclPolicyBinding,
  createAclRole,
  createAclView,
  listAclPolicyBindings,
  listAclRoles,
  listAclViews,
  type AclPolicyBinding,
  type AclPermission,
  type AclResourceKind,
  type AclRole,
  type AclSubjectKind,
  type AclView,
} from "@gizclaw/gizclaw/admin";
import { expectData, toMessage } from "../../components/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { DateTimeInput } from "../../components/date-time-input";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { EmptyState } from "../../components/empty-state";
import { ErrorBanner } from "../../components/banners";
import { FormField } from "../../components/form-field";
import { Input } from "@/components/ui/input";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { appendPermissionText, bindingPayloadFromForm, commonPermissions, emptyBindingForm, permissionsFromText, resourceKinds, type PolicyBindingFormState } from "./acl-utils";

const pageLimit = 50;

type CursorPage<T> = {
  cursor: string | null;
  error: string;
  hasNext: boolean;
  history: Array<string | null>;
  items: T[];
  loading: boolean;
  nextCursor: string | null;
};

type BindingFilters = {
  orderBy: "id" | "display_order";
  permission: string;
  resourceId: string;
  resourceKind: "" | AclResourceKind;
  role: string;
  subjectId: string;
  subjectKind: "" | AclSubjectKind;
};

type ACLPageProps = {
  defaultResourceKind?: "" | AclResourceKind;
};

export function ACLPage({ defaultResourceKind = "" }: ACLPageProps): JSX.Element {
  const navigate = useNavigate();
  const [activeTab, setActiveTab] = useState("bindings");
  const [roles, setRoles] = useState<CursorPage<AclRole>>(initialPage());
  const [views, setViews] = useState<CursorPage<AclView>>(initialPage());
  const [bindings, setBindings] = useState<CursorPage<AclPolicyBinding>>(initialPage());
  const [filters, setFilters] = useState<BindingFilters>(() => emptyBindingFilters(defaultResourceKind));
  const [roleDialogOpen, setRoleDialogOpen] = useState(false);
  const [viewDialogOpen, setViewDialogOpen] = useState(false);
  const [bindingDialogOpen, setBindingDialogOpen] = useState(false);
  const [copiedID, setCopiedID] = useState("");

  const loadRoles = useCallback(async (cursor: string | null, history: Array<string | null>) => {
    setRoles((current) => ({ ...current, error: "", loading: true }));
    try {
      const result = await expectData(listAclRoles({ query: { cursor: cursor ?? undefined, limit: pageLimit } }));
      setRoles({
        cursor,
        error: "",
        hasNext: result.has_next,
        history,
        items: result.items ?? [],
        loading: false,
        nextCursor: result.next_cursor ?? null,
      });
    } catch (error) {
      setRoles((current) => ({ ...current, error: toMessage(error), loading: false }));
    }
  }, []);

  const loadViews = useCallback(async (cursor: string | null, history: Array<string | null>) => {
    setViews((current) => ({ ...current, error: "", loading: true }));
    try {
      const result = await expectData(listAclViews({ query: { cursor: cursor ?? undefined, limit: pageLimit } }));
      setViews({
        cursor,
        error: "",
        hasNext: result.has_next,
        history,
        items: result.items ?? [],
        loading: false,
        nextCursor: result.next_cursor ?? null,
      });
    } catch (error) {
      setViews((current) => ({ ...current, error: toMessage(error), loading: false }));
    }
  }, []);

  const loadBindings = useCallback(
    async (cursor: string | null, history: Array<string | null>, nextFilters: BindingFilters) => {
      setBindings((current) => ({ ...current, error: "", loading: true }));
      try {
        const result = await expectData(
          listAclPolicyBindings({
            query: {
              cursor: cursor ?? undefined,
              limit: pageLimit,
              order_by: nextFilters.orderBy,
              permission: permissionOrUndefined(nextFilters.permission),
              resource_id: valueOrUndefined(nextFilters.resourceId),
              resource_kind: nextFilters.resourceKind || undefined,
              role: valueOrUndefined(nextFilters.role),
              subject_id: valueOrUndefined(nextFilters.subjectId),
              subject_kind: nextFilters.subjectKind || undefined,
            },
          }),
        );
        setBindings({
          cursor,
          error: "",
          hasNext: result.has_next,
          history,
          items: result.items ?? [],
          loading: false,
          nextCursor: result.next_cursor ?? null,
        });
      } catch (error) {
        setBindings((current) => ({ ...current, error: toMessage(error), loading: false }));
      }
    },
    [],
  );

  useEffect(() => {
    void loadRoles(null, []);
  }, [loadRoles]);

  useEffect(() => {
    void loadViews(null, []);
  }, [loadViews]);

  useEffect(() => {
    setFilters((current) => (current.resourceKind === defaultResourceKind ? current : { ...current, resourceKind: defaultResourceKind }));
  }, [defaultResourceKind]);

  useEffect(() => {
    void loadBindings(null, [], filters);
  }, [filters, loadBindings]);

  const refresh = (): void => {
    if (activeTab === "roles") {
      void loadRoles(roles.cursor, roles.history);
      return;
    }
    if (activeTab === "views") {
      void loadViews(views.cursor, views.history);
      return;
    }
    void loadBindings(bindings.cursor, bindings.history, filters);
  };

  const createRole = async (name: string, permissionsTextValue: string): Promise<void> => {
    await expectData(createAclRole({ body: { name: name.trim(), permissions: permissionsFromText(permissionsTextValue) } }));
    setRoleDialogOpen(false);
    await loadRoles(null, []);
  };

  const createView = async (name: string, description: string): Promise<void> => {
    await expectData(
      createAclView({
        body: {
          name: name.trim(),
          description: valueOrUndefined(description),
        },
      }),
    );
    setViewDialogOpen(false);
    await loadViews(null, []);
  };

  const createBinding = async (form: PolicyBindingFormState): Promise<void> => {
    const payload = bindingPayloadFromForm(form);
    await expectData(
      createAclPolicyBinding({
        body: {
          display_order: payload.displayOrder,
          id: payload.id,
          policy: payload.policy,
        },
      }),
    );
    setBindingDialogOpen(false);
    await loadBindings(null, [], filters);
  };

  const copyListID = async (event: MouseEvent<HTMLButtonElement>, id: string): Promise<void> => {
    event.stopPropagation();
    await navigator.clipboard.writeText(id);
    setCopiedID(id);
    window.setTimeout(() => {
      setCopiedID((current) => (current === id ? "" : current));
    }, 1500);
  };

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" onClick={() => setBindingDialogOpen(true)} type="button" variant="outline">
              <Plus className="size-4" />
              New Binding
            </Button>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" onClick={() => setRoleDialogOpen(true)} type="button" variant="outline">
              <Plus className="size-4" />
              New Role
            </Button>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" onClick={() => setViewDialogOpen(true)} type="button" variant="outline">
              <Plus className="size-4" />
              New View
            </Button>
            <Button className="h-8 min-w-fit shrink-0 whitespace-nowrap px-3 text-sm" onClick={refresh} variant="outline">
              <RefreshCw className="size-4" />
              Refresh
            </Button>
          </>
        }
        items={[{ href: "/overview", label: "Overview" }, { label: "Access Control" }]}
      />

      <PageSummaryCard
        description={
          defaultResourceKind === ""
            ? "Roles and policy bindings used by GizClaw admin authorization."
            : `Policy bindings filtered to ${defaultResourceKind} resources.`
        }
        eyebrow="Settings"
        meta={
          <>
            <Badge variant="secondary">{roles.items.length} roles</Badge>
            <Badge variant="secondary">{views.items.length} views</Badge>
            <Badge variant="outline">{bindings.items.length} bindings</Badge>
            {defaultResourceKind !== "" ? <Badge variant="outline">{defaultResourceKind}</Badge> : null}
          </>
        }
        title="Access Control"
      />

      <Tabs onValueChange={setActiveTab} value={activeTab}>
        <TabsList>
          <TabsTrigger value="bindings">Policy Bindings</TabsTrigger>
          <TabsTrigger value="roles">Roles</TabsTrigger>
          <TabsTrigger value="views">Views</TabsTrigger>
        </TabsList>

        <TabsContent value="bindings">
          <Card>
            <CardHeader>
              <CardTitle>Policy bindings</CardTitle>
              <CardDescription>Subject-resource-role bindings sorted by ID or display order.</CardDescription>
              <div className="col-span-full mt-2">
                <BindingFiltersBar filters={filters} onChange={setFilters} />
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {bindings.error !== "" ? <ErrorBanner message={bindings.error} /> : null}
              <Pagination
                hasNext={bindings.hasNext}
                loading={bindings.loading}
                nextPage={() => {
                  if (bindings.nextCursor !== null) {
                    void loadBindings(bindings.nextCursor, [...bindings.history, bindings.cursor], filters);
                  }
                }}
                pageNumber={bindings.history.length + 1}
                prevPage={() => {
                  const previousCursor = bindings.history[bindings.history.length - 1] ?? null;
                  void loadBindings(previousCursor, bindings.history.slice(0, -1), filters);
                }}
              />
              <BindingsTable
                bindings={bindings.items}
                copiedID={copiedID}
                loading={bindings.loading}
                onCopyID={copyListID}
                onOpen={(id) => navigate(`/settings/acl/policy-bindings/${encodeURIComponent(id)}`)}
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="views">
          <Card>
            <CardHeader>
              <CardTitle>Views</CardTitle>
              <CardDescription>Named content views used as ACL subjects and resources.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {views.error !== "" ? <ErrorBanner message={views.error} /> : null}
              <Pagination
                hasNext={views.hasNext}
                loading={views.loading}
                nextPage={() => {
                  if (views.nextCursor !== null) {
                    void loadViews(views.nextCursor, [...views.history, views.cursor]);
                  }
                }}
                pageNumber={views.history.length + 1}
                prevPage={() => {
                  const previousCursor = views.history[views.history.length - 1] ?? null;
                  void loadViews(previousCursor, views.history.slice(0, -1));
                }}
              />
              <ViewsTable
                copiedID={copiedID}
                loading={views.loading}
                onCopyID={copyListID}
                onOpen={(name) => navigate(`/settings/acl/views/${encodeURIComponent(name)}`)}
                views={views.items}
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="roles">
          <Card>
            <CardHeader>
              <CardTitle>Roles</CardTitle>
              <CardDescription>Named permission sets referenced by policy bindings.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {roles.error !== "" ? <ErrorBanner message={roles.error} /> : null}
              <Pagination
                hasNext={roles.hasNext}
                loading={roles.loading}
                nextPage={() => {
                  if (roles.nextCursor !== null) {
                    void loadRoles(roles.nextCursor, [...roles.history, roles.cursor]);
                  }
                }}
                pageNumber={roles.history.length + 1}
                prevPage={() => {
                  const previousCursor = roles.history[roles.history.length - 1] ?? null;
                  void loadRoles(previousCursor, roles.history.slice(0, -1));
                }}
              />
              <RolesTable
                copiedID={copiedID}
                roles={roles.items}
                loading={roles.loading}
                onCopyID={copyListID}
                onOpen={(name) => navigate(`/settings/acl/roles/${encodeURIComponent(name)}`)}
              />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {roleDialogOpen ? <RoleCreateDialog onClose={() => setRoleDialogOpen(false)} onSubmit={createRole} /> : null}
      {viewDialogOpen ? <ViewCreateDialog onClose={() => setViewDialogOpen(false)} onSubmit={createView} /> : null}
      {bindingDialogOpen ? <BindingCreateDialog onClose={() => setBindingDialogOpen(false)} onSubmit={createBinding} /> : null}
    </div>
  );
}

function BindingFiltersBar({ filters, onChange }: { filters: BindingFilters; onChange: (filters: BindingFilters) => void }): JSX.Element {
  return (
    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
      <Select value={filters.orderBy} onValueChange={(value) => onChange({ ...filters, orderBy: value as BindingFilters["orderBy"] })}>
        <SelectTrigger aria-label="Sort policy bindings">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="id">ID order</SelectItem>
          <SelectItem value="display_order">Display order</SelectItem>
        </SelectContent>
      </Select>
      <Input
        aria-label="Subject ID"
        onChange={(event) => onChange({ ...filters, subjectId: event.target.value })}
        placeholder="Subject ID"
        value={filters.subjectId}
      />
      <Select value={filters.resourceKind || "all"} onValueChange={(value) => onChange({ ...filters, resourceKind: value === "all" ? "" : (value as AclResourceKind) })}>
        <SelectTrigger aria-label="Resource kind">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All resources</SelectItem>
          {resourceKinds.map((kind) => (
            <SelectItem key={kind} value={kind}>
              {kind}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Input
        aria-label="Resource ID"
        onChange={(event) => onChange({ ...filters, resourceId: event.target.value })}
        placeholder="Resource ID"
        value={filters.resourceId}
      />
      <Select value={filters.subjectKind || "all"} onValueChange={(value) => onChange({ ...filters, subjectKind: value === "all" ? "" : (value as AclSubjectKind) })}>
        <SelectTrigger aria-label="Subject kind">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All subjects</SelectItem>
          <SelectItem value="pk">Public key</SelectItem>
          <SelectItem value="view">View</SelectItem>
          <SelectItem value="all_peers">All peers</SelectItem>
        </SelectContent>
      </Select>
      <Input aria-label="Role" onChange={(event) => onChange({ ...filters, role: event.target.value })} placeholder="Role" value={filters.role} />
      <Input
        aria-label="Permission"
        onChange={(event) => onChange({ ...filters, permission: event.target.value })}
        placeholder="Permission"
        value={filters.permission}
      />
    </div>
  );
}

function BindingsTable({
  bindings,
  copiedID,
  loading,
  onCopyID,
  onOpen,
}: {
  bindings: AclPolicyBinding[];
  copiedID: string;
  loading: boolean;
  onCopyID: (event: MouseEvent<HTMLButtonElement>, id: string) => Promise<void>;
  onOpen: (id: string) => void;
}): JSX.Element {
  if (loading) {
    return <LoadingRows />;
  }
  if (bindings.length === 0) {
    return <EmptyState description="No policy bindings match the current filters." title="No policy bindings" />;
  }
  return (
    <div className="rounded-md border">
      <Table className="table-fixed">
        <TableHeader>
          <TableRow>
            <TableHead className="w-[28%]">ID</TableHead>
            <TableHead className="text-right">Order</TableHead>
            <TableHead>Subject</TableHead>
            <TableHead>Resource</TableHead>
            <TableHead>Role</TableHead>
            <TableHead className="text-right">Updated</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {bindings.map((binding) => (
            <TableRow
              className="cursor-pointer hover:bg-muted/40"
              key={binding.id}
              onClick={() => onOpen(binding.id)}
              onKeyDown={(event) => rowKeyDown(event, () => onOpen(binding.id))}
              role="link"
              tabIndex={0}
            >
              <TableCell className="min-w-0">
                <div className="flex min-w-0 items-center gap-1.5">
                  <button
                    className="min-w-0 truncate rounded-sm text-left font-mono text-xs font-medium underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    onClick={(event) => {
                      event.stopPropagation();
                      onOpen(binding.id);
                    }}
                    title={binding.id}
                    type="button"
                  >
                    {binding.id}
                  </button>
                  <button
                    aria-label={`Copy policy binding id ${binding.id}`}
                    className="shrink-0 rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    onClick={(event) => void onCopyID(event, binding.id)}
                    title="Copy policy binding id"
                    type="button"
                  >
                    {copiedID === binding.id ? <Check className="size-3 shrink-0 text-emerald-600" /> : <Copy className="size-3 shrink-0" />}
                  </button>
                </div>
              </TableCell>
              <TableCell className="text-right font-mono text-xs">{binding.display_order}</TableCell>
              <TableCell className="font-mono text-xs">
                {binding.policy.subject.kind}:{binding.policy.subject.id}
              </TableCell>
              <TableCell className="font-mono text-xs">
                {binding.policy.resource.kind}:{binding.policy.resource.id}
              </TableCell>
              <TableCell>
                <Badge variant="outline">{binding.policy.role}</Badge>
              </TableCell>
              <TableCell className="text-right text-sm text-muted-foreground">{binding.updated_at}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

function ViewsTable({
  copiedID,
  loading,
  onCopyID,
  onOpen,
  views,
}: {
  copiedID: string;
  loading: boolean;
  onCopyID: (event: MouseEvent<HTMLButtonElement>, id: string) => Promise<void>;
  onOpen: (name: string) => void;
  views: AclView[];
}): JSX.Element {
  if (loading) {
    return <LoadingRows />;
  }
  if (views.length === 0) {
    return <EmptyState description="No ACL views have been created." title="No views" />;
  }
  return (
    <div className="rounded-md border">
      <Table className="table-fixed">
        <TableHeader>
          <TableRow>
            <TableHead className="w-[28%]">View ID</TableHead>
            <TableHead>Description</TableHead>
            <TableHead className="text-right">Updated</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {views.map((view) => (
            <TableRow
              className="cursor-pointer hover:bg-muted/40"
              key={view.name}
              onClick={() => onOpen(view.name)}
              onKeyDown={(event) => rowKeyDown(event, () => onOpen(view.name))}
              role="link"
              tabIndex={0}
            >
              <TableCell className="min-w-0">
                <div className="flex min-w-0 items-center gap-1.5">
                  <button
                    className="min-w-0 truncate rounded-sm text-left font-medium underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    onClick={(event) => {
                      event.stopPropagation();
                      onOpen(view.name);
                    }}
                    title={view.name}
                    type="button"
                  >
                    {view.name}
                  </button>
                  <button
                    aria-label={`Copy ACL view id ${view.name}`}
                    className="shrink-0 rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    onClick={(event) => void onCopyID(event, view.name)}
                    title="Copy ACL view id"
                    type="button"
                  >
                    {copiedID === view.name ? <Check className="size-3 shrink-0 text-emerald-600" /> : <Copy className="size-3 shrink-0" />}
                  </button>
                </div>
              </TableCell>
              <TableCell className="max-w-[28rem] truncate text-sm text-muted-foreground">{view.description ?? "—"}</TableCell>
              <TableCell className="text-right text-sm text-muted-foreground">{view.updated_at}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

function RolesTable({
  copiedID,
  loading,
  onCopyID,
  onOpen,
  roles,
}: {
  copiedID: string;
  loading: boolean;
  onCopyID: (event: MouseEvent<HTMLButtonElement>, id: string) => Promise<void>;
  onOpen: (name: string) => void;
  roles: AclRole[];
}): JSX.Element {
  if (loading) {
    return <LoadingRows />;
  }
  if (roles.length === 0) {
    return <EmptyState description="No ACL roles have been created." title="No roles" />;
  }
  return (
    <div className="rounded-md border">
      <Table className="table-fixed">
        <TableHeader>
          <TableRow>
            <TableHead className="w-[28%]">Role ID</TableHead>
            <TableHead>Permissions</TableHead>
            <TableHead className="text-right">Updated</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {roles.map((role) => (
            <TableRow
              className="cursor-pointer hover:bg-muted/40"
              key={role.name}
              onClick={() => onOpen(role.name)}
              onKeyDown={(event) => rowKeyDown(event, () => onOpen(role.name))}
              role="link"
              tabIndex={0}
            >
              <TableCell className="min-w-0">
                <div className="flex min-w-0 items-center gap-1.5">
                  <button
                    className="min-w-0 truncate rounded-sm text-left font-medium underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    onClick={(event) => {
                      event.stopPropagation();
                      onOpen(role.name);
                    }}
                    title={role.name}
                    type="button"
                  >
                    {role.name}
                  </button>
                  <button
                    aria-label={`Copy ACL role id ${role.name}`}
                    className="shrink-0 rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    onClick={(event) => void onCopyID(event, role.name)}
                    title="Copy ACL role id"
                    type="button"
                  >
                    {copiedID === role.name ? <Check className="size-3 shrink-0 text-emerald-600" /> : <Copy className="size-3 shrink-0" />}
                  </button>
                </div>
              </TableCell>
              <TableCell>
                <div className="flex flex-wrap gap-1">
                  {role.permissions.map((permission) => (
                    <Badge key={permission} variant="secondary">
                      {permission}
                    </Badge>
                  ))}
                </div>
              </TableCell>
              <TableCell className="text-right text-sm text-muted-foreground">{role.updated_at}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

function ViewCreateDialog({
  onClose,
  onSubmit,
}: {
  onClose: () => void;
  onSubmit: (name: string, description: string) => Promise<void>;
}): JSX.Element {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const submit = async (): Promise<void> => {
    setSaving(true);
    setError("");
    try {
      await onSubmit(name, description);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setSaving(false);
    }
  };
  return (
    <ModalFrame onClose={onClose} title="Create ACL View">
      {error !== "" ? <ErrorBanner message={error} /> : null}
      <FormField label="Name">
        <Input onChange={(event) => setName(event.target.value)} value={name} />
      </FormField>
      <FormField label="Description">
        <Textarea
          className="min-h-24"
          onChange={(event) => setDescription(event.target.value)}
          value={description}
        />
      </FormField>
      <DialogActions onClose={onClose} onSubmit={submit} saving={saving} submitLabel="Create" />
    </ModalFrame>
  );
}

function RoleCreateDialog({ onClose, onSubmit }: { onClose: () => void; onSubmit: (name: string, permissions: string) => Promise<void> }): JSX.Element {
  const [name, setName] = useState("");
  const [permissions, setPermissions] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const submit = async (): Promise<void> => {
    setSaving(true);
    setError("");
    try {
      await onSubmit(name, permissions);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setSaving(false);
    }
  };
  return (
    <ModalFrame onClose={onClose} title="Create ACL Role">
      {error !== "" ? <ErrorBanner message={error} /> : null}
      <FormField label="Name">
        <Input onChange={(event) => setName(event.target.value)} value={name} />
      </FormField>
      <FormField description="Use one permission per line or comma-separated values." label="Permissions">
        <Textarea
          className="min-h-32 font-mono"
          onChange={(event) => setPermissions(event.target.value)}
          value={permissions}
        />
        <div className="mt-2 flex flex-wrap gap-2">
          {commonPermissions.map((permission) => (
            <Button key={permission} onClick={() => setPermissions(appendPermissionText(permissions, permission))} size="sm" type="button" variant="outline">
              {permission}
            </Button>
          ))}
        </div>
      </FormField>
      <DialogActions onClose={onClose} onSubmit={submit} saving={saving} submitLabel="Create" />
    </ModalFrame>
  );
}

function BindingCreateDialog({ onClose, onSubmit }: { onClose: () => void; onSubmit: (form: PolicyBindingFormState) => Promise<void> }): JSX.Element {
  const [form, setForm] = useState<PolicyBindingFormState>(emptyBindingForm());
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const submit = async (): Promise<void> => {
    setSaving(true);
    setError("");
    try {
      await onSubmit(form);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setSaving(false);
    }
  };
  return (
    <ModalFrame onClose={onClose} title="Create Policy Binding">
      {error !== "" ? <ErrorBanner message={error} /> : null}
      <BindingForm form={form} onChange={setForm} />
      <DialogActions onClose={onClose} onSubmit={submit} saving={saving} submitLabel="Create" />
    </ModalFrame>
  );
}

function BindingForm({ form, onChange }: { form: PolicyBindingFormState; onChange: (form: PolicyBindingFormState) => void }): JSX.Element {
  return (
    <div className="grid gap-3 md:grid-cols-2">
      <FormField description="Leave empty to generate a short random-prefixed ID." label="ID">
        <Input onChange={(event) => onChange({ ...form, id: event.target.value })} placeholder="Auto-generated when empty" value={form.id} />
      </FormField>
      <FormField label="Display order">
        <Input onChange={(event) => onChange({ ...form, displayOrder: event.target.value })} type="number" value={form.displayOrder} />
      </FormField>
      <FormField label="Subject kind">
        <Select value={form.subjectKind} onValueChange={(value) => onChange({ ...form, subjectKind: value as AclSubjectKind })}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="pk">Public key</SelectItem>
            <SelectItem value="view">View</SelectItem>
            <SelectItem value="all_peers">All peers</SelectItem>
          </SelectContent>
        </Select>
      </FormField>
      <FormField label="Subject ID">
        <Input onChange={(event) => onChange({ ...form, subjectId: event.target.value })} value={form.subjectId} />
      </FormField>
      <FormField label="Resource kind">
        <Select value={form.resourceKind} onValueChange={(value) => onChange({ ...form, resourceKind: value as AclResourceKind })}>
          <SelectTrigger aria-label="Policy binding resource kind">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {resourceKinds.map((kind) => (
              <SelectItem key={kind} value={kind}>
                {kind}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </FormField>
      <FormField label="Resource ID">
        <Input onChange={(event) => onChange({ ...form, resourceId: event.target.value })} value={form.resourceId} />
      </FormField>
      <FormField label="Role">
        <Input onChange={(event) => onChange({ ...form, role: event.target.value })} value={form.role} />
      </FormField>
      <FormField description="Local datetime converted to RFC3339 when saved." label="Not before">
        <DateTimeInput onChange={(value) => onChange({ ...form, notBefore: value })} value={form.notBefore} />
      </FormField>
      <FormField description="Local datetime converted to RFC3339 when saved." label="Expires at">
        <DateTimeInput onChange={(value) => onChange({ ...form, expiresAt: value })} value={form.expiresAt} />
      </FormField>
    </div>
  );
}

function Pagination({
  hasNext,
  loading,
  nextPage,
  pageNumber,
  prevPage,
}: {
  hasNext: boolean;
  loading: boolean;
  nextPage: () => void;
  pageNumber: number;
  prevPage: () => void;
}): JSX.Element {
  return (
    <div className="flex items-center justify-end gap-2">
      <Badge variant="outline">Page {pageNumber}</Badge>
      <Button className="h-8 min-w-fit px-3 text-sm" disabled={loading || pageNumber === 1} onClick={prevPage} type="button" variant="outline">
        Previous
      </Button>
      <Button className="h-8 min-w-fit px-3 text-sm" disabled={loading || !hasNext} onClick={nextPage} type="button" variant="outline">
        Next
      </Button>
    </div>
  );
}

function LoadingRows(): JSX.Element {
  return (
    <div className="space-y-3">
      {Array.from({ length: 6 }).map((_, index) => (
        <Skeleton className="h-14 w-full" key={index} />
      ))}
    </div>
  );
}

function ModalFrame({ children, onClose, title }: { children: ReactNode; onClose: () => void; title: string }): JSX.Element {
  return (
    <Dialog open onOpenChange={(open) => {
      if (!open) {
        onClose();
      }
    }}>
      <DialogContent className="max-h-[90vh] max-w-3xl overflow-auto">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">{children}</div>
      </DialogContent>
    </Dialog>
  );
}

function DialogActions({
  onClose,
  onSubmit,
  saving,
  submitLabel,
}: {
  onClose: () => void;
  onSubmit: () => Promise<void>;
  saving: boolean;
  submitLabel: string;
}): JSX.Element {
  return (
    <div className="flex justify-end gap-2">
      <Button onClick={onClose} type="button" variant="outline">
        Cancel
      </Button>
      <Button disabled={saving} onClick={() => void onSubmit()} type="button">
        {saving ? "Saving" : submitLabel}
      </Button>
    </div>
  );
}

function initialPage<T>(): CursorPage<T> {
  return { cursor: null, error: "", hasNext: false, history: [], items: [], loading: true, nextCursor: null };
}

function emptyBindingFilters(resourceKind: "" | AclResourceKind): BindingFilters {
  return {
    orderBy: "id",
    permission: "",
    resourceId: "",
    resourceKind,
    role: "",
    subjectId: "",
    subjectKind: "",
  };
}

function valueOrUndefined(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

function permissionOrUndefined(value: string): AclPermission | undefined {
  return valueOrUndefined(value) as AclPermission | undefined;
}

function rowKeyDown(event: KeyboardEvent<HTMLTableRowElement>, action: () => void): void {
  if (isInteractiveTarget(event.target)) {
    return;
  }
  if (event.key !== "Enter" && event.key !== " ") {
    return;
  }
  event.preventDefault();
  action();
}

function isInteractiveTarget(target: EventTarget): boolean {
  return target instanceof Element && target.closest("a,button,input,select,textarea") !== null;
}
