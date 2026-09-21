import {
  cp,
  mkdir,
  readdir,
  readFile,
  stat,
  writeFile,
} from "node:fs/promises";

// Usage: prepare-published-sdk.mjs [package-directory-name]
// Finalizes one sdk/js/<package>/dist tree after `tsc`: copies committed
// generated JavaScript that tsc does not emit and rewrites `.ts` import
// specifiers in declaration files to the published `.js` names.
const packageName = process.argv[2] ?? "gizclaw";
if (!/^[a-z][a-z0-9-]*$/u.test(packageName)) {
  throw new Error(`invalid package directory name: ${packageName}`);
}
const packageRoot = new URL(`../${packageName}/`, import.meta.url);
const distRoot = new URL("dist/", packageRoot);
for (const [directory, base] of [
  ["events", "peer_event"],
  ["giznet", "admission"],
]) {
  const source = new URL(`generated/${directory}/`, packageRoot);
  const output = new URL(`generated/${directory}/`, distRoot);
  if (await exists(source)) {
    await mkdir(output, { recursive: true });
    for (const suffix of ["js", "d.ts"]) {
      const name = `${base}_pb.${suffix}`;
      await cp(new URL(name, source), new URL(name, output));
    }
  }
}
await rewriteDeclarationImports(distRoot);

async function exists(url) {
  try {
    await stat(url);
    return true;
  } catch {
    return false;
  }
}

async function rewriteDeclarationImports(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const url = new URL(entry.name, directory);
    if (entry.isDirectory()) {
      await rewriteDeclarationImports(new URL(`${url.href}/`));
      continue;
    }
    if (!entry.name.endsWith(".d.ts")) {
      continue;
    }
    const before = await readFile(url, "utf8");
    const after = before.replace(
      /(\b(?:from|import)\s*(?:\([^)]*\)\s*)?['"]\.?\.?\/[^'"]+)\.ts(['"])/g,
      "$1.js$2",
    );
    if (after !== before) {
      await writeFile(url, after);
    }
  }
}
