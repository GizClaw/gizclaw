import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { guideDocuments } from "../src/guides.ts";
import { createKnowledgeIndex, type KnowledgeIndex } from "../src/knowledge.ts";

/** Reads the repository's guides of one locale, keyed by path under guides/. */
export function readGuides(locale = "zh"): Record<string, string> {
  // Resolved on use: importing the testing entry must not touch the disk.
  const GUIDES = fileURLToPath(new URL("../../../guides/", import.meta.url));
  const files: Record<string, string> = {};
  for (const name of readdirSync(`${GUIDES}${locale}`, { recursive: true })) {
    const path = `${locale}/${String(name)}`;
    if (path.endsWith(".md")) files[path] = readFileSync(GUIDES + path, "utf8");
  }
  return files;
}

let index: KnowledgeIndex | undefined;

/** The zh guides index, built once per process. */
export function guidesIndex(): KnowledgeIndex {
  index ??= createKnowledgeIndex(guideDocuments(readGuides()));
  return index;
}
