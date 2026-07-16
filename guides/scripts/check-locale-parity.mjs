import { readdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const guidesRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

async function markdownPaths(root, relative = "") {
  const entries = await readdir(path.join(root, relative), { withFileTypes: true });
  const paths = [];

  for (const entry of entries) {
    const child = path.join(relative, entry.name);
    if (entry.isDirectory()) {
      paths.push(...await markdownPaths(root, child));
    } else if (entry.isFile() && entry.name.endsWith(".md")) {
      paths.push(child.split(path.sep).join("/"));
    }
  }

  return paths.sort();
}

const [chinesePaths, englishPaths] = await Promise.all([
  markdownPaths(path.join(guidesRoot, "zh")),
  markdownPaths(path.join(guidesRoot, "en")),
]);

const chinese = new Set(chinesePaths);
const english = new Set(englishPaths);
const missingEnglish = chinesePaths.filter((file) => !english.has(file));
const extraEnglish = englishPaths.filter((file) => !chinese.has(file));

if (missingEnglish.length || extraEnglish.length) {
  if (missingEnglish.length) {
    console.error("Missing English guide pages:");
    for (const file of missingEnglish) console.error(`- ${file}`);
  }
  if (extraEnglish.length) {
    console.error("English guide pages without a Chinese counterpart:");
    for (const file of extraEnglish) console.error(`- ${file}`);
  }
  process.exitCode = 1;
} else {
  console.log(`Locale parity verified for ${chinesePaths.length} Markdown pages.`);
}
