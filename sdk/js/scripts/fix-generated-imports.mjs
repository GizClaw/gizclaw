import { readdir, readFile, stat, writeFile } from "node:fs/promises";

const roots = [
  new URL("../gizclaw/generated/", import.meta.url),
  new URL("../../../apps/wails/frontend/src/generated/", import.meta.url),
];

for (const root of roots) {
  await rewriteTree(root);
}

async function rewriteTree(url) {
  const info = await stat(url).catch(() => undefined);
  if (info == null) {
    return;
  }
  if (info.isDirectory()) {
    for (const entry of await readdir(url)) {
      await rewriteTree(new URL(`${entry}${entry.endsWith("/") ? "" : ""}`, ensureDirURL(url)));
    }
    return;
  }
  if (!url.pathname.endsWith(".ts")) {
    return;
  }
  const before = await readFile(url, "utf8");
  const after = await replaceAsync(
    before,
    /\b(from\s+['"])(\.\.?\/[^'"]+?)(['"])/g,
    async (_match, prefix, specifier, suffix) => `${prefix}${await withTSExtension(url, specifier)}${suffix}`,
  );
  if (after !== before) {
    await writeFile(url, after);
  }
}

function ensureDirURL(url) {
  return url.pathname.endsWith("/") ? url : new URL(`${url.href}/`);
}

async function withTSExtension(fileURL, specifier) {
  if (/\.(?:ts|tsx|js|mjs|cjs|json)$/.test(specifier)) {
    return specifier;
  }
  const baseURL = new URL(specifier, fileURL);
  if (await exists(new URL(`${baseURL.href}.ts`))) {
    return `${specifier}.ts`;
  }
  if (await exists(new URL(`${ensureDirURL(baseURL).href}index.ts`))) {
    return `${specifier}/index.ts`;
  }
  return `${specifier}.ts`;
}

async function exists(url) {
  return (await stat(url).catch(() => undefined)) != null;
}

async function replaceAsync(text, pattern, replacer) {
  const matches = [...text.matchAll(pattern)];
  let out = "";
  let lastIndex = 0;
  for (const match of matches) {
    out += text.slice(lastIndex, match.index);
    out += await replacer(...match);
    lastIndex = (match.index ?? 0) + match[0].length;
  }
  out += text.slice(lastIndex);
  return out;
}
