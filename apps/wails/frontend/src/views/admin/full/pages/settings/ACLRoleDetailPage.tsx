import { ChevronLeft, RefreshCw, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import { deleteAclRole, getAclRole, putAclRole, type AclRole, type Resource } from "@gizclaw/gizclaw/admin";
import { ResourceCliPanel } from "../../components/ResourceCliPanel";
import { expectData, toMessage } from "../../components/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { DeleteConfirmButton } from "../../components/delete-confirm-button";
import { DetailBlock } from "../../components/detail-block";
import { EmptyState } from "../../components/empty-state";
import { ErrorBanner } from "../../components/banners";
import { FormField } from "../../components/form-field";
import { Input } from "@/components/ui/input";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { appendPermissionText, commonPermissions, decodeRouteParam, permissionsFromText, permissionsText, shellQuote } from "./acl-utils";

export function ACLRoleDetailPage(): JSX.Element {
  const navigate = useNavigate();
  const params = useParams();
  const roleName = useMemo(() => decodeRouteParam(params.name ?? ""), [params.name]);
  const [role, setRole] = useState<AclRole | null>(null);
  const [permissions, setPermissions] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const load = async (): Promise<void> => {
    if (roleName === "") {
      setError("Missing ACL role name in the URL.");
      setLoading(false);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const next = await expectData(getAclRole({ path: { name: roleName } }));
      setRole(next);
      setPermissions(permissionsText(next));
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, [roleName]);

  const save = async (): Promise<void> => {
    setSaving(true);
    setError("");
    try {
      const updated = await expectData(
        putAclRole({
          path: { name: roleName },
          body: {
            name: roleName,
            permissions: permissionsFromText(permissions),
          },
        }),
      );
      setRole(updated);
      setPermissions(permissionsText(updated));
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const remove = async (): Promise<void> => {
    setSaving(true);
    setError("");
    try {
      await expectData(deleteAclRole({ path: { name: roleName } }));
      navigate("/settings/acl");
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild size="sm" variant="outline">
              <Link to="/settings/acl">
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
          { href: "/settings/acl", label: "Access Control" },
          { label: roleName },
        ]}
      />

      <PageSummaryCard
        description="ACL role details and editable permissions."
        eyebrow="Access Control"
        meta={role ? <Badge variant="secondary">{role.permissions.length} permissions</Badge> : null}
        title={role?.name ?? roleName}
      />

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-80 w-full" />
        </div>
      ) : error !== "" ? (
        <ErrorBanner message={error} />
      ) : role === null ? (
        <EmptyState description="This ACL role could not be loaded." title="ACL role not found" />
      ) : (
        <Tabs defaultValue="summary">
          <TabsList>
            <TabsTrigger value="summary">Summary</TabsTrigger>
            <TabsTrigger value="edit">Edit</TabsTrigger>
            <TabsTrigger value="cli">CLI</TabsTrigger>
          </TabsList>

          <TabsContent className="space-y-4" value="summary">
            <DetailBlock
              items={[
                ["Name", role.name],
                ["Permissions", String(role.permissions.length)],
                ["Created", role.created_at],
                ["Updated", role.updated_at],
              ]}
              title="Role"
            />
            <Card>
              <CardHeader>
                <CardTitle>Permissions</CardTitle>
                <CardDescription>Permission strings granted by this role.</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="flex flex-wrap gap-2">
                  {role.permissions.map((permission) => (
                    <Badge key={permission} variant="secondary">
                      {permission}
                    </Badge>
                  ))}
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent className="space-y-4" value="edit">
            <Card>
              <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
                <div className="space-y-1">
                  <CardTitle>Edit role</CardTitle>
                  <CardDescription>Update the permission list referenced by policy bindings.</CardDescription>
                </div>
                <DeleteConfirmButton disabled={saving} onConfirm={() => void remove()} size="sm" title="Delete ACL role?">
                  <Trash2 className="size-4" />
                  Delete
                </DeleteConfirmButton>
              </CardHeader>
              <CardContent className="space-y-4">
                <FormField label="Name">
                  <Input disabled value={role.name} />
                </FormField>
                <FormField description="Use one permission per line or comma-separated values." label="Permissions">
                  <Textarea
                    className="min-h-48 font-mono"
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
                <div className="flex justify-end gap-2">
                  <Button disabled={saving} onClick={() => setPermissions(permissionsText(role))} type="button" variant="outline">
                    Reset
                  </Button>
                  <Button disabled={saving} onClick={() => void save()} type="button">
                    {saving ? "Saving" : "Save"}
                  </Button>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent className="space-y-4" value="cli">
            <ResourceCliPanel
              commands={roleCliCommands(role)}
              resource={roleResource(role)}
              resourceDescription="Declarative ACL role resource accepted by admin apply."
              resourceTitle="ACL Role Resource"
            />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}

function roleCliCommands(role: AclRole): string {
  const name = shellQuote(role.name);
  return [`gizclaw admin --context <admin-cli-context> show ACLRole ${name}`, `gizclaw admin --context <admin-cli-context> delete ACLRole ${name}`].join("\n");
}

function roleResource(role: AclRole): Resource {
  return {
    apiVersion: "gizclaw.io/v1",
    kind: "ACLRole",
    metadata: { name: role.name },
    spec: { permissions: role.permissions },
  } as Resource;
}
