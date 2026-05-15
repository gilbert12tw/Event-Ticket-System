import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  auditLogs,
  bookEvent,
  checkEligibility,
  createReportExport,
  getEvent,
  getReportExport,
  getNotificationPreferences,
  getTicket,
  listEligibilityImpactReviews,
  listEvents,
  listNotificationDeliveries,
  listTickets,
  me,
  mockProviderToken,
  offlineCheckinPackage,
  previewEligibility,
  resolveEligibilityImpactReview,
  retryNotificationDelivery,
  runLottery,
  setApiObserver,
  setProviderToken,
  setProviderTokenProvider,
  syncOfflineCheckins,
  updateEligibility,
  updateNotificationPreferences,
  type ApiLogEntry,
} from "@/lib/api";

function jsonResponse(body: unknown, init: ResponseInit = {}) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: {
      "Content-Type": "application/json",
    },
    ...init,
  });
}

describe("api client", () => {
  const fetchMock = vi.fn();
  const entries: ApiLogEntry[] = [];

  beforeEach(() => {
    entries.length = 0;
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
    setApiObserver((entry) => entries.push(entry));
  });

  afterEach(() => {
    setApiObserver(null);
    setProviderToken(null);
    setProviderTokenProvider(null);
    vi.unstubAllGlobals();
  });

  function mockSuccess(data: unknown = {}) {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        success: true,
        data,
        error: null,
      }),
    );
  }

  function fetchCall(index: number) {
    const [path, init] = fetchMock.mock.calls[index] as [
      string,
      RequestInit | undefined,
    ];
    const body =
      init?.body === undefined ? undefined : JSON.parse(String(init.body));
    return { path, init, body };
  }

  it("returns envelope data and redacts ticket tokens in API logs", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        success: true,
        data: [
          {
            ticket_id: "T-1",
            registration_id: "R-1",
            event_id: "EVT-1",
            employee_id: "E1001",
            status: "issued",
            signed_token: "ticket-secret",
            qr_payload: "qr-secret",
            issued_at: "2026-05-06T10:00:00Z",
          },
        ],
        error: null,
      }),
    );

    const tickets = await listTickets();

    expect(tickets).toHaveLength(1);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/me/tickets",
      expect.objectContaining({
        credentials: "same-origin",
        headers: {
          "Content-Type": "application/json",
        },
      }),
    );

    const payload = entries[0]?.payload as {
      data: Array<{ signed_token: string; qr_payload: string }>;
    };
    expect(payload.data[0].signed_token).toBe("[redacted ticket token]");
    expect(payload.data[0].qr_payload).toBe("[redacted ticket token]");
  });

  it("attaches provider bearer tokens from memory", async () => {
    setProviderToken("provider-secret");
    mockSuccess([]);

    await listEvents();

    expect(fetchCall(0).init).toMatchObject({
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer provider-secret",
      },
    });
  });

  it("maps provider claims to an authenticated session", async () => {
    mockSuccess({
      employee_id: "E1001",
      display_name: "Ariel Chen",
      role_claims: ["employee"],
      mapped_roles: ["employee"],
      department: "Engineering",
      site: "Taipei HQ",
      city: "Taipei",
      claims_status: "complete",
    });

    const session = await me();

    expect(session).toMatchObject({
      actor: { id: "E1001", role: "employee" },
      source: "provider",
      claims: { claims_status: "complete" },
    });
  });

  it("treats null list envelope data as an empty array", async () => {
    mockSuccess(null);

    const events = await listEvents();

    expect(events).toEqual([]);
    expect(fetchCall(0).path).toBe("/api/v1/events");
  });

  it("uses canonical own-data endpoints without caller-supplied employee_id", async () => {
    mockSuccess([]);
    await listEvents();
    mockSuccess({ event_id: "evt/1" });
    await getEvent("evt/1");
    mockSuccess({ event_id: "evt/1" });
    await checkEligibility("evt/1");
    mockSuccess({ event_id: "evt/1" });
    await bookEvent("evt/1", "book-1");
    mockSuccess([]);
    await listTickets();

    expect(fetchCall(0).path).toBe("/api/v1/events");
    expect(fetchCall(1).path).toBe("/api/v1/events/evt%2F1");
    expect(fetchCall(2).path).toBe("/api/v1/events/evt%2F1/eligibility");
    expect(fetchCall(3)).toMatchObject({
      path: "/api/v1/events/evt%2F1/bookings",
      body: { idempotency_key: "book-1", family_count: 0 },
    });
    expect(fetchCall(4).path).toBe("/api/v1/me/tickets");
    for (let index = 0; index < 5; index += 1) {
      expect(fetchCall(index).path).not.toContain("employee_id");
      expect(JSON.stringify(fetchCall(index).body ?? {})).not.toContain(
        "employee_id",
      );
    }
  });

  it("throws ApiError for unsuccessful JSON envelopes", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse(
        {
          success: false,
          data: null,
          error: "unauthorized",
        },
        { status: 401 },
      ),
    );

    let caught: unknown;
    try {
      await mockProviderToken("E1001");
    } catch (error) {
      caught = error;
    }

    expect(caught).toBeInstanceOf(ApiError);
    expect(caught).toMatchObject({
      status: 401,
      message: "unauthorized",
    });
  });

  it("redacts provider token fields in API log payloads", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        success: true,
        data: {
          provider_token: "provider-token-secret",
          expires_at: "2026-05-06T18:00:00Z",
          nested: {
            token: "nested-token-secret",
          },
        },
        error: null,
      }),
    );

    await mockProviderToken("E1001");

    const payload = entries[0]?.payload as {
      data: { provider_token: string; nested: { token: string } };
    };
    expect(payload.data.provider_token).toBe("[redacted provider token]");
    expect(payload.data.nested.token).toBe("[redacted session]");
  });

  it("calls production eligibility, lottery, ticket, report, and audit endpoints", async () => {
    const rule = {
      department: "Engineering",
      site: "Taipei",
      min_grade: 5,
      employment_status: "active",
    };

    mockSuccess({ event_id: "evt/1", match_count: 2, zero_match: false });
    await previewEligibility("evt/1", { rule });
    mockSuccess({ event_id: "evt/1", match_count: 2, zero_match: false });
    await updateEligibility("evt/1", { rule, allow_zero_match: true });
    mockSuccess([]);
    await listEligibilityImpactReviews();
    mockSuccess({ review_id: "rev/1", status: "resolved" });
    await resolveEligibilityImpactReview("rev/1", { reason: "reviewed" });
    mockSuccess({ event_id: "evt/1", eligible: true, reason: "" });
    await checkEligibility("evt/1");
    mockSuccess({ run_id: "lot_1", status: "completed" });
    await runLottery("evt/1", { seed: "seed-1" });
    mockSuccess({ ticket_id: "tkt/1" });
    await getTicket("tkt/1");
    mockSuccess({ export_id: "exp_1", status: "ready" });
    await createReportExport({ report_type: "participation" });
    mockSuccess({ export_id: "exp/1", status: "ready" });
    await getReportExport("exp/1");
    mockSuccess([]);
    await auditLogs({
      action: "event.updated",
      limit: "25",
      cursor: "2026-05-06T10:00:00Z",
    });

    expect(fetchCall(0)).toMatchObject({
      path: "/api/v1/admin/events/evt%2F1/eligibility/preview",
      body: { rule },
    });
    expect(fetchCall(0).init).toMatchObject({ method: "POST" });
    expect(fetchCall(1)).toMatchObject({
      path: "/api/v1/admin/events/evt%2F1/eligibility",
      body: { rule, allow_zero_match: true },
    });
    expect(fetchCall(1).init).toMatchObject({ method: "PUT" });
    expect(fetchCall(2).path).toBe("/api/v1/admin/eligibility-impact-reviews");
    expect(fetchCall(3)).toMatchObject({
      path: "/api/v1/admin/eligibility-impact-reviews/rev%2F1/resolve",
      body: { reason: "reviewed" },
    });
    expect(fetchCall(4).path).toBe("/api/v1/events/evt%2F1/eligibility");
    expect(fetchCall(5)).toMatchObject({
      path: "/api/v1/admin/events/evt%2F1/lottery-runs",
      body: { seed: "seed-1" },
    });
    expect(fetchCall(6).path).toBe("/api/v1/tickets/tkt%2F1");
    expect(fetchCall(7)).toMatchObject({
      path: "/api/v1/admin/reports/exports",
      body: { report_type: "participation" },
    });
    expect(fetchCall(8).path).toBe("/api/v1/admin/reports/exports/exp%2F1");
    expect(fetchCall(9).path).toContain("/api/v1/admin/audit-logs?");
    expect(fetchCall(9).path).toContain("cursor=2026-05-06T10%3A00%3A00Z");
  });

  it("calls production offline check-in and notification endpoints", async () => {
    mockSuccess({ batch_id: "off_1", tickets: [] });
    await offlineCheckinPackage("evt/1", " gate 1 ");
    mockSuccess({
      batch_id: "off_1",
      accepted: 1,
      duplicate: 0,
      conflict: 0,
      results: [],
    });
    await syncOfflineCheckins({
      batch_id: "off_1",
      event_id: "evt/1",
      device_id: "gate-1",
      scans: [
        { signed_token: "ticket-secret", scanned_at: "2026-05-06T10:00:00Z" },
      ],
    });
    mockSuccess({
      employee_id: "E1001",
      email_enabled: true,
      in_app_enabled: true,
      opted_out_categories: [],
    });
    await getNotificationPreferences();
    mockSuccess({
      employee_id: "E1001",
      email_enabled: false,
      in_app_enabled: true,
      opted_out_categories: ["booking"],
    });
    await updateNotificationPreferences({
      email_enabled: false,
      in_app_enabled: true,
      opted_out_categories: ["booking"],
    });
    mockSuccess([]);
    await listNotificationDeliveries();
    mockSuccess({ delivery_id: "del/1", status: "pending" });
    await retryNotificationDelivery("del/1");

    expect(fetchCall(0).path).toBe(
      "/api/v1/checkins/events/evt%2F1/offline-package?device_id=gate+1",
    );
    expect(fetchCall(1)).toMatchObject({
      path: "/api/v1/checkins/offline-sync",
      body: {
        batch_id: "off_1",
        event_id: "evt/1",
        device_id: "gate-1",
        scans: [
          { signed_token: "ticket-secret", scanned_at: "2026-05-06T10:00:00Z" },
        ],
      },
    });
    expect(fetchCall(1).init).toMatchObject({ method: "POST" });
    expect(fetchCall(2).path).toBe("/api/v1/notifications/preferences");
    expect(fetchCall(3)).toMatchObject({
      path: "/api/v1/notifications/preferences",
      body: {
        email_enabled: false,
        in_app_enabled: true,
        opted_out_categories: ["booking"],
      },
    });
    expect(fetchCall(3).init).toMatchObject({ method: "PUT" });
    expect(fetchCall(4).path).toBe("/api/v1/admin/notifications/deliveries");
    expect(fetchCall(5)).toMatchObject({
      path: "/api/v1/admin/notifications/deliveries/del%2F1/retry",
      body: {},
    });
  });
});
