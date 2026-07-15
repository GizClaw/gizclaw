import assert from "node:assert/strict";
import test from "node:test";

import { selectedWorkflowText, workflowLocale } from "./workflow_i18n.ts";

test("workflowLocale leaves unsupported UI languages unspecified", () => {
  assert.equal(workflowLocale("fr-FR"), "");
  assert.equal(workflowLocale("en-US"), "en");
  assert.equal(workflowLocale("zh_CN"), "zh-CN");
  assert.equal(workflowLocale("zh-TW"), "");
  assert.equal(workflowLocale("zh-HK"), "");
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
