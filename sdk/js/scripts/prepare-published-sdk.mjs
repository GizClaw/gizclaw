import { cp, mkdir, readdir, readFile, writeFile } from "node:fs/promises";

const packageRoot = new URL("../gizclaw/", import.meta.url);
const distRoot = new URL("dist/", packageRoot);
const eventSource = new URL("generated/events/", packageRoot);
const eventOutput = new URL("generated/events/", distRoot);

await mkdir(eventOutput, { recursive: true });
for (const name of ["peer_event_pb.js", "peer_event_pb.d.ts"]) {
  await cp(new URL(name, eventSource), new URL(name, eventOutput));
}
await rewriteDeclarationImports(distRoot);

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
