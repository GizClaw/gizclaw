import { appendFileSync, readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { isDeepStrictEqual } from "node:util";

const args = new Map();
for (let index = 2; index < process.argv.length; index += 2) {
  const name = process.argv[index];
  const value = process.argv[index + 1];
  if (name == null || value == null || !name.startsWith("--")) {
    throw new Error(
      "usage: check-package-release.mjs --base COMMIT [--package sdk/js/NAME] [--github-output PATH]",
    );
  }
  args.set(name, value);
}

const base = args.get("--base")?.trim() ?? "";
if (!/^[0-9a-f]{40}$/u.test(base)) {
  throw new Error("--base must be a full Git commit SHA");
}
git("cat-file", "-e", `${base}^{commit}`);

const packagePath = args.get("--package") ?? "sdk/js/gizclaw";
const packageMatch = /^sdk\/js\/([a-z][a-z0-9-]*)$/u.exec(packagePath);
if (packageMatch == null) {
  throw new Error("--package must be a directory directly under sdk/js");
}
const packageName = `@gizclaw/${packageMatch[1]}`;
const manifestPath = `${packagePath}/package.json`;
const lockPath = "package-lock.json";
const manifest = readJSON(manifestPath);
const baseManifest = readBaseManifest();
const lock = readJSON(lockPath);
const lockedManifest = lock.packages?.[packagePath];

if (manifest.name !== packageName) {
  throw new Error(`unexpected JavaScript SDK package name: ${manifest.name}`);
}
if (manifest.publishConfig?.registry !== "https://npm.pkg.github.com") {
  throw new Error("JavaScript SDK must publish to GitHub Packages");
}
if (lockedManifest?.version !== manifest.version) {
  throw new Error(
    `package-lock JavaScript SDK version ${lockedManifest?.version ?? "missing"} does not match ${manifest.version}`,
  );
}

const currentVersion = parseStableVersion(manifest.version, "current package");
const baseVersion =
  baseManifest == null
    ? [0, 0, 0]
    : parseStableVersion(baseManifest.version, "base package");
const changedPaths = git(
  "diff",
  "--name-only",
  "--diff-filter=ACMRDT",
  `${base}...HEAD`,
)
  .split("\n")
  .filter(Boolean);
const releasePaths = changedPaths.filter((path) => {
  if (path === manifestPath) {
    return (
      baseManifest == null || hasManifestReleaseChange(baseManifest, manifest)
    );
  }
  return isReleasePath(path);
});
const publish = releasePaths.length > 0;

if (publish && compareVersions(currentVersion, baseVersion) <= 0) {
  throw new Error(
    `JavaScript SDK release files changed without increasing ${baseManifest?.version ?? "0.0.0"}`,
  );
}
if (!publish && manifest.version !== baseManifest?.version) {
  throw new Error(
    "JavaScript SDK version changed without a publishable SDK change",
  );
}

const outputPath = args.get("--github-output");
if (outputPath != null) {
  appendFileSync(
    outputPath,
    `version=${manifest.version}\npublish=${publish ? "true" : "false"}\n`,
  );
}
console.log(
  JSON.stringify({
    baseVersion: baseManifest?.version ?? null,
    package: packageName,
    publish,
    releasePaths,
    version: manifest.version,
  }),
);

function isReleasePath(path) {
  if (path === "sdk/js/scripts/prepare-published-sdk.mjs") return true;
  if (!path.startsWith(`${packagePath}/`)) return false;
  if (path === manifestPath) return false;
  if (path.endsWith(".test.ts")) return false;
  return !path.endsWith("/tsconfig.json");
}

function hasManifestReleaseChange(baseValue, currentValue) {
  const { version: _baseVersion, ...baseReleaseValue } = baseValue;
  const { version: _currentVersion, ...currentReleaseValue } = currentValue;
  return !isDeepStrictEqual(baseReleaseValue, currentReleaseValue);
}

function parseStableVersion(value, label) {
  const match = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/u.exec(
    value,
  );
  if (match == null)
    throw new Error(`${label} version is not stable SemVer: ${value}`);
  return match.slice(1).map(Number);
}

function compareVersions(left, right) {
  for (let index = 0; index < left.length; index++) {
    const difference = left[index] - right[index];
    if (difference !== 0) return difference;
  }
  return 0;
}

// The base manifest is null when the package did not exist at the base commit.
function readBaseManifest() {
  try {
    return JSON.parse(git("show", `${base}:${manifestPath}`));
  } catch (error) {
    if (
      /does not exist|exists on disk, but not in/u.test(
        String(error.stderr ?? error),
      )
    ) {
      return null;
    }
    throw error;
  }
}

function readJSON(path) {
  return JSON.parse(readFileSync(path, "utf8"));
}

function git(...arguments_) {
  return execFileSync("git", arguments_, {
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();
}
