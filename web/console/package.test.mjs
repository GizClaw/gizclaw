import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { once } from "node:events";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import { checkReleaseTag, preparePackage } from "./package.mjs";

test("monitor releases have a separate stable tag matching the package version", () => {
  checkReleaseTag("monitor-v0.1.0", "0.1.0");
  for (const tag of [
    "v0.1.0",
    "console-v0.1.0",
    "monitor-v0.1.1",
    "monitor-v00.1.0",
    "monitor-v0.1.0-beta.1",
    "monitor-v0.1.0+build",
  ]) {
    assert.throws(() => checkReleaseTag(tag, "0.1.0"), /Monitor tag must be/);
  }
});

test(
  "the packed monitor installs offline without SDKs and serves all shipped assets",
  { timeout: 60000 },
  async (t) => {
    const root = fileURLToPath(new URL(".", import.meta.url));
    const temp = mkdtempSync(join(tmpdir(), "gizclaw-monitor-package-"));
    t.after(() => rmSync(temp, { recursive: true, force: true }));
    const manifest = preparePackage(root);
    const runNpm = (args, cwd = temp) =>
      execFileSync("npm", args, {
        cwd,
        encoding: "utf8",
        timeout: 30000,
        stdio: ["ignore", "pipe", "pipe"],
      });
    const [packed] = JSON.parse(
      runNpm(
        [
          "pack",
          "--json",
          "--ignore-scripts",
          "--workspaces=false",
          "--pack-destination",
          temp,
        ],
        root,
      ),
    );
    assert.equal(packed.name, "@gizclaw/monitor");
    assert.equal(packed.version, manifest.version);
    assert(packed.files.some(({ path }) => path === "LICENSE"));
    for (const { path } of packed.files) {
      assert(
        ["package.json", "LICENSE", "serve.mjs"].includes(path) ||
          path.startsWith("dist/"),
        `Unexpected published file: ${path}`,
      );
    }
    runNpm([
      "install",
      join(temp, packed.filename),
      "--offline",
      "--omit=dev",
      "--ignore-scripts",
      "--no-audit",
      "--no-fund",
      "--workspaces=false",
    ]);
    const installed = JSON.parse(
      readFileSync(
        join(temp, "node_modules/@gizclaw/monitor/package.json"),
        "utf8",
      ),
    );
    assert.equal(installed.dependencies, undefined);
    const tree = JSON.parse(runNpm(["ls", "--all", "--json"]));
    assert.deepEqual(Object.keys(tree.dependencies), ["@gizclaw/monitor"]);
    assert.equal(tree.dependencies["@gizclaw/monitor"].dependencies, undefined);

    // Exercise npm's actual bin symlink, just as npx or a global install does.
    const executable = join(temp, "node_modules/.bin/gizclaw-monitor");
    assert.equal(
      execFileSync(executable, ["--version"], { encoding: "utf8" }).trim(),
      manifest.version,
    );
    assert.match(
      execFileSync(executable, ["--help"], { encoding: "utf8" }),
      /deviceEndpoint/,
    );
    const child = spawn(executable, ["--port", "0"], {
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stderr = "";
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    t.after(async () => {
      if (child.exitCode === null && child.signalCode === null) {
        const exited = once(child, "exit");
        child.kill("SIGTERM");
        await exited;
      }
    });
    const address = await new Promise((resolve, reject) => {
      const timer = setTimeout(
        () => reject(new Error(`Monitor did not start: ${stderr}`)),
        10000,
      );
      child.once("error", reject);
      child.once("exit", () => {
        clearTimeout(timer);
        reject(new Error(`Monitor exited before startup: ${stderr}`));
      });
      let stdout = "";
      child.stdout.on("data", (chunk) => {
        stdout += chunk;
        const match = stdout.match(/http:\/\/127\.0\.0\.1:\d+\//);
        if (match) {
          clearTimeout(timer);
          resolve(match[0]);
        }
      });
    });
    const page = await fetch(address);
    assert.equal(page.status, 200);
    const html = await page.text();
    assert.match(html, /assets\/[^" ]+\.js/);
    // Include lazy assistant/knowledge chunks as well as initial JS and CSS.
    for (const { path } of packed.files.filter(({ path }) =>
      path.startsWith("dist/"),
    )) {
      const response = await fetch(
        new URL(path.slice("dist/".length), address),
      );
      assert.equal(response.status, 200, path);
      assert.deepEqual(
        Buffer.from(await response.arrayBuffer()),
        readFileSync(join(root, path)),
        path,
      );
    }
  },
);
