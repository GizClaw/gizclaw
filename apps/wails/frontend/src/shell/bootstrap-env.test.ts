import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  readBootstrapEnvValues,
  updateBootstrapEnvContent,
} from "./bootstrap-env.ts";

describe("bootstrap env editor", () => {
  const names = ["FIRST_TOKEN", "SECOND_URL"] as const;

  it("reads editable quoted and unquoted values", () => {
    assert.deepEqual(
      readBootstrapEnvValues(
        "FIRST_TOKEN='literal # token'\nexport SECOND_URL=https://example.com # comment\n",
        names,
      ),
      {
        FIRST_TOKEN: "literal # token",
        SECOND_URL: "https://example.com",
      },
    );
  });

  it("updates known assignments while preserving comments", () => {
    assert.equal(
      updateBootstrapEnvContent(
        "# Provider credentials\nFIRST_TOKEN=old\n",
        names,
        { FIRST_TOKEN: "new value", SECOND_URL: "https://example.com" },
      ),
      '# Provider credentials\nFIRST_TOKEN="new value"\nSECOND_URL="https://example.com"\n',
    );
  });

  it("removes cleared assignments", () => {
    assert.equal(
      updateBootstrapEnvContent("FIRST_TOKEN=old\n", names, {
        FIRST_TOKEN: "",
      }),
      "",
    );
  });
});
