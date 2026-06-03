import type {
  ApiEnvelope,
  AdminHROptions,
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
  EventAsset,
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
import {
  ApiError,
  adminEventPath,
  api,
  apiList,
  authHeaders,
  encoded,
  eventPath,
  headersFor,
  logApi,
  post,
  postForm,
  setProviderToken,
} from "./http";
import type { OpsDashboard } from "./ops-contracts";

export { employees } from "./demo-data";
export {
  ApiError,
  captureProviderToken,
  restoreProviderToken,
  setApiObserver,
  setProviderToken,
  setProviderTokenProvider,
} from "./http";

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

export const adminHROptions = () =>
  api<AdminHROptions>("/api/v1/admin/hr/options");

export function createEvent(body: CreateEventRequest) {
  return post<EventSummary>("/api/v1/admin/events", body);
}

export function uploadEventPoster(eventID: string, file: File) {
  const body = new FormData();
  body.append("poster", file);
  return postForm<EventAsset>(
    `/api/v1/admin/events/${encoded(eventID)}/poster`,
    body,
  );
}

export function eventPosterUrl(eventID: string) {
  return `/api/v1/events/${encoded(eventID)}/poster`;
}

export async function eventPosterBlob(eventID: string) {
  const path = eventPosterUrl(eventID);
  return fetchBlobResource(path, authHeaders(), {
    missingLog: { poster: "missing" },
    successLog: { poster: true },
  });
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
  return fetchBlobResource(path, headersFor(), {
    successLog: { download: true },
  });
}

type BlobResourceLogOptions = {
  missingLog?: Record<string, unknown>;
  successLog: Record<string, unknown>;
};

function fetchBlobResource(
  path: string,
  headers: HeadersInit,
  logOptions: BlobResourceLogOptions & { missingLog: Record<string, unknown> },
): Promise<Blob | null>;
function fetchBlobResource(
  path: string,
  headers: HeadersInit,
  logOptions: BlobResourceLogOptions,
): Promise<Blob>;
async function fetchBlobResource(
  path: string,
  headers: HeadersInit,
  logOptions: BlobResourceLogOptions,
): Promise<Blob | null> {
  const response = await fetch(path, {
    credentials: "same-origin",
    headers,
  });
  const label = `GET ${path}`;
  if (response.status === 404 && logOptions.missingLog) {
    logApi(label, response.status, false, null, logOptions.missingLog);
    return null;
  }
  if (!response.ok) {
    const envelope = await blobErrorEnvelope(response);
    logApi(label, response.status, false, null, envelope);
    throw new ApiError(response.status, envelope);
  }
  const blob = await response.blob();
  logApi(label, response.status, true, null, {
    ...logOptions.successLog,
    content_type: response.headers.get("Content-Type") || "",
  });
  return blob;
}

async function blobErrorEnvelope(
  response: Response,
): Promise<ApiEnvelope<unknown>> {
  const contentType = response.headers.get("Content-Type") || "";
  if (contentType.includes("application/json")) {
    return (await response.json()) as ApiEnvelope<unknown>;
  }
  return {
    success: false,
    data: null,
    error: await response.text(),
  };
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
