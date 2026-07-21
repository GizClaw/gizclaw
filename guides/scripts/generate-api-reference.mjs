import { cp, mkdir, rm } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const guidesRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repositoryRoot = path.resolve(guidesRoot, "..");
const source = path.join(repositoryRoot, "api", "http");
const output = path.join(guidesRoot, "public", "api", "specs");

await rm(output, { force: true, recursive: true });
await mkdir(path.dirname(output), { recursive: true });
await cp(source, output, { recursive: true });

console.log("Generated API Reference schemas from api/http.");
