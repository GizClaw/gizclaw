export type ApiErrorPayload = {
  error: {
    message: string;
  };
};

export type ApiResult<T> = {
  data?: T;
  error?: unknown;
};

export async function expectData<T>(promise: Promise<ApiResult<T>>): Promise<T> {
  const result = await promise;
  if (result.error !== undefined || result.data === undefined) {
    throw result.error ?? new Error("Request failed");
  }
  return result.data;
}

export function isErrorPayload(value: unknown): value is ApiErrorPayload {
  if (typeof value !== "object" || value === null) {
    return false;
  }

  const record = value as Record<string, unknown>;
  if (typeof record.error !== "object" || record.error === null) {
    return false;
  }

  return typeof (record.error as Record<string, unknown>).message === "string";
}

export function toMessage(error: unknown): string {
  if (isErrorPayload(error)) {
    return error.error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
