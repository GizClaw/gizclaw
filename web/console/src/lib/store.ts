// Browser-local encrypted configuration. The AES key is nonextractable but
// usable by same-origin scripts: this is not an OS keychain, and it does not
// protect against same-origin XSS. Cleared on explicit logout.
const DATABASE = "gizclaw-console";
const STORE = "config";
const CONFIG_RECORD = "console-config";
const PEERS_RECORD = "console-peers";

function database(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    if (!globalThis.indexedDB || !globalThis.crypto?.subtle) {
      reject(new Error("此浏览器无法保存配置，请使用 HTTPS 或 localhost"));
      return;
    }
    let blocked = false;
    const request = indexedDB.open(DATABASE, 1);
    request.onupgradeneeded = () => request.result.createObjectStore(STORE);
    request.onerror = () => reject(request.error);
    request.onblocked = () => {
      blocked = true;
      reject(new Error("本地配置数据库被其他页面占用"));
    };
    request.onsuccess = () => {
      if (blocked) {
        request.result.close();
        return;
      }
      request.result.onversionchange = () => request.result.close();
      resolve(request.result);
    };
  });
}

async function record(
  mode: IDBTransactionMode,
  name: string | IDBKeyRange,
  value?: unknown,
): Promise<unknown> {
  const db = await database();
  try {
    return await new Promise((resolve, reject) => {
      const tx = db.transaction(STORE, mode);
      const store = tx.objectStore(STORE);
      const request =
        mode === "readonly"
          ? store.get(name)
          : value === undefined
            ? store.delete(name)
            : // Only deletions take a key range.
              store.put(value, name as string);
      let result: unknown;
      request.onsuccess = () => {
        result = request.result;
      };
      tx.oncomplete = () => resolve(result);
      tx.onerror = () => reject(tx.error);
      tx.onabort = () => reject(tx.error ?? new Error("本地存储已取消"));
    });
  } finally {
    db.close();
  }
}

// Serialize writes so a logout cannot be overtaken by a save still generating
// its key.
let writes: Promise<void> = Promise.resolve();
function write(operation: () => Promise<void>): Promise<void> {
  const result = writes.then(operation);
  writes = result.catch(() => undefined);
  return result;
}

function saveRecord(name: string, text: string): Promise<void> {
  return write(async () => {
    const key = await crypto.subtle.generateKey(
      { name: "AES-GCM", length: 256 },
      false,
      ["encrypt", "decrypt"],
    );
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const encrypted = await crypto.subtle.encrypt(
      { name: "AES-GCM", iv },
      key,
      new TextEncoder().encode(text),
    );
    await record("readwrite", name, { key, iv, encrypted });
  });
}

async function readRecord(name: string): Promise<string> {
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
  ) {
    throw new Error("本地配置记录无效，请退出并清除");
  }
  return new TextDecoder().decode(
    await crypto.subtle.decrypt(
      { name: "AES-GCM", iv: new Uint8Array(item.iv) },
      item.key,
      item.encrypted,
    ),
  );
}

function clearRecord(name: string | IDBKeyRange): Promise<void> {
  return write(async () => {
    await record("readwrite", name);
  });
}

// Assistant conversations and imported knowledge share one key prefix so a
// logout can drop them together.
const ASSISTANT_PREFIX = "console-assistant/";

/** Encrypted JSON records owned by the diagnostic assistant. */
export const assistantRecords = {
  async read<T>(name: string): Promise<T | undefined> {
    const text = await readRecord(ASSISTANT_PREFIX + name);
    return text === "" ? undefined : (JSON.parse(text) as T);
  },
  save: (name: string, value: unknown) =>
    saveRecord(ASSISTANT_PREFIX + name, JSON.stringify(value)),
  remove: (name: string) => clearRecord(ASSISTANT_PREFIX + name),
  /** Removes every record whose name starts with prefix. */
  removeAll: (prefix = "") =>
    clearRecord(
      IDBKeyRange.bound(
        ASSISTANT_PREFIX + prefix,
        `${ASSISTANT_PREFIX}${prefix}\uffff`,
      ),
    ),
};

export const saveConfig = (text: string) => saveRecord(CONFIG_RECORD, text);
export const readConfig = () => readRecord(CONFIG_RECORD);
export const savePeers = (text: string) => saveRecord(PEERS_RECORD, text);
export const readPeers = () => readRecord(PEERS_RECORD);
export const clearConfig = () => clearRecord(CONFIG_RECORD);

/** Logout clears every locally stored record, not just the configuration. */
export async function clearAll(): Promise<void> {
  await clearRecord(CONFIG_RECORD);
  await clearRecord(PEERS_RECORD);
  await assistantRecords.removeAll();
}
