import type { ConsoleRoute } from "../src/routes.ts";
import type { ActionRecord } from "../src/tools.ts";

/** A route the conversation must end on; unset fields are not checked. */
export type RouteExpectation = {
  page: ConsoleRoute["page"];
  id?: string;
  publicKey?: string;
  /** Substrings the logs query must contain. */
  queryIncludes?: string[];
};

export type Expectation = {
  /**
   * Tools that must have been called; arguments match as a subset. A call
   * must succeed unless error names the code it is expected to fail with.
   * Several names mean any of those tools satisfies the entry.
   */
  tools: {
    name: string | string[];
    arguments?: Record<string, unknown>;
    error?: string;
  }[];
  route?: RouteExpectation;
  /** Each entry must appear in the final reply; an array means any of them. */
  replyIncludes: (string | string[])[];
  /** Claims the reply must not make. */
  replyExcludes?: string[];
};

export type Outcome = {
  reply: string;
  actions: ActionRecord[];
  route: ConsoleRoute;
};

/** Checks one conversation outcome; an empty list means it passed. */
export function evaluate(expectation: Expectation, outcome: Outcome): string[] {
  const failures: string[] = [];
  for (const wanted of expectation.tools) {
    const names = Array.isArray(wanted.name) ? wanted.name : [wanted.name];
    const matched = outcome.actions.some(
      (action) =>
        names.includes(action.tool) &&
        action.error?.code === wanted.error &&
        isSubset(wanted.arguments ?? {}, action.arguments),
    );
    if (!matched) {
      const outcome = wanted.error
        ? `failing with ${wanted.error}`
        : "successful";
      failures.push(
        `missing ${outcome} ${names.join(" | ")} call with ${JSON.stringify(wanted.arguments ?? {})}`,
      );
    }
  }
  if (expectation.route && !routeMatches(expectation.route, outcome.route)) {
    failures.push(
      `ended on ${JSON.stringify(outcome.route)}, want ${JSON.stringify(expectation.route)}`,
    );
  }
  const reply = outcome.reply.toLowerCase();
  for (const fact of expectation.replyIncludes) {
    const options = Array.isArray(fact) ? fact : [fact];
    if (!options.some((option) => reply.includes(option.toLowerCase()))) {
      failures.push(`reply lacks ${options.join(" | ")}`);
    }
  }
  for (const claim of expectation.replyExcludes ?? []) {
    if (reply.includes(claim.toLowerCase()))
      failures.push(`reply claims ${claim}`);
  }
  return failures;
}

function routeMatches(
  expected: RouteExpectation,
  route: ConsoleRoute,
): boolean {
  if (expected.page !== route.page) return false;
  if (
    expected.id !== undefined &&
    (route.page !== "server" || route.id !== expected.id)
  ) {
    return false;
  }
  if (
    expected.publicKey !== undefined &&
    (route.page !== "peer" || route.publicKey !== expected.publicKey)
  ) {
    return false;
  }
  if (expected.queryIncludes) {
    const query = route.page === "logs" ? (route.query ?? "") : "";
    return expected.queryIncludes.every((part) => query.includes(part));
  }
  return true;
}

function isSubset(expected: unknown, actual: unknown): boolean {
  if (expected === null || typeof expected !== "object")
    return expected === actual;
  if (actual === null || typeof actual !== "object") return false;
  return Object.entries(expected).every(([key, value]) =>
    isSubset(value, (actual as Record<string, unknown>)[key]),
  );
}
