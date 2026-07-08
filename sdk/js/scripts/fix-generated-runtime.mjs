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
  if (url.pathname.endsWith("/core/serverSentEvents.gen.ts")) {
    await rewriteSseRuntime(url);
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

async function rewriteSseRuntime(url) {
  const before = await readFile(url, "utf8");
  let after = before.replace(
    "export function createSseClient<TData = unknown>({\n",
    "class SseHttpError extends Error {\n  status: number;\n  statusText: string;\n  error: unknown;\n\n  constructor(status: number, statusText: string, error: unknown) {\n    super(`SSE failed: ${status} ${statusText}`);\n    this.status = status;\n    this.statusText = statusText;\n    this.error = error;\n  }\n}\n\nasync function parseSseErrorResponse(response: Response): Promise<unknown> {\n  const text = await response.text();\n  if (text === '') {\n    return new Error(`SSE failed: ${response.status} ${response.statusText}`);\n  }\n  try {\n    return JSON.parse(text);\n  } catch {\n    return text;\n  }\n}\n\nexport function createSseClient<TData = unknown>({\n",
  );
  after = after.replace(
    "        if (!response.ok) throw new Error(`SSE failed: ${response.status} ${response.statusText}`);\n",
    "        if (!response.ok) {\n          throw new SseHttpError(response.status, response.statusText, await parseSseErrorResponse(response));\n        }\n",
  );
  after = after.replace(
    "      } catch (error) {\n        // connection failed or aborted; retry after delay\n        onSseError?.(error);\n\n        if (sseMaxRetryAttempts !== undefined && attempt >= sseMaxRetryAttempts) {\n",
    "      } catch (error) {\n        const reportedError = error instanceof SseHttpError ? error.error : error;\n        onSseError?.(reportedError);\n\n        if (error instanceof SseHttpError) {\n          throw reportedError;\n        }\n\n        // connection failed or aborted; retry after delay\n        if (sseMaxRetryAttempts !== undefined && attempt >= sseMaxRetryAttempts) {\n",
  );

  if (after !== before) {
    await writeFile(url, after);
  }
}

function ensureDirURL(url) {
  return url.pathname.endsWith("/") ? url : new URL(`${url.href}/`);
}
