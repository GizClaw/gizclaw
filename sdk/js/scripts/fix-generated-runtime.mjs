import { readdir, readFile, stat, writeFile } from "node:fs/promises";

const roots = [new URL("../gizclaw/generated/", import.meta.url)];

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
      await rewriteTree(
        new URL(`${entry}${entry.endsWith("/") ? "" : ""}`, ensureDirURL(url)),
      );
    }
    return;
  }
  if (url.pathname.endsWith("/core/serverSentEvents.gen.ts")) {
    await rewriteSseRuntime(url);
    return;
  }
  if (url.pathname.endsWith("/adminhttp/types.gen.ts")) {
    await rewriteResourceDiscriminators(url);
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

async function rewriteResourceDiscriminators(url) {
  const before = await readFile(url, "utf8");
  const resourceKinds = new Map();
  const resourceType =
    /export type ([A-Za-z0-9]+Resource(?:Writable)?) = \{\n([\s\S]*?)\n\};/g;
  for (const match of before.matchAll(resourceType)) {
    const kind = match[2].match(/^\s+kind: '([^']+)';$/m)?.[1];
    if (kind != null) {
      resourceKinds.set(match[1], kind);
    }
  }

  let rewriteCount = 0;
  const after = before.replace(
    /kind: '[^']+';\n} & ([A-Za-z0-9]+Resource(?:Writable)?)\)/g,
    (intersection, typeName) => {
      const kind = resourceKinds.get(typeName);
      if (kind == null) {
        return intersection;
      }
      rewriteCount++;
      return intersection.replace(/kind: '[^']+';/, `kind: '${kind}';`);
    },
  );
  if (resourceKinds.size > 0 && rewriteCount === 0) {
    throw new Error(`failed to rewrite resource discriminators in ${url}`);
  }
  for (const [typeName, kind] of resourceKinds) {
    const invalid = new RegExp(
      `kind: '(?!${escapeRegExp(kind)}')[^']+';\\n} & ${escapeRegExp(typeName)}\\)`,
    );
    if (invalid.test(after)) {
      throw new Error(
        `generated discriminator for ${typeName} does not match ${kind}`,
      );
    }
  }
  if (after !== before) {
    await writeFile(url, after);
  }
}

async function rewriteSseRuntime(url) {
  const before = await readFile(url, "utf8");
  let after = before;
  if (!after.includes("class SseHttpError extends Error")) {
    after = after.replace(
      "export function createSseClient<TData = unknown>({\n",
      "class SseHttpError extends Error {\n  status: number;\n  statusText: string;\n  error: unknown;\n\n  constructor(status: number, statusText: string, error: unknown) {\n    super(`SSE failed: ${status} ${statusText}`);\n    this.status = status;\n    this.statusText = statusText;\n    this.error = error;\n  }\n}\n\nfunction isRetryableSseHttpError(error: SseHttpError): boolean {\n  return error.status === 408 || error.status === 429 || (error.status >= 500 && error.status !== 501);\n}\n\nasync function parseSseErrorResponse(response: Response): Promise<unknown> {\n  const text = await response.text();\n  if (text === '') {\n    return new Error(`SSE failed: ${response.status} ${response.statusText}`);\n  }\n  try {\n    return JSON.parse(text);\n  } catch {\n    return text;\n  }\n}\n\nexport function createSseClient<TData = unknown>({\n",
    );
  }
  if (
    !after.includes(
      "function isRetryableSseHttpError(error: SseHttpError): boolean",
    )
  ) {
    after = after.replace(
      "async function parseSseErrorResponse(response: Response): Promise<unknown> {\n",
      "function isRetryableSseHttpError(error: SseHttpError): boolean {\n  return error.status === 408 || error.status === 429 || (error.status >= 500 && error.status !== 501);\n}\n\nasync function parseSseErrorResponse(response: Response): Promise<unknown> {\n",
    );
  }
  after = after.replace(
    "function isRetryableSseHttpError(error: SseHttpError): boolean {\n  return error.status === 408 || error.status === 429 || (error.status >= 500 && error.status !== 501);\n}\n\n",
    "function isJsonSseErrorResponse(value: unknown): boolean {\n  if (value == null || typeof value !== 'object') {\n    return false;\n  }\n  return 'error' in value && (value as { error?: unknown }).error !== undefined;\n}\n\nfunction isRetryableSseHttpError(error: SseHttpError): boolean {\n  if (error.status === 408 || error.status === 429) {\n    return true;\n  }\n  if (error.status < 500 || error.status === 501) {\n    return false;\n  }\n  return !isJsonSseErrorResponse(error.error);\n}\n\n",
  );
  after = after.replace(
    "        if (!response.ok) throw new Error(`SSE failed: ${response.status} ${response.statusText}`);\n",
    "        if (!response.ok) {\n          throw new SseHttpError(response.status, response.statusText, await parseSseErrorResponse(response));\n        }\n",
  );
  after = after.replace(
    "      } catch (error) {\n        // connection failed or aborted; retry after delay\n        onSseError?.(error);\n\n        if (sseMaxRetryAttempts !== undefined && attempt >= sseMaxRetryAttempts) {\n",
    "      } catch (error) {\n        const reportedError = error instanceof SseHttpError ? error.error : error;\n        onSseError?.(reportedError);\n\n        if (error instanceof SseHttpError && !isRetryableSseHttpError(error)) {\n          throw reportedError;\n        }\n\n        // connection failed or aborted; retry after delay\n        if (sseMaxRetryAttempts !== undefined && attempt >= sseMaxRetryAttempts) {\n",
  );
  after = after.replace(
    "        if (error instanceof SseHttpError) {\n          throw reportedError;\n        }\n",
    "        if (error instanceof SseHttpError && !isRetryableSseHttpError(error)) {\n          throw reportedError;\n        }\n",
  );

  if (after !== before) {
    await writeFile(url, after);
  }
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function ensureDirURL(url) {
  return url.pathname.endsWith("/") ? url : new URL(`${url.href}/`);
}
