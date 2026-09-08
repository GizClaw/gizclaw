import assert from "node:assert/strict";
import {
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
  existsSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { test } from "node:test";
import { generateEmbed } from "./embed.mjs";

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), "gizclaw-console-embed-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(join(root, "dist/assets"), { recursive: true });
  writeFileSync(
    join(root, "dist/index.html"),
    '<script src="./assets/app.js"></script><link href="./assets/app.css">',
  );
  writeFileSync(join(root, "dist/assets/app.js"), "console.log('fixture');");
  writeFileSync(join(root, "dist/assets/app.css"), "body{}");
  writeFileSync(join(root, "go.mod"), "module consoleembedtest\n\ngo 1.26\n");
  return root;
}

function build(root) {
  return spawnSync("go", ["build", "."], {
    cwd: root,
    encoding: "utf8",
    timeout: 60000,
  });
}

test("complete assets build and missing referenced JS or CSS fails Go compilation", (t) => {
  const root = fixture(t);
  generateEmbed(root);
  const complete = build(root);
  assert.equal(complete.status, 0, complete.stderr || complete.error?.message);
  assert.match(
    readFileSync(join(root, "assets_generated.go"), "utf8"),
    /dist\/assets\/app.js/,
  );
  writeFileSync(join(root, "dist/assets/unrelated.txt"), "remaining asset");
  for (const extension of ["js", "css"]) {
    const path = join(root, `dist/assets/app.${extension}`);
    const content = readFileSync(path);
    rmSync(path);
    const incomplete = build(root);
    assert.notEqual(incomplete.status, 0);
    assert.match(incomplete.stderr, /no matching files found/);
    writeFileSync(path, content);
  }
});

test("generation rejects incomplete HTML references and removes stale manifests", (t) => {
  const root = fixture(t);
  generateEmbed(root);
  rmSync(join(root, "dist/assets/app.js"));
  assert.throws(() => generateEmbed(root), /references missing asset/);
  assert.equal(existsSync(join(root, "assets_generated.go")), false);
});
