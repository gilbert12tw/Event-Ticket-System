import type {
  ApiEnvelope,
  ApiLogEntry,
  AuditLog,
  AuditLogFilters,
  AuthSession,
  BookingResponse,
  CheckinResponse,
  CreateEventRequest,
  EligibilityCheckResult,
  EligibilityImpactReview,
  EligibilityPreviewRequest,
  EligibilityPreviewResponse,
  EventSummary,
  LotteryRun,
  LotteryRunRequest,
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
  UpdateEventRequest
} from "./contracts";

type RequestOptions = Omit<RequestInit, "headers" | "body"> & {
  body?: unknown;
};

export type ApiObserver = (entry: ApiLogEntry) => void;

let observer: ApiObserver | null = null;

export function setApiObserver(next: ApiObserver | null) {
  observer = next;
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

export const employees = [
  {
    employee_id: "E1001",
    full_name: "Ariel Chen",
    department: "Engineering",
    site: "Taipei",
    job_grade: 6,
    employment_status: "active"
  },
  {
    employee_id: "E1002",
    full_name: "Ben Lin",
    department: "Engineering",
    site: "Taipei",
    job_grade: 5,
    employment_status: "active"
  },
  {
    employee_id: "E2001",
    full_name: "Carla Wu",
    department: "Sales",
    site: "Taipei",
    job_grade: 4,
    employment_status: "active"
  }
];

function headersFor(): HeadersInit {
  return {
    "Content-Type": "application/json"
  };
}

async function api<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const method = options.method || "GET";
  try {
    const response = await fetch(path, {
      ...options,
      credentials: "same-origin",
      headers: headersFor(),
      body: options.body === undefined ? undefined : JSON.stringify(options.body)
    });
    const contentType = response.headers.get("Content-Type") || "";
    const envelope = contentType.includes("application/json")
      ? ((await response.json()) as ApiEnvelope<T>)
      : ({
          success: false,
          data: null as T,
          error: await response.text()
        } satisfies ApiEnvelope<T>);

    logApi(`${method} ${path}`, response.status, response.ok, envelope);
    if (!response.ok) {
      throw new ApiError(response.status, envelope as ApiEnvelope<unknown>);
    }
    return envelope.data;
  } catch (error) {
    if (error instanceof ApiError) throw error;
    logApi(`${method} ${path}`, "ERR", false, { error: error instanceof Error ? error.message : String(error) });
    throw error;
  }
}

async function apiList<T>(path: string, options: RequestOptions = {}): Promise<T[]> {
  return (await api<T[] | null>(path, options)) ?? [];
}

function logApi(label: string, status: number | "ERR", ok: boolean, payload: unknown) {
  observer?.({
    id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
    label,
    status,
    ok,
    payload: redact(payload),
    createdAt: new Date().toISOString()
  });
}

function redact(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(redact);
  if (!value || typeof value !== "object") return value;
  const output: Record<string, unknown> = {};
  for (const [key, raw] of Object.entries(value)) {
    if (key === "signed_token" || key === "qr_payload") {
      output[key] = typeof raw === "string" && raw.length > 0 ? "[redacted ticket token]" : raw;
    } else if (key === "cets_session" || key === "session" || key === "token") {
      output[key] = typeof raw === "string" && raw.length > 0 ? "[redacted session]" : raw;
    } else {
      output[key] = redact(raw);
    }
  }
  return output;
}

export function login(principalID: string) {
  return api<AuthSession>("/api/v1/auth/login", {
    method: "POST",
    body: {
      principal_id: principalID
    }
  });
}

export function me() {
  return api<AuthSession>("/api/v1/auth/me");
}

export function logout() {
  return api<{ status: string }>("/api/v1/auth/logout", {
    method: "POST",
    body: {}
  });
}

export function seedDemo() {
  return api<{ status: string }>("/api/v1/admin/seed-demo", {
    method: "POST",
    body: {}
  });
}

export function createEvent(body: CreateEventRequest) {
  return api<EventSummary>("/api/v1/admin/events", {
    method: "POST",
    body
  });
}

export function listAdminEvents() {
  return apiList<EventSummary>("/api/v1/admin/events");
}

export function listEvents(employeeID: string) {
  return apiList<EventSummary>(`/api/v1/events?employee_id=${encodeURIComponent(employeeID)}`);
}

export function getEvent(eventID: string, employeeID?: string) {
  const params = new URLSearchParams();
  if (employeeID) params.set("employee_id", employeeID);
  const query = params.toString();
  return api<EventSummary>(`/api/v1/events/${encodeURIComponent(eventID)}${query ? `?${query}` : ""}`);
}

export function checkEligibility(eventID: string, employeeID: string) {
  const params = new URLSearchParams({ employee_id: employeeID });
  return api<EligibilityCheckResult>(`/api/v1/events/${encodeURIComponent(eventID)}/eligibility?${params.toString()}`);
}

export function updateEvent(eventID: string, body: UpdateEventRequest) {
  return api<EventSummary>(`/api/v1/admin/events/${encodeURIComponent(eventID)}`, {
    method: "PATCH",
    body
  });
}

export function changeEventState(eventID: string, status: string, reason: string) {
  return api<EventSummary>(`/api/v1/admin/events/${encodeURIComponent(eventID)}/state`, {
    method: "POST",
    body: { status, reason }
  });
}

