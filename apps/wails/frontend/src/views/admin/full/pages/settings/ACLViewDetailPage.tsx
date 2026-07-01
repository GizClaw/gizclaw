import { ChevronLeft, RefreshCw, Save, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import { deleteAclView, getAclView, getResource, putAclView, type AclView, type Resource } from "@gizclaw/gizclaw/admin";
import { expectData, toMessage } from "@/dashboard";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { DashboardDeleteButton as DeleteConfirmButton } from "@/dashboard";
import { DetailBlock } from "@/dashboard";
import { EmptyState } from "@/dashboard";
import { ErrorBanner, NoticeBanner } from "@/dashboard";
import { FormField } from "@/dashboard";
import { Input } from "@/components/ui/input";
import { PageHeader, PageSummaryCard } from "@/dashboard";
import { ResourceCliPanel } from "../../components/ResourceCliPanel";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { decodeRouteParam, shellQuote } from "./acl-utils";

type ViewForm = {
  description: string;
};

export function ACLViewDetailPage(): JSX.Element {
  const params = useParams();
  const navigate = useNavigate();
  const viewName = useMemo(() => decodeRouteParam(params.name ?? ""), [params.name]);
  const [view, setView] = useState<AclView | null>(null);
  const [resource, setResource] = useState<Resource | null>(null);
  const [form, setForm] = useState<ViewForm>(() => ({ description: "" }));
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const load = async (): Promise<void> => {
    if (viewName === "") {
      setLoading(false);
      setError("Missing ACL view name in the URL.");
      return;
    }
    setLoading(true);
    setError("");
    setNotice("");
    try {
      const [nextView, nextResource] = await Promise.all([
        expectData(getAclView({ path: { name: viewName } })),
        expectData(getResource({ path: { kind: "ACLView", name: viewName } })),
      ]);
      setView(nextView);
      setResource(nextResource);
      setForm({ description: nextView.description ?? "" });
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, [viewName]);

  const save = async (): Promise<void> => {
    if (view === null) {
      return;
    }
    setSaving(true);
    setError("");
    setNotice("");
    try {
      const updated = await expectData(
        putAclView({
          body: {
            name: view.name,
            description: optionalString(form.description),
          },
          path: { name: view.name },
        }),
      );
      setView(updated);
      setForm({ description: updated.description ?? "" });
      setResource(await expectData(getResource({ path: { kind: "ACLView", name: updated.name } })));
      setNotice("ACL view saved.");
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const remove = async (): Promise<void> => {
    if (view === null) {
      return;
    }
    setDeleting(true);
    setError("");
    try {
      await expectData(deleteAclView({ path: { name: view.name } }));
      navigate("/settings/acl");
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setDeleting(false);
    }
  };

  if (viewName === "") {
    return <EmptyState description="Missing ACL view name in the URL." title="Invalid route" />;
  }

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <>
            <Button asChild size="sm" variant="outline">
              <Link to="/settings/acl">
                <ChevronLeft className="size-4" />
                Back to ACL
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
          { label: viewName },
        ]}
      />

      <PageSummaryCard description="Named content view available to ACL subjects and resources." eyebrow="Settings" title={view?.name ?? viewName} />

      {notice !== "" ? <NoticeBanner message={notice} tone="success" /> : null}
      {error !== "" ? <ErrorBanner message={error} /> : null}

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-80 w-full" />
        </div>
      ) : view === null ? (
        <EmptyState description="This ACL view could not be loaded." title="ACL view not found" />
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
                ["Name", view.name],
                ["Description", view.description],
                ["Created", view.created_at],
                ["Updated", view.updated_at],
              ]}
              title="ACL View"
            />
          </TabsContent>

          <TabsContent value="edit">
            <Card>
              <CardHeader>
                <CardTitle>Edit ACL View</CardTitle>
                <CardDescription>Update the operator-facing description.</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <FormField description="Resource identity. Rename via resource replacement if needed." label="Name">
                  <Input disabled value={view.name} />
                </FormField>
                <FormField label="Description">
                  <Textarea
                    className="min-h-24"
                    onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))}
                    value={form.description}
                  />
                </FormField>
                <div className="flex justify-between border-t pt-4">
                  <DeleteConfirmButton disabled={deleting} onConfirm={() => void remove()} title="Delete ACL view?">
                    <Trash2 className="size-4" />
                    {deleting ? "Deleting..." : "Delete"}
                  </DeleteConfirmButton>
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
              commands={viewCliCommands(view)}
              resource={resource}
              resourceDescription="JSON returned by the resource API and accepted by admin apply."
              resourceTitle="ACLView Resource Spec"
            />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}

function optionalString(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

function viewCliCommands(view: AclView): string {
  const name = shellQuote(view.name);
  return [
    `# Read this ACL view through the ACL CLI`,
    `gizclaw admin acl --context <admin-cli-context> views get ${name}`,
    ``,
    `# Show this declarative view resource`,
    `gizclaw admin --context <admin-cli-context> show ACLView ${name}`,
    ``,
    `# Apply/update from a JSON file`,
    `gizclaw admin --context <admin-cli-context> apply -f acl-view.json`,
    ``,
    `# Delete this ACL view resource`,
    `gizclaw admin --context <admin-cli-context> delete ACLView ${name}`,
  ].join("\n");
}
