import { ChevronLeft, RefreshCw, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import { deleteAclPolicyBinding, getAclPolicyBinding, putAclPolicyBinding, type AclPolicyBinding, type Resource } from "@gizclaw/adminservice";
import { ResourceCliPanel } from "../../components/ResourceCliPanel";
import { expectData, toMessage } from "../../components/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { DateTimeInput } from "../../components/date-time-input";
import { DeleteConfirmButton } from "../../components/delete-confirm-button";
import { DetailBlock } from "../../components/detail-block";
import { EmptyState } from "../../components/empty-state";
import { ErrorBanner } from "../../components/banners";
import { FormField } from "../../components/form-field";
import { Input } from "@/components/ui/input";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  bindingFormFromBinding,
  bindingPayloadFromForm,
  decodeRouteParam,
  resourceKinds,
  shellQuote,
  type PolicyBindingFormState,
} from "./acl-utils";
import type { AclResourceKind, AclSubjectKind } from "@gizclaw/adminservice";

export function ACLPolicyBindingDetailPage(): JSX.Element {
  const navigate = useNavigate();
  const params = useParams();
  const bindingID = useMemo(() => decodeRouteParam(params.id ?? ""), [params.id]);
  const [binding, setBinding] = useState<AclPolicyBinding | null>(null);
  const [form, setForm] = useState<PolicyBindingFormState | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const load = async (): Promise<void> => {
    if (bindingID === "") {
      setError("Missing policy binding ID in the URL.");
      setLoading(false);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const next = await expectData(getAclPolicyBinding({ path: { id: bindingID } }));
      setBinding(next);
      setForm(bindingFormFromBinding(next));
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, [bindingID]);

  const save = async (): Promise<void> => {
    if (form === null) {
      return;
    }
    setSaving(true);
    setError("");
    try {
      const payload = bindingPayloadFromForm(form);
      const updated = await expectData(
        putAclPolicyBinding({
          path: { id: bindingID },
          body: {
            display_order: payload.displayOrder,
            id: payload.id,
            policy: payload.policy,
          },
        }),
      );
      setBinding(updated);
      setForm(bindingFormFromBinding(updated));
      if (updated.id !== bindingID) {
        navigate(`/settings/acl/policy-bindings/${encodeURIComponent(updated.id)}`, { replace: true });
      }
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
      await expectData(deleteAclPolicyBinding({ path: { id: bindingID } }));
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
          { label: bindingID },
        ]}
      />

      <PageSummaryCard
        description="Policy binding details and editable display order."
        eyebrow="Access Control"
        meta={binding ? <Badge variant="secondary">{binding.policy.role}</Badge> : null}
        title={binding?.id ?? bindingID}
      />

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-80 w-full" />
        </div>
      ) : error !== "" ? (
        <ErrorBanner message={error} />
      ) : binding === null || form === null ? (
        <EmptyState description="This policy binding could not be loaded." title="Policy binding not found" />
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
                  ["ID", binding.id],
                  ["Display order", String(binding.display_order)],
                  ["Role", binding.policy.role],
                  ["Created", binding.created_at],
                  ["Updated", binding.updated_at],
                ]}
                title="Binding"
              />
              <DetailBlock
                items={[
                  ["Subject", `${binding.policy.subject.kind}:${binding.policy.subject.id}`],
                  ["Resource", `${binding.policy.resource.kind}:${binding.policy.resource.id}`],
                  ["Not before", binding.policy.not_before],
                  ["Expires at", binding.policy.expires_at],
                ]}
                title="Policy"
              />
            </div>
          </TabsContent>

          <TabsContent className="space-y-4" value="edit">
            <Card>
              <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
                <div className="space-y-1">
                  <CardTitle>Edit policy binding</CardTitle>
                  <CardDescription>Update policy fields and display ordering.</CardDescription>
                </div>
                <DeleteConfirmButton disabled={saving} onConfirm={() => void remove()} size="sm" title="Delete policy binding?">
                  <Trash2 className="size-4" />
                  Delete
                </DeleteConfirmButton>
              </CardHeader>
              <CardContent className="space-y-4">
                <BindingEditForm form={form} onChange={setForm} />
                <div className="flex justify-end gap-2">
                  <Button disabled={saving} onClick={() => setForm(bindingFormFromBinding(binding))} type="button" variant="outline">
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
              commands={bindingCliCommands(binding)}
              resource={bindingResource(binding)}
              resourceDescription="Declarative ACL policy binding resource accepted by admin apply."
              resourceTitle="Policy Binding Resource"
            />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}

function BindingEditForm({ form, onChange }: { form: PolicyBindingFormState; onChange: (form: PolicyBindingFormState) => void }): JSX.Element {
  return (
    <div className="grid gap-3 md:grid-cols-2">
      <FormField label="ID">
        <Input disabled value={form.id} />
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
          </SelectContent>
        </Select>
      </FormField>
      <FormField label="Subject ID">
        <Input onChange={(event) => onChange({ ...form, subjectId: event.target.value })} value={form.subjectId} />
      </FormField>
      <FormField label="Resource kind">
        <Select value={form.resourceKind} onValueChange={(value) => onChange({ ...form, resourceKind: value as AclResourceKind })}>
          <SelectTrigger>
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

function bindingCliCommands(binding: AclPolicyBinding): string {
  const id = shellQuote(binding.id);
  return [`gizclaw admin --context <admin-cli-context> show ACLPolicyBinding ${id}`, `gizclaw admin --context <admin-cli-context> delete ACLPolicyBinding ${id}`].join("\n");
}

function bindingResource(binding: AclPolicyBinding): Resource {
  return {
    apiVersion: "gizclaw.io/v1",
    kind: "ACLPolicyBinding",
    metadata: { name: binding.id },
    spec: binding.policy,
  } as Resource;
}
