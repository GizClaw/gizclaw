import assert from "node:assert/strict";
import { once } from "node:events";
import { mkdtemp, mkdir, rm, symlink, writeFile } from "node:fs/promises";
import { request } from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { createConsoleServer } from "./serve.mjs";

async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), "gizclaw-monitor-http-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  await mkdir(join(root, "dist/assets"), { recursive: true });
  await writeFile(join(root, "dist/index.html"), "<title>Monitor</title>");
  await writeFile(join(root, "dist/assets/app.js"), "export default 1;");
  await writeFile(join(root, "dist/assets/app.css"), "body{}");
  await writeFile(join(root, "private.txt"), "outside the published assets");
  await symlink(join(root, "private.txt"), join(root, "dist/leak.txt"));
  const server = await createConsoleServer(join(root, "dist"));
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => {
    server.closeAllConnections();
    return new Promise((resolve) => server.close(resolve));
  });
  return (path, method = "GET") =>
    new Promise((resolve, reject) => {
      const req = request(
        { hostname: "127.0.0.1", port: server.address().port, path, method },
        (response) => {
          const chunks = [];
          response.on("data", (chunk) => chunks.push(chunk));
          response.on("error", reject);
          response.on("end", () =>
            resolve({
              status: response.statusCode,
              headers: response.headers,
              body: Buffer.concat(chunks).toString(),
            }),
          );
        },
      );
      req.on("error", reject);
      req.end();
    });
}

test("serves the same console at root and /monitor with correct caching and MIME", async (t) => {
  const get = await fixture(t);
  for (const path of ["/", "/monitor/", "/index.html?reload=1"]) {
    const page = await get(path);
    assert.equal(page.status, 200);
    assert.equal(page.body, "<title>Monitor</title>");
    assert.match(page.headers["content-type"], /^text\/html/);
    assert.equal(page.headers["cache-control"], "no-cache");
    assert.equal(page.headers["x-content-type-options"], "nosniff");
  }
  const redirect = await get("/monitor");
  assert.equal(redirect.status, 308);
  assert.equal(redirect.headers.location, "/monitor/");
  for (const prefix of ["", "/monitor"]) {
    const js = await get(`${prefix}/assets/app.js`);
    assert.equal(js.body, "export default 1;");
    assert.match(js.headers["content-type"], /^text\/javascript/);
    assert.match(js.headers["cache-control"], /immutable/);
    const css = await get(`${prefix}/assets/app.css`, "HEAD");
    assert.equal(css.status, 200);
    assert.match(css.headers["content-type"], /^text\/css/);
    assert.equal(css.body, "");
    assert.equal(css.headers["content-length"], "6");
  }
});

test("missing assets and APIs remain 404, and write methods are rejected", async (t) => {
  const get = await fixture(t);
  for (const path of [
    "/assets/missing.js",
    "/assets/",
    "/monitor/api/node",
    "/gizclaw/v1/device/status",
    "/openai/v1/chat/completions",
  ]) {
    const result = await get(path);
    assert.equal(result.status, 404, path);
    assert.equal(result.headers["cache-control"], "no-cache");
  }
  const denied = await get("/", "POST");
  assert.equal(denied.status, 405);
  assert.equal(denied.headers.allow, "GET, HEAD");
});

test("rejects malformed paths, traversal and symlinks outside the asset root", async (t) => {
  const get = await fixture(t);
  for (const path of [
    "/%zz",
    "/%00",
    "/%2e%2e/private.txt",
    "/monitor/../private.txt",
    "/assets/%2e%2e/%2e%2e/private.txt",
    "/assets%5c..%5cprivate.txt",
    "/.env",
  ]) {
    assert.equal((await get(path)).status, 400, path);
  }
  const leak = await get("/leak.txt");
  assert.equal(leak.status, 404);
  assert.equal(leak.body, "");
});
