import en from "../../i18n/locales/en.json";
import zhCN from "../../i18n/locales/zh-CN.json";
import { useSyncExternalStore } from "react";
import { matchLocale } from "./i18n-locale";

const catalogs = { en, "zh-CN": zhCN } as const;
type MessageKey = keyof typeof en;

let locale = matchLocale(
  typeof navigator === "undefined" ? "en" : navigator.language,
);
const listeners = new Set<() => void>();

export function setLocale(next: string) {
  const matched = matchLocale(next);
  if (matched === locale) return;
  locale = matched;
  listeners.forEach((listener) => listener());
}

export function useMessages() {
  const selected = useSyncExternalStore(
    (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    () => locale,
    () => locale,
  );
  const messages = catalogs[selected];
  return (key: MessageKey) => messages[key] ?? en[key];
}
