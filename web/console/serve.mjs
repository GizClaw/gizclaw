#!/usr/bin/env node
import { createServer } from "node:http";
import { readFile, realpath, stat } from "node:fs/promises";
import { extname, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";

const contentTypes = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".webp": "image/webp",
  ".ico": "image/x-icon",
  ".woff": "font/woff",
  ".woff2": "font/woff2",
};

// This server only serves the packaged UI. Node, device and model API requests
// go directly from the browser to the endpoints in its encrypted configuration.
export async function createConsoleServer(
  directory = fileURLToPath(new URL("./dist", import.meta.url)),
) {
  const root = await realpath(directory);
  await stat(resolve(root, "index.html"));
  return createServer((request, response) => {
    serve(request, response).catch(() => {
      if (response.headersSent) response.destroy();
      else response.writeHead(500).end("Unable to read console asset");
    });
  });

  async function serve(request, response) {
    response.setHeader("X-Content-Type-Options", "nosniff");
    response.setHeader("Cache-Control", "no-cache");
    if (request.method !== "GET" && request.method !== "HEAD") {
      response.writeHead(405, { Allow: "GET, HEAD" }).end();
      return;
    }

    let pathname;
    try {
      // Read the raw path, so URL normalization cannot hide encoded traversal.
      pathname = decodeURIComponent((request.url ?? "/").split("?")[0]);
      if (
        !pathname.startsWith("/") ||
        pathname.includes("\\") ||
        pathname.includes("\0") ||
        pathname.split("/").some((part) => part.startsWith("."))
      ) {
        throw new Error("Invalid asset path");
      }
    } catch {
      response.writeHead(400).end("Invalid asset path");
      return;
    }

    if (pathname === "/monitor") {
      response.writeHead(308, { Location: "/monitor/" }).end();
      return;
    }
    if (pathname.startsWith("/monitor/")) {
      pathname = pathname.slice("/monitor".length);
    }
    if (pathname === "/") pathname = "/index.html";

    let body;
    let filename;
    try {
      filename = await realpath(resolve(root, `.${pathname}`));
      if (!filename.startsWith(`${root}${sep}`)) {
        response.writeHead(404).end();
        return;
      }
      if (!(await stat(filename)).isFile()) {
        response.writeHead(404).end();
        return;
      }
      body = await readFile(filename);
    } catch (error) {
      if (error.code !== "ENOENT" && error.code !== "ENOTDIR") throw error;
      response.writeHead(404).end();
      return;
    }

    response.setHeader(
      "Content-Type",
      contentTypes[extname(filename)] ?? "application/octet-stream",
    );
    response.setHeader("Content-Length", body.byteLength);
    if (pathname.startsWith("/assets/")) {
      response.setHeader(
        "Cache-Control",
        "public, max-age=31536000, immutable",
      );
    }
    response.writeHead(200).end(request.method === "HEAD" ? undefined : body);
  }
}

async function main() {
  const { values } = parseArgs({
    options: {
      host: { type: "string", default: "127.0.0.1" },
      port: { type: "string", default: "5174" },
      help: { type: "boolean", short: "h" },
      version: { type: "boolean", short: "v" },
    },
  });
  const { version } = JSON.parse(
    await readFile(new URL("./package.json", import.meta.url), "utf8"),
  );
  if (values.version) {
    console.log(version);
    return;
  }
  if (values.help) {
    console.log(`GizClaw Monitor ${version}
Usage: gizclaw-monitor [--host HOST] [--port PORT]

Serves the bundled monitoring UI at / and /monitor/.
Defaults to http://127.0.0.1:5174. Configure deviceEndpoint in the UI
to connect directly to an existing GizClaw access point.
Use an HTTPS reverse proxy when hosting beyond localhost.`);
    return;
  }
  if (
    !values.host ||
    !/^\d+$/.test(values.port) ||
    Number(values.port) > 65535
  ) {
    throw new Error("--host must be nonempty and --port must be 0..65535");
  }

  const server = await createConsoleServer();
  server.once("error", (error) => {
    console.error(error.message);
    process.exitCode = 1;
  });
  server.listen(Number(values.port), values.host, () => {
    const host = values.host.includes(":") ? `[${values.host}]` : values.host;
    console.log(
      `GizClaw Monitor ${version}: http://${host}:${server.address().port}/`,
    );
  });
  const stop = () => server.close();
  process.once("SIGINT", stop);
  process.once("SIGTERM", stop);
}

if (
  process.argv[1] &&
  fileURLToPath(import.meta.url) === (await realpath(process.argv[1]))
) {
  main().catch((error) => {
    console.error(error.message);
    process.exitCode = 1;
  });
}
