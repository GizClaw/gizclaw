import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

// Repository manifests and workspace lock entries use this development version.
// Only Release packaging replaces it with the canonical tag version.
export const DEVELOPMENT_VERSION = "0.0.0";

function main() {
  const args = new Map();
  const options = new Set(["--package", "--release-version", "--manifest"]);
  for (let index = 2; index < process.argv.length; index += 2) {
    const name = process.argv[index];
    const value = process.argv[index + 1];
    if (
      !options.has(name) ||
      args.has(name) ||
      !value ||
      value.startsWith("--")
    ) {
      throw new Error(
        "usage: check-package-release.mjs [--package sdk/js/NAME] [--release-version VERSION [--manifest PATH]]",
      );
    }
    args.set(name, value);
  }

  const packagePath = args.get("--package") ?? "sdk/js/gizclaw";
  if (!/^sdk\/js\/(gizclaw|gizclaw-control)$/u.test(packagePath)) {
    throw new Error(
      "--package must select sdk/js/gizclaw or sdk/js/gizclaw-control",
    );
  }
  const releaseVersion = args.get("--release-version");
  if (
    releaseVersion != null &&
    !/^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/u.test(releaseVersion)
  ) {
    throw new Error("--release-version must be canonical MAJOR.MINOR.PATCH");
  }
  if (args.has("--manifest") && releaseVersion == null) {
    throw new Error("--manifest requires --release-version");
  }

  const packageName = `@gizclaw/${packagePath.slice("sdk/js/".length)}`;
  const manifest = readJSON(
    args.get("--manifest") ?? `${packagePath}/package.json`,
  );
  if (manifest.name !== packageName) {
    throw new Error(`unexpected JavaScript SDK package name: ${manifest.name}`);
  }
  const expectedVersion = releaseVersion ?? DEVELOPMENT_VERSION;
  if (manifest.version !== expectedVersion) {
    throw new Error(
      `JavaScript SDK version must equal ${expectedVersion}, got ${manifest.version}`,
    );
  }
  if (manifest.publishConfig?.registry !== "https://npm.pkg.github.com") {
    throw new Error("JavaScript SDK must publish to GitHub Packages");
  }

  if (releaseVersion == null) {
    const lock = readJSON("package-lock.json");
    const lockedVersion = lock.packages?.[packagePath]?.version;
    if (lockedVersion !== manifest.version) {
      throw new Error(
        `package-lock JavaScript SDK version ${lockedVersion ?? "missing"} does not match ${manifest.version}`,
      );
    }
  } else if (
    packageName === "@gizclaw/gizclaw-control" &&
    manifest.dependencies?.["@gizclaw/gizclaw"] !== releaseVersion
  ) {
    throw new Error(
      `released control SDK must depend on exact @gizclaw/gizclaw version ${releaseVersion}`,
    );
  }
  console.log(
    JSON.stringify({ package: packageName, version: manifest.version }),
  );
}

function readJSON(path) {
  return JSON.parse(readFileSync(path, "utf8"));
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  main();
}
