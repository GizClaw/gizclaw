import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdtemp, mkdir, rm, unlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const checker = fileURLToPath(
  new URL("./check-package-release.mjs", import.meta.url),
);

test("accepts publishable SDK content with a version increase", async () => {
  await withRepository(async ({ base, directory }) => {
    await writePackage(directory, "0.7.2");
    await writeFile(join(directory, "sdk/js/gizclaw/events.ts"), "export {}\n");
    commit(directory, "release SDK");

    const result = runChecker(directory, base);
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(JSON.parse(result.stdout), {
      baseVersion: "0.7.1",
      publish: true,
      releasePaths: ["sdk/js/gizclaw/events.ts"],
      version: "0.7.2",
    });
  });
});

test("rejects publishable SDK content without a version increase", async () => {
  await withRepository(async ({ base, directory }) => {
    await writeFile(join(directory, "sdk/js/gizclaw/events.ts"), "export {}\n");
    commit(directory, "change SDK");

    const result = runChecker(directory, base);
    assert.notEqual(result.status, 0);
    assert.match(
      result.stderr,
      /release files changed without increasing 0\.7\.1/u,
    );
  });
});

test("rejects a version-only change", async () => {
  await withRepository(async ({ base, directory }) => {
    await writePackage(directory, "0.7.2");
    commit(directory, "change version only");

    const result = runChecker(directory, base);
    assert.notEqual(result.status, 0);
    assert.match(
      result.stderr,
      /version changed without a publishable SDK change/u,
    );
  });
});

test("accepts publishable manifest metadata with a version increase", async () => {
  await withRepository(async ({ base, directory }) => {
    await writePackage(directory, "0.7.2", { description: "GizClaw SDK" });
    commit(directory, "change package metadata");

    const result = runChecker(directory, base);
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(JSON.parse(result.stdout).releasePaths, [
      "sdk/js/gizclaw/package.json",
    ]);
  });
});

test("accepts a publishable SDK deletion with a version increase", async () => {
  await withRepository(async ({ base, directory }) => {
    await writePackage(directory, "0.7.2");
    await unlink(join(directory, "sdk/js/gizclaw/events.ts"));
    commit(directory, "delete SDK content");

    const result = runChecker(directory, base);
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(JSON.parse(result.stdout).releasePaths, [
      "sdk/js/gizclaw/events.ts",
    ]);
  });
});

test("accepts a build configuration change with a version increase", async () => {
  await withRepository(async ({ base, directory }) => {
    await writePackage(directory, "0.7.2");
    await writeFile(
      join(directory, "sdk/js/gizclaw/tsconfig.build.json"),
      '{"compilerOptions":{"sourceMap":true}}\n',
    );
    commit(directory, "change build configuration");

    const result = runChecker(directory, base);
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(JSON.parse(result.stdout).releasePaths, [
      "sdk/js/gizclaw/tsconfig.build.json",
    ]);
  });
});

test("ignores package manifest property ordering", async () => {
  await withRepository(async ({ base, directory }) => {
    const reorderedManifest = {
      publishConfig: { registry: "https://npm.pkg.github.com" },
      version: "0.7.1",
      name: "@gizclaw/gizclaw",
    };
    await writeFile(
      join(directory, "sdk/js/gizclaw/package.json"),
      `${JSON.stringify(reorderedManifest, null, 2)}\n`,
    );
    commit(directory, "reorder package metadata");

    const result = runChecker(directory, base);
    assert.equal(result.status, 0, result.stderr);
    assert.equal(JSON.parse(result.stdout).publish, false);
  });
});

test("rejects a package-lock version mismatch", async () => {
  await withRepository(async ({ base, directory }) => {
    await writePackage(directory, "0.7.2", {}, "0.7.1");
    await writeFile(join(directory, "sdk/js/gizclaw/events.ts"), "export {}\n");
    commit(directory, "mismatch lockfile");

    const result = runChecker(directory, base);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /package-lock JavaScript SDK version 0\.7\.1/u);
  });
});

async function withRepository(callback) {
  const directory = await mkdtemp(join(tmpdir(), "gizclaw-js-release-"));
  try {
    git(directory, "init", "-b", "main");
    git(directory, "config", "user.email", "test@example.com");
    git(directory, "config", "user.name", "Release Contract Test");
    await mkdir(join(directory, "sdk/js/gizclaw"), { recursive: true });
    await writePackage(directory, "0.7.1");
    await writeFile(
      join(directory, "sdk/js/gizclaw/events.ts"),
      "export const initial = true;\n",
    );
    commit(directory, "base");
    const base = git(directory, "rev-parse", "HEAD").trim();
    await callback({ base, directory });
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
}

async function writePackage(
  directory,
  version,
  manifestAdditions = {},
  lockVersion = version,
) {
  const manifest = {
    name: "@gizclaw/gizclaw",
    version,
    publishConfig: { registry: "https://npm.pkg.github.com" },
    ...manifestAdditions,
  };
  const lock = {
    packages: {
      "sdk/js/gizclaw": {
        name: "@gizclaw/gizclaw",
        version: lockVersion,
      },
    },
  };
  await writeFile(
    join(directory, "sdk/js/gizclaw/package.json"),
    `${JSON.stringify(manifest, null, 2)}\n`,
  );
  await writeFile(
    join(directory, "package-lock.json"),
    `${JSON.stringify(lock, null, 2)}\n`,
  );
}

function commit(directory, message) {
  git(directory, "add", ".");
  git(directory, "-c", "commit.gpgsign=false", "commit", "-m", message);
}

function runChecker(directory, base) {
  return spawnSync(process.execPath, [checker, "--base", base], {
    cwd: directory,
    encoding: "utf8",
  });
}

function git(directory, ...arguments_) {
  return execFileSync("git", arguments_, {
    cwd: directory,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
}