export function duplicateEvent(eventID: string) {
  return api<EventSummary>(`/api/v1/admin/events/${encodeURIComponent(eventID)}/duplicate`, {
    method: "POST",
    body: {}
  });
}

export function archiveEvent(eventID: string) {
  return api<EventSummary>(`/api/v1/admin/events/${encodeURIComponent(eventID)}`, {
    method: "DELETE"
  });
}

export function previewEligibility(eventID: string, body: EligibilityPreviewRequest) {
  return api<EligibilityPreviewResponse>(`/api/v1/admin/events/${encodeURIComponent(eventID)}/eligibility/preview`, {
    method: "POST",
    body
  });
}

export function updateEligibility(eventID: string, body: UpdateEligibilityRequest) {
  return api<EligibilityPreviewResponse>(`/api/v1/admin/events/${encodeURIComponent(eventID)}/eligibility`, {
    method: "PUT",
    body
  });
}

export function listEligibilityImpactReviews() {
  return apiList<EligibilityImpactReview>("/api/v1/admin/eligibility-impact-reviews");
}

export function resolveEligibilityImpactReview(reviewID: string, body: ResolveImpactReviewRequest) {
  return api<EligibilityImpactReview>(`/api/v1/admin/eligibility-impact-reviews/${encodeURIComponent(reviewID)}/resolve`, {
    method: "POST",
    body
  });
}

export function bookEvent(eventID: string, employeeID: string, idempotencyKey: string) {
  return api<BookingResponse>(`/api/v1/events/${eventID}/bookings`, {
    method: "POST",
    body: {
      employee_id: employeeID,
      idempotency_key: idempotencyKey
    }
  });
}

export function cancelRegistration(eventID: string, registrationID: string, reason: string, idempotencyKey: string) {
  return api<BookingResponse>(`/api/v1/events/${encodeURIComponent(eventID)}/bookings/${encodeURIComponent(registrationID)}/cancel`, {
    method: "POST",
    body: {
      reason,
      idempotency_key: idempotencyKey
    }
  });
}

export function listRegistrations(eventID: string) {
  return apiList<RegistrationDetail>(`/api/v1/admin/events/${encodeURIComponent(eventID)}/registrations`);
}

export function promoteWaitlist(eventID: string) {
  return api<PromoteWaitlistResponse>(`/api/v1/admin/events/${encodeURIComponent(eventID)}/waitlist/promote`, {
    method: "POST",
    body: {}
  });
}

export function runLottery(eventID: string, body: LotteryRunRequest) {
  return api<LotteryRun>(`/api/v1/admin/events/${encodeURIComponent(eventID)}/lottery-runs`, {
    method: "POST",
    body
  });
}

export function listTickets(employeeID: string) {
  return apiList<Ticket>(`/api/v1/employees/${employeeID}/tickets`);
}

export function getTicket(ticketID: string) {
  return api<Ticket>(`/api/v1/tickets/${encodeURIComponent(ticketID)}`);
}

export function checkIn(signedToken: string, deviceID: string) {
  return api<CheckinResponse>("/api/v1/checkins", {
    method: "POST",
    body: {
      signed_token: signedToken,
      device_id: deviceID
    }
  });
}

export function revokeTicket(ticketID: string, reason: string) {
  return api<Ticket>(`/api/v1/admin/tickets/${encodeURIComponent(ticketID)}/revoke`, {
    method: "POST",
    body: { reason }
  });
}

export function offlineCheckinPackage(eventID: string, deviceID: string) {
  const params = new URLSearchParams();
  if (deviceID.trim()) params.set("device_id", deviceID.trim());
  const query = params.toString();
  return api<OfflineCheckinPackage>(`/api/v1/checkins/events/${encodeURIComponent(eventID)}/offline-package${query ? `?${query}` : ""}`);
}

export function syncOfflineCheckins(body: OfflineCheckinSyncRequest) {
  return api<OfflineCheckinSyncResponse>("/api/v1/checkins/offline-sync", {
    method: "POST",
    body
  });
}

export function getNotificationPreferences() {
  return api<NotificationPreferences>("/api/v1/notifications/preferences");
}

export function updateNotificationPreferences(body: UpdateNotificationPreferencesRequest) {
  return api<NotificationPreferences>("/api/v1/notifications/preferences", {
    method: "PUT",
    body
  });
}

export function listNotificationDeliveries() {
  return apiList<NotificationDelivery>("/api/v1/admin/notifications/deliveries");
}

export function retryNotificationDelivery(deliveryID: string) {
  return api<NotificationDelivery>(`/api/v1/admin/notifications/deliveries/${encodeURIComponent(deliveryID)}/retry`, {
    method: "POST",
    body: {}
  });
}

export function reports() {
  return apiList<ReportRow>("/api/v1/admin/reports");
}

export function createReportExport(body: ReportExportRequest) {
  return api<ReportExport>("/api/v1/admin/reports/exports", {
    method: "POST",
    body
  });
}

export function getReportExport(exportID: string) {
  return api<ReportExport>(`/api/v1/admin/reports/exports/${encodeURIComponent(exportID)}`);
}

export function auditLogs(filters: AuditLogFilters = {}) {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && String(value).trim() !== "") params.set(key, String(value).trim());
  }
  const query = params.toString();
  return apiList<AuditLog>(`/api/v1/admin/audit-logs${query ? `?${query}` : ""}`);
}

export function readiness(path: "/healthz" | "/readyz") {
  return api<{ status: string }>(path);
}
