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
  if (!url.pathname.endsWith("/client/client.gen.ts")) {
    return;
  }

  const before = await readFile(url, "utf8");
  let after = before.replace(
    "    let request: Request | undefined;\n    let response: Response | undefined;\n\n    try {",
    "    let request: Request | undefined;\n    let response: Response | undefined;\n    let resolvedOptions: ResolvedRequestOptions | undefined;\n\n    try {",
  );
  after = after.replace(
    "      const { opts, url } = await beforeRequest(options);\n      const requestInit: ReqInit = {",
    "      const { opts, url } = await beforeRequest(options);\n      resolvedOptions = opts;\n      const requestInit: ReqInit = {",
  );
  after = after.replaceAll(
    "      for (const fn of interceptors.error.fns) {\n        if (fn) {\n          finalError = await fn(finalError, response, request, options as ResolvedRequestOptions);\n        }\n      }",
    "      if (resolvedOptions) {\n        for (const fn of interceptors.error.fns) {\n          if (fn) {\n            finalError = await fn(finalError, response, request, resolvedOptions);\n          }\n        }\n      }",
  );

  if (after !== before) {
    await writeFile(url, after);
  }
}

function ensureDirURL(url) {
  return url.pathname.endsWith("/") ? url : new URL(`${url.href}/`);
}
