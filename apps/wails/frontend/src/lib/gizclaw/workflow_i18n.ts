export interface SelectedWorkflowText {
  description?: string;
  name?: string;
}

export function workflowLocale(localeTag: string): "en" | "unspecified" | "zh-CN" {
  const normalizedTag = localeTag.trim().replaceAll("_", "-").toLowerCase();
  const subtags = normalizedTag.split("-");
  const language = subtags[0];
  if (language === "en") {
    return "en";
  }
  if (
    language === "zh" &&
    !subtags.includes("hant") &&
    (subtags.includes("hans") || subtags.includes("cn"))
  ) {
    return "zh-CN";
  }
  return "unspecified";
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
