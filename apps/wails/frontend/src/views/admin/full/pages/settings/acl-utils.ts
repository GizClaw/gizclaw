import type { AclPermission, AclPolicy, AclPolicyBinding, AclResourceKind, AclRole, AclSubjectKind } from "@gizclaw/gizclaw/admin";

export type PolicyBindingFormState = {
  displayOrder: string;
  expiresAt: string;
  id: string;
  notBefore: string;
  resourceId: string;
  resourceKind: AclResourceKind;
  role: string;
  subjectId: string;
  subjectKind: AclSubjectKind;
};

export const resourceKinds: AclResourceKind[] = [
  "workspace",
  "workflow",
  "voice",
  "credential",
  "model",
  "view",
  "firmware",
  "gameruleset",
];

export const commonPermissions: AclPermission[] = [
  "read",
  "use",
  "create",
  "admin",
];

export function bindingFormFromBinding(binding: AclPolicyBinding): PolicyBindingFormState {
  return {
    displayOrder: String(binding.display_order),
    expiresAt: binding.policy.expires_at ?? "",
    id: binding.id,
    notBefore: binding.policy.not_before ?? "",
    resourceId: binding.policy.resource.id,
    resourceKind: binding.policy.resource.kind,
    role: binding.policy.role,
    subjectId: binding.policy.subject.id,
    subjectKind: binding.policy.subject.kind,
  };
}

export function emptyBindingForm(): PolicyBindingFormState {
  return {
    displayOrder: "0",
    expiresAt: "",
    id: "",
    notBefore: "",
    resourceId: "",
    resourceKind: "workspace",
    role: "",
    subjectId: "",
    subjectKind: "pk",
  };
}

export function bindingPayloadFromForm(form: PolicyBindingFormState): { displayOrder: number; id?: string; policy: AclPolicy } {
  const displayOrder = Number(form.displayOrder.trim() === "" ? "0" : form.displayOrder);
  if (!Number.isFinite(displayOrder)) {
    throw new Error("Display order must be a finite number.");
  }
  const id = form.id.trim();
  const subjectId = form.subjectId.trim();
  const resourceId = form.resourceId.trim();
  const role = form.role.trim();
  if (form.subjectKind !== "all_peers" && subjectId === "") {
    throw new Error("Subject ID is required unless subject kind is all_peers.");
  }
  if (resourceId === "" || role === "") {
    throw new Error("Resource and role are required.");
  }
  const policy: AclPolicy = {
    subject: { kind: form.subjectKind, id: subjectId },
    resource: { kind: form.resourceKind, id: resourceId },
    role,
  };
  if (form.notBefore.trim() !== "") {
    policy.not_before = form.notBefore.trim();
  }
  if (form.expiresAt.trim() !== "") {
    policy.expires_at = form.expiresAt.trim();
  }
  return { displayOrder, id: id === "" ? undefined : id, policy };
}

export function permissionsFromText(value: string): AclPermission[] {
  return value
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter((item) => item !== "") as AclPermission[];
}

export function permissionsText(role: AclRole): string {
  return role.permissions.join("\n");
}

export function appendPermissionText(value: string, permission: AclPermission): string {
  const permissions = permissionsFromText(value);
  if (permissions.includes(permission)) {
    return value;
  }
  return [...permissions, permission].join("\n");
}

export function decodeRouteParam(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

export function shellQuote(value: string): string {
  return `'${value.replaceAll("'", "'\\''")}'`;
}
