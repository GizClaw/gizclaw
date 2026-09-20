import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { DEVELOPMENT_VERSION } from "./check-package-release.mjs";

const checker = fileURLToPath(
  new URL("./check-package-release.mjs", import.meta.url),
);

for (const packageDirectory of ["gizclaw", "gizclaw-control"]) {
  test(`accepts ${packageDirectory} development manifests without Git history`, async () => {
    await withPackage(packageDirectory, {}, async ({ run }) => {
      const result = run();
      assert.equal(result.status, 0, result.stderr);
      assert.deepEqual(JSON.parse(result.stdout), {
        package: `@gizclaw/${packageDirectory}`,
        version: DEVELOPMENT_VERSION,
      });
    });
  });

  test(`accepts injected ${packageDirectory} Release identity without a lockfile`, async () => {
    await withPackage(
      packageDirectory,
      {
        version: "0.18.17",
        dependencies: { "@gizclaw/gizclaw": "0.18.17" },
      },
      async ({ run, directory, manifestPath }) => {
        await rm(join(directory, "package-lock.json"));
        const result = run(
          "--release-version",
          "0.18.17",
          "--manifest",
          manifestPath,
        );
        assert.equal(result.status, 0, result.stderr);
        assert.equal(JSON.parse(result.stdout).version, "0.18.17");
      },
    );
  });
}

for (const [label, additions, args, pattern] of [
  ["wrong package", { name: "@gizclaw/other" }, [], /package name/u],
  [
    "independent version",
    { version: "0.32.1" },
    [],
    /version must equal 0\.0\.0/u,
  ],
  ["missing version", { version: undefined }, [], /version must equal/u],
  [
    "wrong injected version",
    {},
    ["--release-version", "0.18.17"],
    /version must equal 0\.18\.17/u,
  ],
  ["unknown package", {}, ["--package", "sdk/js/other"], /usage|must select/u],
  ["removed base argument", {}, ["--base", "1".repeat(40)], /usage/u],
  ["unknown option", {}, ["--unknown", "value"], /usage/u],
  ["missing argument", {}, ["--release-version"], /usage/u],
  [
    "duplicate argument",
    {},
    ["--release-version", "0.18.17", "--release-version", "0.18.17"],
    /usage/u,
  ],
  [
    "manifest without release mode",
    {},
    ["--manifest", "package.json"],
    /requires/u,
  ],
  ["leading zero", {}, ["--release-version", "00.18.17"], /canonical/u],
  ["tag prefix", {}, ["--release-version", "v0.18.17"], /canonical/u],
  ["prerelease", {}, ["--release-version", "0.18.17-rc.1"], /canonical/u],
]) {
  test(`rejects ${label}`, async () => {
    await withPackage("gizclaw", additions, async ({ run }) => {
      const result = run(...args);
      assert.notEqual(result.status, 0);
      assert.match(result.stderr, pattern);
    });
  });
}

for (const packageDirectory of ["gizclaw", "gizclaw-control"]) {
  for (const releaseVersion of [undefined, "0.18.17"]) {
    for (const publishConfig of [
      undefined,
      {},
      { registry: "https://registry.npmjs.org" },
      { registry: "https://NPM.PKG.GITHUB.COM/" },
      { registry: 1 },
    ]) {
      test(`rejects ${packageDirectory} ${releaseVersion ?? "development"} registry ${JSON.stringify(publishConfig)}`, async () => {
        await withPackage(
          packageDirectory,
          {
            version: releaseVersion ?? DEVELOPMENT_VERSION,
            publishConfig,
            dependencies: { "@gizclaw/gizclaw": releaseVersion },
          },
          async ({ run }) => {
            const result = run(
              ...(releaseVersion ? ["--release-version", releaseVersion] : []),
            );
            assert.notEqual(result.status, 0);
            assert.match(result.stderr, /must publish to GitHub Packages/u);
          },
        );
      });
    }
  }
}

for (const lockedVersion of ["0.32.1", undefined]) {
  test(`rejects lock version ${lockedVersion}`, async () => {
    await withPackage("gizclaw", {}, async ({ run, directory }) => {
      await writeFile(
        join(directory, "package-lock.json"),
        JSON.stringify({
          packages: { "sdk/js/gizclaw": { version: lockedVersion } },
        }),
      );
      const result = run();
      assert.notEqual(result.status, 0);
      assert.match(result.stderr, /package-lock JavaScript SDK version/u);
    });
  });
}

for (const dependency of ["^0.18.17", "0.32.0", "*", undefined]) {
  test(`rejects released control dependency ${dependency}`, async () => {
    await withPackage(
      "gizclaw-control",
      {
        version: "0.18.17",
        dependencies: { "@gizclaw/gizclaw": dependency },
      },
      async ({ run }) => {
        const result = run("--release-version", "0.18.17");
        assert.notEqual(result.status, 0);
        assert.match(result.stderr, /must depend on exact/u);
      },
    );
  });
}

async function withPackage(packageDirectory, additions, callback) {
  const directory = await mkdtemp(join(tmpdir(), "gizclaw-js-release-"));
  try {
    const packagePath = `sdk/js/${packageDirectory}`;
    await mkdir(join(directory, packagePath), { recursive: true });
    const manifestPath = join(directory, packagePath, "package.json");
    await writeFile(
      manifestPath,
      JSON.stringify({
        name: `@gizclaw/${packageDirectory}`,
        version: DEVELOPMENT_VERSION,
        publishConfig: { registry: "https://npm.pkg.github.com" },
        ...additions,
      }),
    );
    await writeFile(
      join(directory, "package-lock.json"),
      JSON.stringify({
        packages: { [packagePath]: { version: DEVELOPMENT_VERSION } },
      }),
    );
    await callback({
      directory,
      manifestPath,
      run: (...args) =>
        spawnSync(
          process.execPath,
          [checker, "--package", packagePath, ...args],
          {
            cwd: directory,
            encoding: "utf8",
          },
        ),
    });
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
}
