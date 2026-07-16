import assert from "node:assert/strict";
import test from "node:test";

import {
  localizedAdminWorkflowText,
  selectedWorkflowText,
  supportedWorkflowLocales,
  workflowLocale,
} from "./workflow_i18n.ts";

test("workflowLocale leaves unsupported UI languages unspecified", () => {
  assert.equal(workflowLocale("fr-FR"), "unspecified");
  assert.equal(workflowLocale("en-US"), "en");
  assert.equal(workflowLocale("zh_CN"), "zh-CN");
  assert.equal(workflowLocale("zh-Hans"), "zh-CN");
  assert.equal(workflowLocale("zh-Hans-CN"), "zh-CN");
  assert.equal(workflowLocale("zh-TW"), "unspecified");
  assert.equal(workflowLocale("zh-HK"), "unspecified");
  assert.equal(workflowLocale("zh-Hant-CN"), "unspecified");
});

test("localizedAdminWorkflowText selects browser locale and default fallback", () => {
  const workflow = {
    name: "assistant",
    i18n: {
      default_locale: "en",
      en: { name: "Assistant", description: "General help" },
      "zh-CN": { name: "助手", description: "通用帮助" },
    },
  };
  assert.deepEqual(localizedAdminWorkflowText(workflow, ["zh-Hans-CN"]), {
    description: "通用帮助",
    name: "助手",
  });
  assert.deepEqual(localizedAdminWorkflowText(workflow, ["fr-FR"]), {
    description: "General help",
    name: "Assistant",
  });
});

test("localizedAdminWorkflowText preserves stable-ID fallback for empty catalogs", () => {
  assert.deepEqual(
    localizedAdminWorkflowText(
      { i18n: { default_locale: "en", en: { name: " ", description: "" } } },
      ["en-US"],
    ),
    {},
  );
});

test("supportedWorkflowLocales continues through browser fallbacks", () => {
  assert.deepEqual(supportedWorkflowLocales(["fr-FR", "en-US", "zh-Hans", "en-GB"]), [
    "en",
    "zh-CN",
  ]);
});

test("selectedWorkflowText reads the selected workflow catalog", () => {
  assert.deepEqual(
    selectedWorkflowText({
      name: "assistant",
      i18n: {
        name: "助手",
        description: "默认助手工作流",
      },
      spec: { driver: "flowcraft" },
    }),
    {
      description: "默认助手工作流",
      name: "助手",
    },
  );
});
