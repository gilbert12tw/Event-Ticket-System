import type {
  ApiEnvelope,
  ApiLogEntry,
  AuditLog,
  AuditLogFilters,
  AuthBootstrap,
  AuthMeClaims,
  AuthSession,
  BookingResponse,
  CheckinResponse,
  CreateEventRequest,
  DemoClockSnapshot,
  DemoClockUpdateRequest,
  EligibilityDecision,
  EligibilityImpactReview,
  EligibilityPreviewRequest,
  EligibilityPreviewResponse,
  EventSummary,
  LotteryRun,
  LotteryRunRequest,
  MockProviderToken,
  NotificationDelivery,
  NotificationPreferences,
  OfflineCheckinPackage,
  OfflineCheckinSyncRequest,
  OfflineCheckinSyncResponse,
  PromoteWaitlistResponse,
  ReportExport,
  ReportExportRequest,
  ReportRow,
  RegistrationDetail,
  ResolveImpactReviewRequest,
  Ticket,
  UpdateEligibilityRequest,
  UpdateNotificationPreferencesRequest,
  UpdateEventRequest,
} from "./contracts";
import type { OpsDashboard } from "./ops-contracts";
import { redact } from "./redaction";

export { employees } from "./demo-data";

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

function headersFor(): HeadersInit {
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

async function api<T>(path: string, options: RequestOptions = {}): Promise<T> {
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

async function apiList<T>(path: string, options: RequestOptions = {}) {
  return (await api<T[] | null>(path, options)) ?? [];
}

function encoded(value: string) {
  return encodeURIComponent(value);
}

function eventPath(eventID: string, suffix = "") {
  return `/api/v1/events/${encoded(eventID)}${suffix}`;
}

function adminEventPath(eventID: string, suffix = "") {
  return `/api/v1/admin/events/${encoded(eventID)}${suffix}`;
}

function post<T>(path: string, body: unknown = {}) {
  return api<T>(path, { method: "POST", body });
}

function nextApiLogID() {
  apiLogSequence = (apiLogSequence + 1) % Number.MAX_SAFE_INTEGER;
  return `${Date.now()}-${apiLogSequence}`;
}

function logApi(
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

export function authSessionFromClaims(claims: AuthMeClaims): AuthSession {
  return {
    actor: {
      id: claims.employee_id,
      role: claims.mapped_roles[0] || "employee",
    },
    expires_at: "",
    claims,
    source: "provider",
  };
}

export const me = (): Promise<AuthSession> =>
  api<AuthMeClaims>("/api/v1/auth/me").then(authSessionFromClaims);

export const authBootstrap = () => api<AuthBootstrap>("/api/v1/auth/bootstrap");

export const getDemoClock = () =>
  api<DemoClockSnapshot>("/api/v1/debug/demo-clock");

export function updateDemoClock(body: DemoClockUpdateRequest) {
  return api<DemoClockSnapshot>("/api/v1/debug/demo-clock", {
    method: "PUT",
    body,
  });
}

export function mockProviderToken(profileID: string) {
  return post<MockProviderToken>("/api/v1/auth/mock-provider-token", {
    profile_id: profileID,
  });
}

export async function selectMockProfile(
  profileID: string,
): Promise<AuthSession> {
  const token = await mockProviderToken(profileID);
  setProviderToken(token.provider_token);
  return me();
}

export const clearProviderToken = () => setProviderToken(null);

export function seedDemo() {
  return post<{ status: string }>("/api/v1/admin/seed-demo");
}

export function createEvent(body: CreateEventRequest) {
  return post<EventSummary>("/api/v1/admin/events", body);
}

export const listAdminEvents = () =>
  apiList<EventSummary>("/api/v1/admin/events");

export const listEvents = () => apiList<EventSummary>("/api/v1/events");

export function getEvent(eventID: string) {
  return api<EventSummary>(eventPath(eventID));
}

export function checkEligibility(eventID: string) {
  return api<EligibilityDecision>(eventPath(eventID, "/eligibility"));
}

export function updateEvent(eventID: string, body: UpdateEventRequest) {
  return api<EventSummary>(adminEventPath(eventID), { method: "PATCH", body });
}

export function changeEventState(
  eventID: string,
  status: string,
  reason: string,
) {
  return post<EventSummary>(adminEventPath(eventID, "/state"), {
    status,
    reason,
  });
}

export function duplicateEvent(eventID: string) {
  return post<EventSummary>(adminEventPath(eventID, "/duplicate"));
}

export function archiveEvent(eventID: string) {
  return api<EventSummary>(adminEventPath(eventID), { method: "DELETE" });
}

export function previewEligibility(
  eventID: string,
  body: EligibilityPreviewRequest,
) {
  return post<EligibilityPreviewResponse>(
    adminEventPath(eventID, "/eligibility/preview"),
    body,
  );
}

export function updateEligibility(
  eventID: string,
  body: UpdateEligibilityRequest,
) {
  return api<EligibilityPreviewResponse>(
    adminEventPath(eventID, "/eligibility"),
    { method: "PUT", body },
  );
}

export function listEligibilityImpactReviews() {
  return apiList<EligibilityImpactReview>(
    "/api/v1/admin/eligibility-impact-reviews",
  );
}

export function resolveEligibilityImpactReview(
  reviewID: string,
  body: ResolveImpactReviewRequest,
) {
  return post<EligibilityImpactReview>(
    `/api/v1/admin/eligibility-impact-reviews/${encoded(reviewID)}/resolve`,
    body,
  );
}

export function bookEvent(
  eventID: string,
  idempotencyKey: string,
  familyCount = 0,
) {
  return post<BookingResponse>(eventPath(eventID, "/bookings"), {
    idempotency_key: idempotencyKey,
    family_count: familyCount,
  });
}

export function cancelMyRegistration(
  registrationID: string,
  reason: string,
  idempotencyKey: string,
) {
  return post<BookingResponse>(
    `/api/v1/me/registrations/${encoded(registrationID)}/cancel`,
    {
      reason,
      idempotency_key: idempotencyKey,
    },
  );
}

export function cancelRegistration(
  eventID: string,
  registrationID: string,
  reason: string,
  idempotencyKey: string,
) {
  return post<BookingResponse>(
    adminEventPath(eventID, `/registrations/${encoded(registrationID)}/cancel`),
    {
      reason,
      idempotency_key: idempotencyKey,
    },
  );
}

export function listRegistrations(eventID: string) {
  return apiList<RegistrationDetail>(adminEventPath(eventID, "/registrations"));
}

export function promoteWaitlist(eventID: string) {
  return post<PromoteWaitlistResponse>(
    adminEventPath(eventID, "/waitlist/promote"),
  );
}

export function runLottery(eventID: string, body: LotteryRunRequest) {
  return post<LotteryRun>(adminEventPath(eventID, "/lottery-runs"), body);
}

export const listTickets = () => apiList<Ticket>("/api/v1/me/tickets");

export function getTicket(ticketID: string) {
  return api<Ticket>(`/api/v1/tickets/${encoded(ticketID)}`);
}

export function checkIn(
  signedToken: string,
  deviceID: string,
  eventID: string,
  holderMismatchReason = "",
) {
  const body: Record<string, string> = {
    signed_token: signedToken,
    device_id: deviceID,
    event_id: eventID.trim(),
  };
  if (holderMismatchReason.trim()) {
    body.holder_mismatch_reason = holderMismatchReason.trim();
  }
  return post<CheckinResponse>("/api/v1/checkins", body);
}

export function revokeTicket(ticketID: string, reason: string) {
  return post<Ticket>(`/api/v1/admin/tickets/${encoded(ticketID)}/revoke`, {
    reason,
  });
}

export function offlineCheckinPackage(eventID: string, deviceID: string) {
  const params = new URLSearchParams();
  if (deviceID.trim()) params.set("device_id", deviceID.trim());
  const query = params.toString();
  const querySuffix = query ? `?${query}` : "";
  return api<OfflineCheckinPackage>(
    `/api/v1/checkins/events/${encoded(eventID)}/offline-package${querySuffix}`,
  );
}

export function syncOfflineCheckins(body: OfflineCheckinSyncRequest) {
  return post<OfflineCheckinSyncResponse>(
    "/api/v1/checkins/offline-sync",
    body,
  );
}

export function getNotificationPreferences() {
  return api<NotificationPreferences>("/api/v1/notifications/preferences");
}

export function updateNotificationPreferences(
  body: UpdateNotificationPreferencesRequest,
) {
  return api<NotificationPreferences>("/api/v1/notifications/preferences", {
    method: "PUT",
    body,
  });
}

export function listNotificationDeliveries() {
  return apiList<NotificationDelivery>(
    "/api/v1/admin/notifications/deliveries",
  );
}

export function retryNotificationDelivery(deliveryID: string) {
  return post<NotificationDelivery>(
    `/api/v1/admin/notifications/deliveries/${encoded(deliveryID)}/retry`,
  );
}

export const reports = () => apiList<ReportRow>("/api/v1/admin/reports");

export function createReportExport(body: ReportExportRequest) {
  return post<ReportExport>("/api/v1/admin/reports/exports", body);
}

export function getReportExport(exportID: string) {
  return api<ReportExport>(
    `/api/v1/admin/reports/exports/${encoded(exportID)}`,
  );
}

export async function downloadReportExport(exportID: string) {
  const path = `/api/v1/admin/reports/exports/${encoded(exportID)}/download`;
  const response = await fetch(path, {
    credentials: "same-origin",
    headers: headersFor(),
  });
  if (!response.ok) {
    const contentType = response.headers.get("Content-Type") || "";
    const envelope = contentType.includes("application/json")
      ? ((await response.json()) as ApiEnvelope<unknown>)
      : ({
          success: false,
          data: null,
          error: await response.text(),
        } satisfies ApiEnvelope<unknown>);
    logApi(`GET ${path}`, response.status, false, null, envelope);
    throw new ApiError(response.status, envelope);
  }
  const blob = await response.blob();
  logApi(`GET ${path}`, response.status, true, null, {
    download: true,
    content_type: response.headers.get("Content-Type") || "",
  });
  return blob;
}

export const getOpsDashboard = () =>
  api<OpsDashboard>("/api/v1/admin/ops/dashboard");

export function auditLogs(filters: AuditLogFilters = {}) {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && String(value).trim() !== "")
      params.set(key, String(value).trim());
  }
  const query = params.toString();
  const querySuffix = query ? `?${query}` : "";
  return apiList<AuditLog>(`/api/v1/admin/audit-logs${querySuffix}`);
}

export const readiness = (path: "/healthz" | "/readyz") =>
  api<{ status: string }>(path);
