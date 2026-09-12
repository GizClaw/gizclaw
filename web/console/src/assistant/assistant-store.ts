import type { ActionRecord, AgentInputItem } from "@gizclaw/assistant";

/** One line of a conversation as the panel shows it. */
export type ChatEntry = {
  id: string;
  /** "notice" marks panel notes such as a context compaction. */
  role: "user" | "assistant" | "notice";
  text: string;
  actions?: ActionRecord[];
  error?: string;
};

export type ThreadSummary = {
  id: string;
  title: string;
  createdAt: number;
  updatedAt: number;
};

/** A conversation: what the panel shows and the context the model resumes. */
export type StoredThread = ThreadSummary & {
  entries: ChatEntry[];
  history: AgentInputItem[];
};

/** Named JSON records; the console backs them with its encrypted store. */
export type RecordBackend = {
  read<T>(name: string): Promise<T | undefined>;
  save(name: string, value: unknown): Promise<void>;
  remove(name: string): Promise<void>;
  /** Removes every record whose name starts with prefix. */
  removeAll(prefix?: string): Promise<void>;
};

export type AssistantStore = {
  /** Most recently updated first. */
  listThreads(): Promise<ThreadSummary[]>;
  loadThread(id: string): Promise<StoredThread | undefined>;
  /** Saves a thread and returns the updated list. */
  saveThread(thread: StoredThread): Promise<ThreadSummary[]>;
  deleteThread(id: string): Promise<ThreadSummary[]>;
  clearThreads(): Promise<void>;
};

/** Older threads beyond this count are dropped when a thread is saved. */
export const MAX_THREADS = 50;

const THREADS = "threads";
const threadRecord = (id: string) => `thread/${id}`;

export function createAssistantStore(records: RecordBackend): AssistantStore {
  // The thread list is read, changed and written back; serializing changes
  // keeps a late save of an aborted turn from dropping another thread.
  let changes: Promise<unknown> = Promise.resolve();
  const change = <T>(operation: () => Promise<T>): Promise<T> => {
    const result = changes.then(operation);
    changes = result.catch(() => undefined);
    return result;
  };
  const listThreads = async () =>
    (await records.read<ThreadSummary[]>(THREADS)) ?? [];

  return {
    listThreads,
    loadThread: (id) => records.read<StoredThread>(threadRecord(id)),
    saveThread: (thread) =>
      change(async () => {
        await records.save(threadRecord(thread.id), thread);
        const summary: ThreadSummary = {
          id: thread.id,
          title: thread.title,
          createdAt: thread.createdAt,
          updatedAt: thread.updatedAt,
        };
        const list = [
          summary,
          ...(await listThreads()).filter((item) => item.id !== thread.id),
        ].sort((left, right) => right.updatedAt - left.updatedAt);
        for (const dropped of list.slice(MAX_THREADS)) {
          await records.remove(threadRecord(dropped.id));
        }
        const kept = list.slice(0, MAX_THREADS);
        await records.save(THREADS, kept);
        return kept;
      }),
    deleteThread: (id) =>
      change(async () => {
        await records.remove(threadRecord(id));
        const list = (await listThreads()).filter((item) => item.id !== id);
        await records.save(THREADS, list);
        return list;
      }),
    // "thread" prefixes both the list and every thread record.
    clearThreads: () => change(() => records.removeAll("thread")),
  };
}

/** An in-memory backend, used where the browser store is unavailable. */
export function memoryRecords(): RecordBackend {
  const values = new Map<string, string>();
  return {
    read: async <T>(name: string) => {
      const value = values.get(name);
      return value === undefined ? undefined : (JSON.parse(value) as T);
    },
    save: async (name, value) => {
      values.set(name, JSON.stringify(value));
    },
    remove: async (name) => {
      values.delete(name);
    },
    removeAll: async (prefix = "") => {
      for (const name of values.keys()) {
        if (name.startsWith(prefix)) values.delete(name);
      }
    },
  };
}

/** The thread title: the start of its first question. */
export function threadTitle(entries: ChatEntry[]): string {
  const first = entries.find((entry) => entry.role === "user")?.text ?? "";
  const line = first.replace(/\s+/g, " ").trim();
  if (line === "") return "新对话";
  return line.length > 30 ? `${line.slice(0, 30)}…` : line;
}
