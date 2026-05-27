import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  auditLogs,
  bookEvent,
  cancelRegistration,
  checkIn,
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
  revokeTicket,
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
      jsonResponse({ success: true, data, error: null }),
    );
  }

  async function mockCall<T>(data: unknown, call: () => Promise<T>) {
    mockSuccess(data);
    return call();
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

  type ExpectedRequest = [path: string, body?: unknown, method?: string];

  function expectRequest(index: number, [path, body, method]: ExpectedRequest) {
    expect(fetchCall(index)).toMatchObject({
      path,
      ...(body === undefined ? {} : { body }),
      ...(method ? { init: { method } } : {}),
    });
  }

  function expectRequests(requests: ExpectedRequest[], start = 0) {
    requests.forEach((request, offset) =>
      expectRequest(start + offset, request),
    );
  }

  function requestLog<T>(index = 0) {
    return entries[index]?.requestBody as T;
  }

  function responseData<T>(index = 0) {
    return (entries[index]?.responseBody as { data: T }).data;
  }

  function eligibleEvent() {
    return {
      event_id: "evt/1",
      eligible: true,
      can_book: true,
      reasons: [],
      warnings: [],
      no_show_cooldown: { active: false },
    };
  }

  function offlineSyncRequest(packageSignature = "package-signature") {
    return {
      batch_id: "off_1",
      event_id: "evt/1",
      device_id: "gate-1",
      package_signature: packageSignature,
      scans: [
        { signed_token: "ticket-secret", scanned_at: "2026-05-06T10:00:00Z" },
      ],
    };
  }

  function notificationPrefs(email_enabled: boolean) {
    return {
      employee_id: "E1001",
      email_enabled,
      in_app_enabled: true,
      opted_out_categories: email_enabled ? [] : ["booking"],
    };
  }

  it("returns envelope data and redacts ticket tokens in API logs", async () => {
    const tickets = await mockCall(
      [
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
      listTickets,
    );

    expect(tickets).toHaveLength(1);
    expect(fetchCall(0)).toMatchObject({
      path: "/api/v1/me/tickets",
      init: {
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
      },
    });

    expect(entries[0]?.requestBody).toBeNull();
    const ticketLog = responseData<Array<Record<string, string>>>()[0];
    expect(ticketLog.signed_token).toBe("[票券簽章已遮蔽]");
    expect(ticketLog.qr_payload).toBe("[票券簽章已遮蔽]");
  });

  it("attaches provider bearer tokens from memory", async () => {
    setProviderToken("provider-secret");
    await mockCall([], listEvents);

    expect(fetchCall(0).init).toMatchObject({
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer provider-secret",
      },
    });
  });

  it("maps provider claims to an authenticated session", async () => {
    const session = await mockCall(
      {
        employee_id: "E1001",
        display_name: "Ariel Chen",
        role_claims: ["employee"],
        mapped_roles: ["employee"],
        department: "Engineering",
        site: "Taipei HQ",
        city: "Taipei",
        grade: 6,
        employment_status: "active",
        claims_status: "complete",
      },
      me,
    );

    expect(session).toMatchObject({
      actor: { id: "E1001", role: "employee" },
      source: "provider",
      claims: { claims_status: "complete" },
    });
  });

  it("treats null list envelope data as an empty array", async () => {
    const events = await mockCall(null, listEvents);

    expect(events).toEqual([]);
    expect(fetchCall(0).path).toBe("/api/v1/events");
  });

  it("uses canonical own-data endpoints without caller-supplied employee_id", async () => {
    await mockCall([], listEvents);
    await mockCall({ event_id: "evt/1" }, () => getEvent("evt/1"));
    await mockCall(eligibleEvent(), () => checkEligibility("evt/1"));
    await mockCall({ event_id: "evt/1" }, () => bookEvent("evt/1", "book-1"));
    await mockCall([], listTickets);

    expectRequests([
      ["/api/v1/events"],
      ["/api/v1/events/evt%2F1"],
      ["/api/v1/events/evt%2F1/eligibility"],
      [
        "/api/v1/events/evt%2F1/bookings",
        { idempotency_key: "book-1", family_count: 0 },
      ],
      ["/api/v1/me/tickets"],
    ]);
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

    const caught = await mockProviderToken("E1001").catch(
      (error: unknown) => error,
    );

    expect(caught).toBeInstanceOf(ApiError);
    expect(caught).toMatchObject({
      status: 401,
      message: "unauthorized",
    });
  });

  it("redacts provider token fields in API log response bodies", async () => {
    await mockCall(
      {
        provider_token: "provider-token-secret",
        expires_at: "2026-05-06T18:00:00Z",
        nested: {
          token: "nested-token-secret",
        },
      },
      () => mockProviderToken("E1001"),
    );

    const body = responseData<{
      provider_token: string;
      nested: { token: string };
    }>();
    expect(body.provider_token).toBe("[身分簽章已遮蔽]");
    expect(body.nested.token).toBe("[工作階段已遮蔽]");
  });

  it("logs redacted request and response bodies separately", async () => {
    await mockCall(
      {
        batch_id: "off_1",
        accepted: 1,
        duplicate: 0,
        conflict: 0,
        results: [],
        provider_token: "provider-token-secret",
        signed_token: "ticket-secret",
        qr_payload: "qr-secret",
        qr_token: "qr-token-secret",
        token_hash: "hash-secret",
        signed_token_hash: "signed-hash-secret",
        package_signature: "package-signature-secret",
        nested: {
          token: "nested-token-secret",
        },
      },
      () => syncOfflineCheckins(offlineSyncRequest("package-signature-secret")),
    );

    const requestBody = requestLog<{
      scans: Array<{ signed_token: string }>;
    }>();
    const responseBody = responseData<
      Record<string, string> & { nested: { token: string } }
    >();
    expect(requestBody.scans[0].signed_token).toBe("[票券簽章已遮蔽]");
    expect(responseBody).toMatchObject({
      provider_token: "[身分簽章已遮蔽]",
      signed_token: "[票券簽章已遮蔽]",
      qr_payload: "[票券簽章已遮蔽]",
      qr_token: "[票券簽章已遮蔽]",
    });
    for (const key of [
      "token_hash",
      "signed_token_hash",
      "package_signature",
    ]) {
      expect(responseBody[key]).toMatch(/^\[票券證據已遮蔽 #[0-9a-f]{8}\]$/);
    }
    expect(responseBody.nested.token).toBe("[工作階段已遮蔽]");
  });

  it("sends event context for online check-in", async () => {
    await mockCall({ ticket_id: "tkt_1", status: "accepted" }, () =>
      checkIn("signed-token", "gate-1", "evt_1", "photo mismatch"),
    );

    expectRequest(0, [
      "/api/v1/checkins",
      {
        signed_token: "signed-token",
        device_id: "gate-1",
        event_id: "evt_1",
        holder_mismatch_reason: "photo mismatch",
      },
    ]);
  });

  it("calls production eligibility, lottery, ticket, report, and audit endpoints", async () => {
    const rule = {
      department: "Engineering",
      site: "Taipei",
      min_grade: 5,
      employment_status: "active",
    };
    const impactResult = {
      event_id: "evt/1",
      match_count: 2,
      zero_match: false,
    };

    await mockCall(impactResult, () => previewEligibility("evt/1", { rule }));
    await mockCall(impactResult, () =>
      updateEligibility("evt/1", { rule, allow_zero_match: true }),
    );
    await mockCall([], listEligibilityImpactReviews);
    await mockCall({ review_id: "rev/1", status: "resolved" }, () =>
      resolveEligibilityImpactReview("rev/1", { reason: "reviewed" }),
    );
    const eligibility = await mockCall(eligibleEvent(), () =>
      checkEligibility("evt/1"),
    );
    await mockCall({ run_id: "lot_1", status: "completed" }, () =>
      runLottery("evt/1", { seed: "seed-1" }),
    );
    await mockCall({ ticket_id: "tkt/1" }, () => getTicket("tkt/1"));
    await mockCall({ message: "registration cancelled" }, () =>
      cancelRegistration("evt/1", "reg/1", "manager request", "cancel-1"),
    );
    await mockCall({ ticket_id: "tkt/1", status: "revoked" }, () =>
      revokeTicket("tkt/1", "security review"),
    );
    await mockCall({ export_id: "exp_1", status: "ready" }, () =>
      createReportExport({ report_type: "participation" }),
    );
    await mockCall({ export_id: "exp/1", status: "ready" }, () =>
      getReportExport("exp/1"),
    );
    await mockCall([], () =>
      auditLogs({
        action: "event.updated",
        limit: "25",
        cursor: "2026-05-06T10:00:00Z",
      }),
    );

    expectRequests([
      ["/api/v1/admin/events/evt%2F1/eligibility/preview", { rule }, "POST"],
      [
        "/api/v1/admin/events/evt%2F1/eligibility",
        { rule, allow_zero_match: true },
        "PUT",
      ],
      ["/api/v1/admin/eligibility-impact-reviews"],
      [
        "/api/v1/admin/eligibility-impact-reviews/rev%2F1/resolve",
        { reason: "reviewed" },
      ],
      ["/api/v1/events/evt%2F1/eligibility"],
    ]);
    expect(eligibility).toMatchObject({
      event_id: "evt/1",
      can_book: true,
      warnings: [],
    });
    expectRequests(
      [
        ["/api/v1/admin/events/evt%2F1/lottery-runs", { seed: "seed-1" }],
        ["/api/v1/tickets/tkt%2F1"],
        [
          "/api/v1/admin/events/evt%2F1/registrations/reg%2F1/cancel",
          { reason: "manager request", idempotency_key: "cancel-1" },
        ],
        ["/api/v1/admin/tickets/tkt%2F1/revoke", { reason: "security review" }],
        ["/api/v1/admin/reports/exports", { report_type: "participation" }],
        ["/api/v1/admin/reports/exports/exp%2F1"],
      ],
      5,
    );
    expect(fetchCall(11).path).toContain("/api/v1/admin/audit-logs?");
    expect(fetchCall(11).path).toContain("cursor=2026-05-06T10%3A00%3A00Z");
  });

  it("calls production offline check-in and notification endpoints", async () => {
    await mockCall({ batch_id: "off_1", tickets: [] }, () =>
      offlineCheckinPackage("evt/1", " gate 1 "),
    );
    await mockCall(
      {
        batch_id: "off_1",
        accepted: 1,
        duplicate: 0,
        conflict: 0,
        results: [],
      },
      () => syncOfflineCheckins(offlineSyncRequest()),
    );
    await mockCall(notificationPrefs(true), getNotificationPreferences);
    await mockCall(notificationPrefs(false), () =>
      updateNotificationPreferences({
        email_enabled: false,
        in_app_enabled: true,
        opted_out_categories: ["booking"],
      }),
    );
    await mockCall([], listNotificationDeliveries);
    await mockCall({ delivery_id: "del/1", status: "pending" }, () =>
      retryNotificationDelivery("del/1"),
    );

    expectRequests([
      ["/api/v1/checkins/events/evt%2F1/offline-package?device_id=gate+1"],
      ["/api/v1/checkins/offline-sync", offlineSyncRequest(), "POST"],
      ["/api/v1/notifications/preferences"],
      [
        "/api/v1/notifications/preferences",
        {
          email_enabled: false,
          in_app_enabled: true,
          opted_out_categories: ["booking"],
        },
        "PUT",
      ],
      ["/api/v1/admin/notifications/deliveries"],
      ["/api/v1/admin/notifications/deliveries/del%2F1/retry", {}],
    ]);
  });
});
