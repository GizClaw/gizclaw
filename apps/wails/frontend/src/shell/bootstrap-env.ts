const assignmentPattern = /^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=/;

export function readBootstrapEnvValues(
  content: string,
  names: readonly string[],
): Record<string, string> {
  const allowed = new Set(names);
  const values: Record<string, string> = {};
  for (const line of content.split(/\r?\n/)) {
    const match = assignmentPattern.exec(line);
    if (!match || !allowed.has(match[1])) continue;
    const raw = line.slice(match[0].length).trim();
    values[match[1]] = readEditorValue(raw);
  }
  return values;
}

export function updateBootstrapEnvContent(
  content: string,
  names: readonly string[],
  values: Readonly<Record<string, string>>,
): string {
  const allowed = new Set(names);
  const written = new Set<string>();
  const lines: string[] = [];
  for (const line of content.split(/\r?\n/)) {
    const match = assignmentPattern.exec(line);
    const name = match?.[1];
    if (!name || !allowed.has(name)) {
      lines.push(line);
      continue;
    }
    if (written.has(name)) continue;
    written.add(name);
    const value = values[name] ?? "";
    if (value !== "") lines.push(`${name}=${JSON.stringify(value)}`);
  }
  while (lines.length > 0 && lines.at(-1) === "") lines.pop();
  for (const name of names) {
    if (written.has(name)) continue;
    const value = values[name] ?? "";
    if (value !== "") lines.push(`${name}=${JSON.stringify(value)}`);
  }
  return lines.length === 0 ? "" : `${lines.join("\n")}\n`;
}

function readEditorValue(raw: string): string {
  if (raw.startsWith('"')) {
    const end = findClosingQuote(raw, '"');
    if (end > 0) {
      try {
        return JSON.parse(raw.slice(0, end + 1)) as string;
      } catch {
        return raw.slice(1, end);
      }
    }
  }
  if (raw.startsWith("'")) {
    const end = raw.indexOf("'", 1);
    if (end > 0) return raw.slice(1, end);
  }
  const comment = raw.search(/\s#/);
  return (comment < 0 ? raw : raw.slice(0, comment)).trim();
}

function findClosingQuote(raw: string, quote: string): number {
  let escaped = false;
  for (let index = 1; index < raw.length; index += 1) {
    const character = raw[index];
    if (character === quote && !escaped) return index;
    if (character === "\\") {
      escaped = !escaped;
    } else {
      escaped = false;
    }
  }
  return -1;
}
