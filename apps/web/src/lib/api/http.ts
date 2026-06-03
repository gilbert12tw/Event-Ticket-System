import type { ApiEnvelope, ApiLogEntry } from "./contracts";
import { redact } from "./redaction";

type RequestOptions = Omit<RequestInit, "headers" | "body"> & {
  body?: unknown;
};

export type ApiObserver = (entry: ApiLogEntry) => void;
export type ProviderTokenProvider = () => string | null | undefined;
export type ProviderTokenSnapshot = Readonly<{
  explicitProviderToken: string | null;
}>;

let observer: ApiObserver | null = null;
let apiLogSequence = 0;
let explicitProviderToken: string | null = null;
let providerTokenProvider: ProviderTokenProvider = defaultProviderTokenProvider;

export function setApiObserver(next: ApiObserver | null) {
  observer = next;
}

export function setProviderToken(token: string | null) {
  explicitProviderToken = token;
}

export function captureProviderToken(): ProviderTokenSnapshot {
  return { explicitProviderToken };
}

export function restoreProviderToken(snapshot: ProviderTokenSnapshot) {
  explicitProviderToken = snapshot.explicitProviderToken;
}

export function setProviderTokenProvider(next: ProviderTokenProvider | null) {
  providerTokenProvider = next ?? defaultProviderTokenProvider;
}

export class ApiError extends Error {
  status: number;
  response: ApiEnvelope<unknown>;

  constructor(status: number, response: ApiEnvelope<unknown>) {
    super(response.error || "request failed");
    this.status = status;
    this.response = response;
  }
}

export function headersFor(): HeadersInit {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  const providerToken = currentProviderToken();
  if (providerToken) {
    headers.Authorization = `Bearer ${providerToken}`;
  }
  return headers;
}

function defaultProviderTokenProvider() {
  const token = (
    globalThis as typeof globalThis & {
      __CETS_PROVIDER_TOKEN__?: string | null;
    }
  ).__CETS_PROVIDER_TOKEN__;
  return typeof token === "string" ? token : "";
}

function currentProviderToken() {
  return (explicitProviderToken ?? providerTokenProvider() ?? "").trim();
}

export async function api<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const method = options.method || "GET";
  const requestBody = options.body ?? null;
  try {
    const response = await fetch(path, {
      ...options,
      credentials: "same-origin",
      headers: headersFor(),
      body:
        options.body === undefined ? undefined : JSON.stringify(options.body),
    });
    const contentType = response.headers.get("Content-Type") || "";
    const envelope = contentType.includes("application/json")
      ? ((await response.json()) as ApiEnvelope<T>)
      : ({
          success: false,
          data: null as T,
          error: await response.text(),
        } satisfies ApiEnvelope<T>);

    logApi(
      `${method} ${path}`,
      response.status,
      response.ok,
      requestBody,
      envelope,
    );
    if (!response.ok) {
      throw new ApiError(response.status, envelope as ApiEnvelope<unknown>);
    }
    return envelope.data;
  } catch (error) {
    if (error instanceof ApiError) throw error;
    logApi(`${method} ${path}`, "ERR", false, requestBody, {
      error: error instanceof Error ? error.message : String(error),
    });
    throw error;
  }
}

export async function apiList<T>(path: string, options: RequestOptions = {}) {
  return (await api<T[] | null>(path, options)) ?? [];
}

export function encoded(value: string) {
  return encodeURIComponent(value);
}

export function eventPath(eventID: string, suffix = "") {
  return `/api/v1/events/${encoded(eventID)}${suffix}`;
}

export function adminEventPath(eventID: string, suffix = "") {
  return `/api/v1/admin/events/${encoded(eventID)}${suffix}`;
}

export function post<T>(path: string, body: unknown = {}) {
  return api<T>(path, { method: "POST", body });
}

export async function postForm<T>(path: string, body: FormData) {
  const response = await fetch(path, {
    method: "POST",
    credentials: "same-origin",
    headers: authHeaders(),
    body,
  });
  const contentType = response.headers.get("Content-Type") || "";
  const envelope = contentType.includes("application/json")
    ? ((await response.json()) as ApiEnvelope<T>)
    : ({
        success: false,
        data: null as T,
        error: await response.text(),
      } satisfies ApiEnvelope<T>);
  logApi(
    `POST ${path}`,
    response.status,
    response.ok,
    { form: true },
    envelope,
  );
  if (!response.ok) {
    throw new ApiError(response.status, envelope as ApiEnvelope<unknown>);
  }
  return envelope.data;
}

export function authHeaders(): HeadersInit {
  const providerToken = currentProviderToken();
  return providerToken ? { Authorization: `Bearer ${providerToken}` } : {};
}

function nextApiLogID() {
  apiLogSequence = (apiLogSequence + 1) % Number.MAX_SAFE_INTEGER;
  return `${Date.now()}-${apiLogSequence}`;
}

export function logApi(
  label: string,
  status: number | "ERR",
  ok: boolean,
  requestBody: unknown,
  responseBody: unknown,
) {
  observer?.({
    id: nextApiLogID(),
    label,
    status,
    ok,
    requestBody: redact(requestBody),
    responseBody: redact(responseBody),
    createdAt: new Date().toISOString(),
  });
}
