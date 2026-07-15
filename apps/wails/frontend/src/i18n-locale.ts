export type SupportedLocale = "en" | "zh-CN";

export function matchLocale(locale: string): SupportedLocale {
  return locale.toLowerCase().replace("_", "-").startsWith("zh") ? "zh-CN" : "en";
}
