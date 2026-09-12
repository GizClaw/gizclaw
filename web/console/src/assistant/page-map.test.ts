import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";

import { ASSISTANT_INSTRUCTIONS } from "@gizclaw/assistant";
import { describe, expect, it } from "vitest";

// The assistant guides users by the console's own labels; every label its page
// map quotes must still exist in the console source.
function sources(dir: string): string {
  return readdirSync(dir, { withFileTypes: true })
    .map((entry) => {
      const file = path.join(dir, entry.name);
      if (entry.isDirectory()) return sources(file);
      return /\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name)
        ? readFileSync(file, "utf8")
        : "";
    })
    .join("\n");
}

describe("assistant page map", () => {
  it("quotes only labels the console renders", () => {
    const map =
      ASSISTANT_INSTRUCTIONS.split("## 控制台页面地图")[1]?.split("\n## ")[0];
    expect(map).toBeTruthy();
    const labels = [...map!.matchAll(/"([^"]+)"/g)].map((match) => match[1]);
    expect(labels.length).toBeGreaterThan(10);
    const source = sources(path.resolve(import.meta.dirname, ".."));
    const missing = labels.filter(
      (label) => !/^[a-z_:-]+$/i.test(label) && !source.includes(label),
    );
    expect(missing).toEqual([]);
  });
});
