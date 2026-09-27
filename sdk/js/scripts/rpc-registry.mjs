export function parseRegistry(proto, prefix, extension) {
  const entries = [];
  const pattern = new RegExp(
    `^\\s*${prefix}_[A-Z0-9_]+\\s*=\\s*(\\d+)\\s*\\[\\(${extension}\\)\\s*=\\s*\\{\\s*name:\\s*"([^"]+)"\\s+request:\\s*"(\\w+)"\\s+response:\\s*"(\\w+)"\\s*\\}\\s*(?:,\\s*deprecated\\s*=\\s*(true|false)\\s*)?\\]\\s*;`,
  );
  const lines = proto.split(/\r?\n/);
  for (const [index, line] of lines.entries()) {
    const entry = pattern.exec(line);
    if (entry == null) {
      if (
        new RegExp(`^\\s*${prefix}_[A-Z0-9_]+\\s*=`).test(line) &&
        !line.includes(`${prefix}_UNSPECIFIED`)
      )
        throw new Error(`registry entry missing ${extension}: ${line.trim()}`);
      continue;
    }
    entries.push({
      id: Number(entry[1]),
      method: entry[2],
      request: entry[3],
      response: entry[4],
      deprecation:
        entry[5] === "true"
          ? lines[index - 1]?.trim().replace(/^\/\/ Deprecated:\s*/, "")
          : undefined,
    });
  }
  return entries;
}

// HWD options bind a read result and, for writable HWDs, a write pair.
export function parseHwdRegistry(proto) {
  const entries = [];
  const pattern =
    /^\s*CLIENT_HWD_[A-Z0-9_]+\s*=\s*(\d+)\s*\[\(client_hwd\)\s*=\s*\{\s*name:\s*"([^"]+)"\s+read_response:\s*"(\w+)"(?:\s+write_request:\s*"(\w+)"\s+write_response:\s*"(\w+)")?\s*\}\s*\]\s*;/;
  for (const line of proto.split(/\r?\n/)) {
    const match = pattern.exec(line);
    if (match != null) {
      entries.push({
        id: Number(match[1]),
        name: match[2],
        readResponse: match[3],
        writeRequest: match[4],
        writeResponse: match[5],
      });
    } else if (
      /^\s*CLIENT_HWD_[A-Z0-9_]+\s*=/.test(line) &&
      !line.includes("CLIENT_HWD_UNSPECIFIED")
    ) {
      throw new Error(`HWD registry entry missing client_hwd: ${line.trim()}`);
    }
  }
  return entries;
}
