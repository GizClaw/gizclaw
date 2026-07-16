export interface SelectedWorkflowText {
  description?: string;
  name?: string;
}

export function localizedAdminWorkflowText(
  workflow: unknown,
  localeTags: readonly string[],
): SelectedWorkflowText {
  if (!isRecord(workflow) || !isRecord(workflow.i18n)) {
    return {};
  }
  const candidates: string[] = supportedWorkflowLocales(localeTags);
  const defaultLocale = optionalString(workflow.i18n.default_locale);
  if (defaultLocale != null) candidates.push(defaultLocale);
  for (const locale of candidates) {
    const catalog = workflow.i18n[locale];
    if (!isRecord(catalog)) continue;
    const description = optionalString(catalog.description);
    const name = optionalString(catalog.name);
    if (description == null && name == null) return {};
    return { description, name };
  }
  return {};
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

export function supportedWorkflowLocales(localeTags: readonly string[]): Array<"en" | "zh-CN"> {
  const locales: Array<"en" | "zh-CN"> = [];
  for (const localeTag of localeTags) {
    const locale = workflowLocale(localeTag);
    if (locale !== "unspecified" && !locales.includes(locale)) {
      locales.push(locale);
    }
  }
  return locales;
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
