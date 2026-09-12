/** A document in the assistant's knowledge base, written in Markdown. */
export type KnowledgeDocument = {
  id: string;
  title: string;
  /** Where the document comes from, e.g. its path in the repository. */
  source: string;
  /** Where a reader can open the document. */
  url?: string;
  text: string;
};

/** One searchable section of a document. */
export type KnowledgePassage = {
  documentId: string;
  title: string;
  source: string;
  url?: string;
  /** The heading path of the section inside its document. */
  heading: string;
  text: string;
  score: number;
};

export type KnowledgeIndex = {
  search(query: string, limit?: number): KnowledgePassage[];
  /** Passages indexed, for display. */
  readonly size: number;
};

const MAX_PASSAGE_CHARS = 700;
const K1 = 1.2;
const B = 0.75;

/**
 * Splits text into search terms without a dictionary: CJK runs become
 * overlapping character pairs, other runs become lowercase words, and words
 * joined by underscores or dots also yield their parts so both
 * "DEBUG_ACCESS_FORBIDDEN" and "forbidden" match.
 */
export function tokenize(text: string): string[] {
  const terms: string[] = [];
  const pattern = /([一-鿿㐀-䶿]+)|([\p{L}\p{N}_.-]+)/gu;
  for (const match of text.toLowerCase().matchAll(pattern)) {
    if (match[1]) {
      const run = match[1];
      if (run.length === 1) terms.push(run);
      for (let index = 0; index + 1 < run.length; index++) {
        terms.push(run.slice(index, index + 2));
      }
    } else if (match[2]) {
      const word = match[2].replace(/^[._-]+|[._-]+$/g, "");
      if (word === "") continue;
      terms.push(word);
      const parts = word.split(/[_.-]+/).filter(Boolean);
      if (parts.length > 1) terms.push(...parts);
    }
  }
  return terms;
}

/** Cuts a Markdown document into sections by heading, then by length. */
export function chunkDocument(
  document: KnowledgeDocument,
): Omit<KnowledgePassage, "score">[] {
  const passages: Omit<KnowledgePassage, "score">[] = [];
  const headings: string[] = [];
  let buffer: string[] = [];
  const flush = () => {
    const text = buffer.join("\n").trim();
    buffer = [];
    if (text === "") return;
    for (const piece of splitByLength(text, MAX_PASSAGE_CHARS)) {
      passages.push({
        documentId: document.id,
        title: document.title,
        source: document.source,
        ...(document.url === undefined ? {} : { url: document.url }),
        heading: headings.filter(Boolean).join(" / "),
        text: piece,
      });
    }
  };
  for (const line of document.text.split(/\r?\n/)) {
    const heading = /^(#{1,6})\s+(.*)$/.exec(line);
    if (heading) {
      flush();
      const level = heading[1].length;
      headings.length = level - 1;
      // Guides decorate headings with components such as <Badge />.
      headings[level - 1] = heading[2].replace(/<[^>]*>/g, "").trim();
      continue;
    }
    buffer.push(line);
  }
  flush();
  return passages;
}

function splitByLength(text: string, limit: number): string[] {
  if (text.length <= limit) return [text];
  const pieces: string[] = [];
  let current = "";
  for (const paragraph of text.split(/\n{2,}/)) {
    if (current && current.length + paragraph.length + 2 > limit) {
      pieces.push(current);
      current = "";
    }
    if (paragraph.length > limit) {
      for (let start = 0; start < paragraph.length; start += limit) {
        pieces.push(paragraph.slice(start, start + limit));
      }
      continue;
    }
    current = current ? `${current}\n\n${paragraph}` : paragraph;
  }
  if (current) pieces.push(current);
  return pieces;
}

/** Builds a BM25 index over every section of the given documents. */
export function createKnowledgeIndex(
  documents: KnowledgeDocument[],
): KnowledgeIndex {
  const passages = documents.flatMap(chunkDocument);
  const termCounts = passages.map((passage) => {
    const counts = new Map<string, number>();
    for (const term of tokenize(
      `${passage.title} ${passage.heading} ${passage.text}`,
    )) {
      counts.set(term, (counts.get(term) ?? 0) + 1);
    }
    return counts;
  });
  const lengths = termCounts.map((counts) =>
    [...counts.values()].reduce((sum, count) => sum + count, 0),
  );
  const averageLength =
    lengths.reduce((sum, length) => sum + length, 0) /
    Math.max(1, lengths.length);
  const documentFrequency = new Map<string, number>();
  for (const counts of termCounts) {
    for (const term of counts.keys()) {
      documentFrequency.set(term, (documentFrequency.get(term) ?? 0) + 1);
    }
  }

  return {
    size: passages.length,
    search(query: string, limit = 5): KnowledgePassage[] {
      const terms = [...new Set(tokenize(query))];
      if (terms.length === 0) return [];
      const scored: KnowledgePassage[] = [];
      termCounts.forEach((counts, index) => {
        let score = 0;
        for (const term of terms) {
          const frequency = counts.get(term);
          if (!frequency) continue;
          const df = documentFrequency.get(term) ?? 0;
          const idf = Math.log(1 + (passages.length - df + 0.5) / (df + 0.5));
          score +=
            (idf * frequency * (K1 + 1)) /
            (frequency + K1 * (1 - B + (B * lengths[index]) / averageLength));
        }
        if (score > 0) scored.push({ ...passages[index], score });
      });
      return scored
        .sort((left, right) => right.score - left.score)
        .slice(0, limit);
    },
  };
}
