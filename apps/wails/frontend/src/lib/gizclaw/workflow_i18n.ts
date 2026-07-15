export interface SelectedWorkflowText {
  description?: string;
  name?: string;
}

export function workflowLocale(localeTag: string): "" | "en" | "zh-CN" {
  const normalizedTag = localeTag.trim().replaceAll("_", "-").toLowerCase();
  const language = normalizedTag.split("-")[0];
  if (language === "en") {
    return "en";
  }
  if (normalizedTag === "zh-cn") {
    return "zh-CN";
  }
  return "";
}

export function selectedWorkflowText(workflow: unknown): SelectedWorkflowText {
  if (!isRecord(workflow) || !isRecord(workflow.i18n)) {
    return {};
  }
  return {
    description: optionalString(workflow.i18n.description),
    name: optionalString(workflow.i18n.name),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function optionalString(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() !== "" ? value : undefined;
}
