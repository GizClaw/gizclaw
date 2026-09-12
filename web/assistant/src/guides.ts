import type { KnowledgeDocument } from "./knowledge.ts";

/** Where the project guides are published. */
export const GUIDES_SITE = "https://gizclaw.github.io/gizclaw/";

// Pages the guide site excludes from its build.
const UNPUBLISHED = /(^|\/)reviewing\/examples\//;

/**
 * Turns the project guides into the assistant's knowledge base. Files are
 * keyed by their path under guides/, for example "zh/developing/monitor.md";
 * each document links to its page on the published site.
 */
export function guideDocuments(
  files: Record<string, string>,
): KnowledgeDocument[] {
  return Object.entries(files)
    .filter(([path]) => path.endsWith(".md") && !UNPUBLISHED.test(path))
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([path, raw]) => {
      const text = raw.replace(/^---\r?\n[\s\S]*?\r?\n---\r?\n/, "");
      return {
        id: path,
        title:
          /^#\s+(.+)$/m
            .exec(text)?.[1]
            ?.replace(/<[^>]*>/g, "")
            .trim() ?? path,
        source: `guides/${path}`,
        url: guideUrl(path),
        text,
      };
    });
}

function guideUrl(path: string): string {
  const page = path.replace(/\.md$/, "").replace(/(^|\/)index$/, "$1");
  return `${GUIDES_SITE}${page}`;
}
