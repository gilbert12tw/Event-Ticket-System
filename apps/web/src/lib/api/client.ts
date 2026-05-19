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
import { redact } from "./redaction";

export { employees } from "./demo-data";

type RequestOptions = Omit<RequestInit, "headers" | "body"> & {
  body?: unknown;
};

export type ApiObserver = (entry: ApiLogEntry) => void;
export type ProviderTokenProvider = () => string | null | undefined;

let observer: ApiObserver | null = null;
let explicitProviderToken: string | null = null;
let providerTokenProvider: ProviderTokenProvider = defaultProviderTokenProvider;

export function setApiObserver(next: ApiObserver | null) {
  observer = next;
}

export function setProviderToken(token: string | null) {
  explicitProviderToken = token;
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

function logApi(
  label: string,
  status: number | "ERR",
  ok: boolean,
  requestBody: unknown,
  responseBody: unknown,
) {
  observer?.({
    id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
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

export function mockProviderToken(profileID: string) {
  return api<MockProviderToken>("/api/v1/auth/mock-provider-token", {
    method: "POST",
    body: {
      profile_id: profileID,
    },
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
  return api<{ status: string }>("/api/v1/admin/seed-demo", {
    method: "POST",
    body: {},
  });
}

export function createEvent(body: CreateEventRequest) {
  return api<EventSummary>("/api/v1/admin/events", {
    method: "POST",
    body,
  });
}

export const listAdminEvents = () =>
  apiList<EventSummary>("/api/v1/admin/events");

export const listEvents = () => apiList<EventSummary>("/api/v1/events");

export function getEvent(eventID: string) {
  return api<EventSummary>(`/api/v1/events/${encodeURIComponent(eventID)}`);
}

export function checkEligibility(eventID: string) {
  return api<EligibilityDecision>(
    `/api/v1/events/${encodeURIComponent(eventID)}/eligibility`,
  );
}

export function updateEvent(eventID: string, body: UpdateEventRequest) {
  return api<EventSummary>(
    `/api/v1/admin/events/${encodeURIComponent(eventID)}`,
    {
      method: "PATCH",
      body,
    },
  );
}

export function changeEventState(
  eventID: string,
  status: string,
  reason: string,
) {
  return api<EventSummary>(
    `/api/v1/admin/events/${encodeURIComponent(eventID)}/state`,
    {
      method: "POST",
      body: { status, reason },
    },
  );
}

export function duplicateEvent(eventID: string) {
  return api<EventSummary>(
    `/api/v1/admin/events/${encodeURIComponent(eventID)}/duplicate`,
    {
      method: "POST",
      body: {},
    },
  );
}

export function archiveEvent(eventID: string) {
  return api<EventSummary>(
    `/api/v1/admin/events/${encodeURIComponent(eventID)}`,
    {
      method: "DELETE",
    },
  );
}

export function previewEligibility(
  eventID: string,
  body: EligibilityPreviewRequest,
) {
  return api<EligibilityPreviewResponse>(
    `/api/v1/admin/events/${encodeURIComponent(eventID)}/eligibility/preview`,
    {
      method: "POST",
      body,
    },
  );
}

export function updateEligibility(
  eventID: string,
  body: UpdateEligibilityRequest,
) {
  return api<EligibilityPreviewResponse>(
    `/api/v1/admin/events/${encodeURIComponent(eventID)}/eligibility`,
    {
      method: "PUT",
      body,
    },
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
  return api<EligibilityImpactReview>(
    `/api/v1/admin/eligibility-impact-reviews/${encodeURIComponent(reviewID)}/resolve`,
    {
      method: "POST",
      body,
    },
  );
}

export function bookEvent(
  eventID: string,
  idempotencyKey: string,
  familyCount = 0,
) {
  return api<BookingResponse>(
    `/api/v1/events/${encodeURIComponent(eventID)}/bookings`,
    {
      method: "POST",
      body: {
        idempotency_key: idempotencyKey,
        family_count: familyCount,
      },
    },
  );
}

export function cancelMyRegistration(
  registrationID: string,
  reason: string,
  idempotencyKey: string,
) {
  return api<BookingResponse>(
    `/api/v1/me/registrations/${encodeURIComponent(registrationID)}/cancel`,
    {
      method: "POST",
      body: {
        reason,
        idempotency_key: idempotencyKey,
      },
    },
  );
}

export function cancelRegistration(
  eventID: string,
  registrationID: string,
  reason: string,
  idempotencyKey: string,
) {
  return api<BookingResponse>(
    `/api/v1/admin/events/${encodeURIComponent(eventID)}/registrations/${encodeURIComponent(registrationID)}/cancel`,
    {
      method: "POST",
      body: {
        reason,
        idempotency_key: idempotencyKey,
      },
    },
  );
}

export function listRegistrations(eventID: string) {
  return apiList<RegistrationDetail>(
    `/api/v1/admin/events/${encodeURIComponent(eventID)}/registrations`,
  );
}

export function promoteWaitlist(eventID: string) {
  return api<PromoteWaitlistResponse>(
    `/api/v1/admin/events/${encodeURIComponent(eventID)}/waitlist/promote`,
    {
      method: "POST",
      body: {},
    },
  );
}

export function runLottery(eventID: string, body: LotteryRunRequest) {
  return api<LotteryRun>(
    `/api/v1/admin/events/${encodeURIComponent(eventID)}/lottery-runs`,
    {
      method: "POST",
      body,
    },
  );
}

export const listTickets = () => apiList<Ticket>("/api/v1/me/tickets");

export function getTicket(ticketID: string) {
  return api<Ticket>(`/api/v1/tickets/${encodeURIComponent(ticketID)}`);
}

export function checkIn(signedToken: string, deviceID: string) {
  return api<CheckinResponse>("/api/v1/checkins", {
    method: "POST",
    body: {
      signed_token: signedToken,
      device_id: deviceID,
    },
  });
}

export function revokeTicket(ticketID: string, reason: string) {
  return api<Ticket>(
    `/api/v1/admin/tickets/${encodeURIComponent(ticketID)}/revoke`,
    {
      method: "POST",
      body: { reason },
    },
  );
}

export function offlineCheckinPackage(eventID: string, deviceID: string) {
  const params = new URLSearchParams();
  if (deviceID.trim()) params.set("device_id", deviceID.trim());
  const query = params.toString();
  return api<OfflineCheckinPackage>(
    `/api/v1/checkins/events/${encodeURIComponent(eventID)}/offline-package${query ? `?${query}` : ""}`,
  );
}

export function syncOfflineCheckins(body: OfflineCheckinSyncRequest) {
  return api<OfflineCheckinSyncResponse>("/api/v1/checkins/offline-sync", {
    method: "POST",
    body,
  });
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
  return api<NotificationDelivery>(
    `/api/v1/admin/notifications/deliveries/${encodeURIComponent(deliveryID)}/retry`,
    {
      method: "POST",
      body: {},
    },
  );
}

export const reports = () => apiList<ReportRow>("/api/v1/admin/reports");

export function createReportExport(body: ReportExportRequest) {
  return api<ReportExport>("/api/v1/admin/reports/exports", {
    method: "POST",
    body,
  });
}

export function getReportExport(exportID: string) {
  return api<ReportExport>(
    `/api/v1/admin/reports/exports/${encodeURIComponent(exportID)}`,
  );
}

export function auditLogs(filters: AuditLogFilters = {}) {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && String(value).trim() !== "")
      params.set(key, String(value).trim());
  }
  const query = params.toString();
  return apiList<AuditLog>(
    `/api/v1/admin/audit-logs${query ? `?${query}` : ""}`,
  );
}

export const readiness = (path: "/healthz" | "/readyz") =>
  api<{ status: string }>(path);
