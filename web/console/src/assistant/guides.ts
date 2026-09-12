import {
  createKnowledgeIndex,
  guideDocuments,
  type KnowledgeIndex,
} from "@gizclaw/assistant";

let index: Promise<KnowledgeIndex> | undefined;

/**
 * The assistant's knowledge base: the project guides, loaded and indexed on
 * the first search. A failed load is retried on the next search.
 */
export function loadGuidesIndex(): Promise<KnowledgeIndex> {
  index ??= import("./guides-content").then(({ GUIDE_FILES }) =>
    createKnowledgeIndex(guideDocuments(GUIDE_FILES)),
  );
  index.catch(() => {
    index = undefined;
  });
  return index;
}
