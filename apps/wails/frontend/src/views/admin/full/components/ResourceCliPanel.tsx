import { Copy, FileJson } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";

import type { Resource } from "@gizclaw/gizclaw/admin";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

type ResourceCliPanelProps = {
  commands: string;
  resource: Resource | null;
  resourceDescription: string;
  resourceTitle: string;
};

export function ResourceCliPanel({
  commands,
  resource,
  resourceDescription,
  resourceTitle,
}: ResourceCliPanelProps): JSX.Element {
  const [copied, setCopied] = useState(false);
  const editHref = resourceEditHref(resource);

  const copyJSON = async (): Promise<void> => {
    if (resource === null) {
      return;
    }
    await navigator.clipboard.writeText(JSON.stringify(resource, null, 2));
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };

  return (
    <div className="flex flex-col gap-4" data-slot="resource-cli-panel">
      <Card>
        <CardHeader>
          <CardTitle>CLI Commands</CardTitle>
          <CardDescription>
            Declarative admin resources use JSON. Use a separate CLI context
            instead of reusing the UI context.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <pre className="overflow-auto rounded-md bg-muted p-4 text-xs leading-5">
            {commands}
          </pre>
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div className="space-y-1">
            <CardTitle>{resourceTitle}</CardTitle>
            <CardDescription>{resourceDescription}</CardDescription>
          </div>
          <div className="flex flex-wrap justify-end gap-2">
            {editHref !== null ? (
              <Button
                asChild
                className="min-w-fit shrink-0 whitespace-nowrap"
                size="sm"
                variant="outline"
              >
                <Link to={editHref}>
                  <FileJson className="size-4" />
                  Edit Resource
                </Link>
              </Button>
            ) : null}
            <Button
              className="min-w-fit shrink-0 whitespace-nowrap"
              disabled={resource === null}
              onClick={() => void copyJSON()}
              size="sm"
              variant="outline"
            >
              <Copy className="size-4" />
              {copied ? "Copied" : "Copy Spec"}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <pre className="max-h-[36rem] overflow-auto rounded-md bg-muted p-4 text-xs leading-5">
            {JSON.stringify(resource, null, 2)}
          </pre>
        </CardContent>
      </Card>
    </div>
  );
}

function resourceEditHref(resource: Resource | null): string | null {
  if (resource === null) {
    return null;
  }
  const record = resource as unknown as Record<string, unknown>;
  const kind = normalizeResourceKind(record.kind);
  const metadata = record.metadata;
  if (kind === null || typeof metadata !== "object" || metadata === null) {
    return null;
  }
  const name = (metadata as Record<string, unknown>).name;
  if (typeof name !== "string" || name.trim() === "") {
    return null;
  }
  return `/resources?kind=${encodeURIComponent(kind)}&name=${encodeURIComponent(name)}`;
}

function normalizeResourceKind(value: unknown): string | null {
  if (typeof value !== "string" || value.trim() === "") {
    return null;
  }
  const trimmed = value.trim();
  if (trimmed.endsWith("Resource") && trimmed !== "Resource") {
    return trimmed.slice(0, -"Resource".length);
  }
  return trimmed;
}
