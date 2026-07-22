import assert from "node:assert/strict";
import test from "node:test";

import { hasThinkingToggle, thinkingParameter } from "./play-thinking.ts";

test("thinking toggle accepts either provider-data parameter field", () => {
  assert.equal(hasThinkingToggle({ thinking_param: "enable_thinking" }), true);
  assert.equal(
    hasThinkingToggle({ thinking_level_param: "thinking.type" }),
    true,
  );
});

test("thinking toggle follows advertised disabled levels", () => {
  assert.equal(
    hasThinkingToggle({
      thinking_level_param: "reasoning_effort",
      thinking_levels: ["low", "high"],
    }),
    false,
  );
  assert.equal(
    hasThinkingToggle({
      thinking_level_param: "reasoning_effort",
      thinking_levels: ["low", "disabled"],
    }),
    true,
  );
});

test("thinking parameter prefers the request parameter and falls back to the level parameter", () => {
  assert.equal(
    thinkingParameter({
      thinking_param: "enable_thinking",
      thinking_level_param: "reasoning_effort",
    }),
    "enable_thinking",
  );
  assert.equal(
    thinkingParameter({ thinking_level_param: "reasoning_effort" }),
    "reasoning_effort",
  );
});
