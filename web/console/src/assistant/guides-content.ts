// The zh project guides, bundled at build time into their own chunk.
const files = import.meta.glob<string>("../../../../guides/zh/**/*.md", {
  query: "?raw",
  import: "default",
  eager: true,
});

/** Guide Markdown keyed by path under guides/, e.g. "zh/developing/monitor.md". */
export const GUIDE_FILES: Record<string, string> = Object.fromEntries(
  Object.entries(files).map(([path, text]) => [
    path.slice(path.lastIndexOf("guides/") + "guides/".length),
    text,
  ]),
);
