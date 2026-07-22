import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { matchLocale } from "./i18n-locale.ts";

test("desktop catalogs have key parity and locale fallback", () => {
  const en = JSON.parse(
    readFileSync(
      new URL("../../i18n/locales/en.json", import.meta.url),
      "utf8",
    ),
  );
  const zhCN = JSON.parse(
    readFileSync(
      new URL("../../i18n/locales/zh-CN.json", import.meta.url),
      "utf8",
    ),
  );
  assert.deepEqual(Object.keys(zhCN).sort(), Object.keys(en).sort());
  assert.equal(matchLocale("zh_CN"), "zh-CN");
  assert.equal(matchLocale("zh-Hans-CN"), "zh-CN");
  assert.equal(matchLocale("fr-FR"), "en");
});
