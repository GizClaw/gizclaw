import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { test } from "node:test";
import assert from "node:assert/strict";

test("desktop runtime module does not persist injected credentials in browser storage", () => {
  const src = dirname(dirname(dirname(new URL(import.meta.url).pathname)));
  const offenders: string[] = [];
  for (const file of walk(src)) {
    if (!/\.(ts|tsx)$/.test(file) || file.endsWith(".test.ts")) {
      continue;
    }
    const source = readFileSync(file, "utf8");
    if (/\b(localStorage|indexedDB|sessionStorage)\b/.test(source)) {
      offenders.push(file);
    }
  }
  assert.deepEqual(offenders, []);
});

function walk(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    const stat = statSync(path);
    if (stat.isDirectory()) {
      out.push(...walk(path));
    } else {
      out.push(path);
    }
  }
  return out;
}
