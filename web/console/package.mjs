import { copyFileSync, readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { parseArgs } from "node:util";

const root = fileURLToPath(new URL(".", import.meta.url));

export function checkReleaseTag(tag, version) {
  if (
    !/^monitor-v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(tag) ||
    tag !== `monitor-v${version}`
  ) {
    throw new Error(`Monitor tag must be monitor-v${version}`);
  }
}

export function preparePackage(directory = root) {
  const manifest = JSON.parse(
    readFileSync(join(directory, "package.json"), "utf8"),
  );
  if (
    manifest.name !== "@gizclaw/monitor" ||
    manifest.private ||
    !/^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(
      manifest.version,
    ) ||
    ["dependencies", "optionalDependencies", "peerDependencies"].some(
      (key) => Object.keys(manifest[key] ?? {}).length > 0,
    )
  ) {
    throw new Error(
      "Console must be a public stable package without runtime dependencies",
    );
  }
  const files = [];
  function collect(relative) {
    for (const entry of readdirSync(join(directory, relative), {
      withFileTypes: true,
    })) {
      const path = `${relative}/${entry.name}`;
      if (!/^[a-zA-Z0-9_./-]+$/.test(path) || entry.name.startsWith(".")) {
        throw new Error(`Unsupported package asset path: ${path}`);
      }
      if (entry.isDirectory()) collect(path);
      else if (entry.isFile()) files.push(path);
      else throw new Error(`Package asset must be a regular file: ${path}`);
    }
  }
  collect("dist");
  const html = readFileSync(join(directory, "dist/index.html"), "utf8");
  const references = [...html.matchAll(/(?:src|href)="\.\/(assets\/[^"]+)"/g)];
  if (
    !references.some(([, path]) => path.endsWith(".js")) ||
    !references.some(([, path]) => path.endsWith(".css")) ||
    references.some(([, path]) => !files.includes(`dist/${path}`))
  ) {
    throw new Error(
      "Build the console before packing: JavaScript or CSS is missing",
    );
  }
  copyFileSync(join(directory, "../../LICENSE"), join(directory, "LICENSE"));
  return manifest;
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  const { values } = parseArgs({ options: { tag: { type: "string" } } });
  const manifest = preparePackage();
  if (values.tag !== undefined) checkReleaseTag(values.tag, manifest.version);
  console.log(`${manifest.name}@${manifest.version} ready to pack`);
}
