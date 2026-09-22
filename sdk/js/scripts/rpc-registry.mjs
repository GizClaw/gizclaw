export function parseRegistry(proto, prefix, extension) {
  const entries = [];
  const pattern = new RegExp(`^\\s*${prefix}_[A-Z0-9_]+\\s*=\\s*(\\d+)\\s*\\[\\(${extension}\\)\\s*=\\s*\\{\\s*name:\\s*"([^"]+)"\\s+request:\\s*"(\\w+)"\\s+response:\\s*"(\\w+)"\\s*\\}\\s*(?:,\\s*deprecated\\s*=\\s*(true|false)\\s*)?\\]\\s*;`);
  const lines = proto.split(/\r?\n/);
  for (const [index, line] of lines.entries()) {
    const entry = pattern.exec(line);
    if (entry == null) {
      if (new RegExp(`^\\s*${prefix}_[A-Z0-9_]+\\s*=`).test(line) && !line.includes(`${prefix}_UNSPECIFIED`)) throw new Error(`registry entry missing ${extension}: ${line.trim()}`);
      continue;
    }
    entries.push({id:Number(entry[1]), method:entry[2], request:entry[3], response:entry[4], deprecation:entry[5] === "true" ? lines[index - 1]?.trim().replace(/^\/\/ Deprecated:\s*/, "") : undefined});
  }
  return entries;
}
