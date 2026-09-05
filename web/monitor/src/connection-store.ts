// Browser-local encrypted connection records. Same-origin scripts can use the
// stored CryptoKey; this is not an OS keychain or protection from same-origin XSS.
function database(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    if (!globalThis.indexedDB || !globalThis.crypto?.subtle) {
      reject(new Error("此浏览器无法保存连接，请使用 HTTPS 或 localhost"));
      return;
    }
    let blocked = false;
    const req = indexedDB.open("gizclaw-monitor-connections", 1);
    req.onupgradeneeded = () => req.result.createObjectStore("connections");
    req.onerror = () => reject(req.error);
    req.onblocked = () => {
      blocked = true;
      reject(new Error("本地连接数据库被其他页面占用"));
    };
    req.onsuccess = () => {
      if (blocked) {
        req.result.close();
        return;
      }
      req.result.onversionchange = () => req.result.close();
      resolve(req.result);
    };
  });
}
async function record(
  mode: IDBTransactionMode,
  name: string,
  value?: unknown,
): Promise<unknown> {
  const db = await database();
  try {
    return await new Promise((resolve, reject) => {
      const tx = db.transaction("connections", mode),
        store = tx.objectStore("connections");
      const req =
        mode === "readonly"
          ? store.get(name)
          : value === undefined
            ? store.delete(name)
            : store.put(value, name);
      let result: unknown;
      req.onsuccess = () => {
        result = req.result;
      };
      tx.oncomplete = () => resolve(result);
      tx.onerror = () => reject(tx.error);
      tx.onabort = () => reject(tx.error ?? new Error("本地存储已取消"));
    });
  } finally {
    db.close();
  }
}
// Serialize the whole encryption/write operation so logout cannot be overtaken
// by a connection save still generating its key. Callers receive each error.
let writes: Promise<void> = Promise.resolve();
function write(operation: () => Promise<void>): Promise<void> {
  const result = writes.then(operation);
  writes = result.catch(() => undefined);
  return result;
}
export function saveConnection(name: string, credential: string) {
  return write(async () => {
    if (!crypto.subtle) throw new Error("保存连接需要 HTTPS 或 localhost");
    const key = await crypto.subtle.generateKey(
      { name: "AES-GCM", length: 256 },
      false,
      ["encrypt", "decrypt"],
    );
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const encrypted = await crypto.subtle.encrypt(
      { name: "AES-GCM", iv },
      key,
      new TextEncoder().encode(credential),
    );
    await record("readwrite", name, { key, iv, encrypted });
  });
}
export async function readConnection(name: string): Promise<string> {
  const item = await record("readonly", name);
  if (item === undefined) return "";
  if (
    typeof item !== "object" ||
    item === null ||
    !("key" in item) ||
    !("iv" in item) ||
    !("encrypted" in item) ||
    !(item.key instanceof CryptoKey) ||
    !(item.iv instanceof Uint8Array) ||
    !(item.encrypted instanceof ArrayBuffer)
  )
    throw new Error("本地连接记录无效，请退出并清除");
  return new TextDecoder().decode(
    await crypto.subtle.decrypt(
      { name: "AES-GCM", iv: new Uint8Array(item.iv) },
      item.key,
      item.encrypted,
    ),
  );
}
export function clearConnection(name: string) {
  return write(async () => {
    await record("readwrite", name);
  });
}
