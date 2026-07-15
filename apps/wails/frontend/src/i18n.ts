import en from "../../i18n/locales/en.json";
import zhCN from "../../i18n/locales/zh-CN.json";
import { matchLocale } from "./i18n-locale";

const catalogs = { en, "zh-CN": zhCN } as const;
type MessageKey = keyof typeof en;

function systemLocale(): keyof typeof catalogs {
  return matchLocale(navigator.language);
}

export function useMessages() {
  const messages = catalogs[systemLocale()];
  return (key: MessageKey) => messages[key] ?? en[key];
}
