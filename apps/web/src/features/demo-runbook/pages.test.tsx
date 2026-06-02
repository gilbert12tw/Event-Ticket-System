import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  auditLogs,
  bookEvent,
  captureProviderToken,
  checkIn,
  createEvent,
  getDemoClock,
  listRegistrations,
  listTickets,
  reports,
  restoreProviderToken,
  runLottery,
  selectMockProfile,
  updateDemoClock,
} from "@/lib/api";
import { DemoRunbookPage } from "./pages";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    auditLogs: vi.fn().mockResolvedValue([]),
    bookEvent: vi.fn(),
    captureProviderToken: vi
      .fn()
      .mockReturnValue({ explicitProviderToken: "previous-token" }),
    checkIn: vi.fn(),
    createEvent: vi.fn(),
    getDemoClock: vi.fn(),
    listRegistrations: vi.fn().mockResolvedValue([]),
    listTickets: vi.fn().mockResolvedValue([]),
    reports: vi.fn().mockResolvedValue([]),
    restoreProviderToken: vi.fn(),
    runLottery: vi.fn(),
    seedDemo: vi.fn().mockResolvedValue({ status: "seeded" }),
    selectMockProfile: vi.fn(),
    updateDemoClock: vi.fn(),
  };
});

const adminSession = {
  actor: { id: "admin-1", role: "activity_admin" as const },
  expires_at: "2026-05-31T08:00:00Z",
  claims: {
    employee_id: "admin-1",
    display_name: "Admin One",
    role_claims: ["activity_admin"],
    mapped_roles: ["activity_admin" as const],
    department: "Welfare Committee",
    site: "Taipei HQ",
    city: "Taipei",
    grade: 7,
    employment_status: "active",
    claims_status: "complete" as const,
  },
  source: "provider" as const,
};

const externalAdminSession = {
  ...adminSession,
  actor: { id: "external-admin", role: "activity_admin" as const },
  claims: {
    ...adminSession.claims,
    employee_id: "external-admin",
    display_name: "External Admin",
  },
};

const demoLotteryEvent = {
  event_id: "evt_demo",
  title: "Demo 抽籤活動",
  description: "Demo",
  location: "台北總部禮堂",
  event_city: "Taipei",
  event_site: "Taipei HQ",
  starts_at: "2026-05-31T12:00:00Z",
  registration_start: "2026-05-31T07:00:00Z",
  registration_close: "2026-05-31T09:00:00Z",
  capacity_type: "limited" as const,
  capacity: 1,
  allows_family: false,
  allocation_mode: "lottery" as const,
  status: "published" as const,
  created_by: "admin-1",
  created_at: "2026-05-31T08:00:00Z",
  updated_at: "2026-05-31T08:00:00Z",
  rule: {
    department: "Engineering",
    site: "Taipei HQ",
    min_grade: 5,
    employment_status: "active",
  },
  confirmed_count: 0,
  waitlist_count: 0,
  remaining_capacity: 1,
  current_user_status: "",
};

const firstReceived = {
  registration_id: "reg-1",
  event_id: "evt_demo",
  employee_id: "E1001",
  employee_name: "Ariel Chen",
  status: "received",
  idempotency_key: "demo-E1001",
  family_count: 0,
  created_at: "2026-05-31T08:05:00Z",
};

const secondReceived = {
  registration_id: "reg-2",
  event_id: "evt_demo",
  employee_id: "E1002",
  employee_name: "Ben Lin",
  status: "received",
  idempotency_key: "demo-E1002",
  family_count: 0,
  created_at: "2026-05-31T08:06:00Z",
};

const confirmedWinner = {
  ...firstReceived,
  status: "confirmed",
  ticket: {
    ticket_id: "ticket-1",
    registration_id: "reg-1",
    event_id: "evt_demo",
    employee_id: "E1001",
    status: "active",
    signed_token: "signed-ticket-token",
    issued_at: "2026-05-31T09:02:00Z",
    non_transferable: true as const,
  },
};

const waitlistedCandidate = {
  ...secondReceived,
  status: "waitlisted",
};

function setupClock() {
  vi.mocked(getDemoClock).mockResolvedValue({
    enabled: true,
    mode: "real",
    now: "2026-05-31T08:00:00Z",
    real_now: "2026-05-31T08:00:00Z",
  });
}

function stepRow(label: string) {
  const row = screen
    .getByText(
      (content, element) => element?.tagName === "STRONG" && content === label,
    )
    .closest(".step-item");
  expect(row).not.toBeNull();
  return row as HTMLElement;
}

async function runStep(label: string, expectedHint: string | RegExp) {
  const row = stepRow(label);
  await userEvent.click(within(row).getByRole("button", { name: /執行/ }));
  await within(row).findByText(expectedHint);
}

describe("DemoRunbookPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders manual controls and creates a real lottery event", async () => {
    setupClock();
    vi.mocked(selectMockProfile).mockResolvedValue(adminSession);
    vi.mocked(createEvent).mockResolvedValue(demoLotteryEvent);

    render(
      <DemoRunbookPage
        session={adminSession}
        demoDebugAvailable
        onSessionChange={vi.fn()}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Demo 控制台" }),
    ).toBeInTheDocument();
    expect(screen.queryByText(/一鍵/)).not.toBeInTheDocument();

    const buttons = await screen.findAllByRole("button", { name: /執行/ });
    await userEvent.click(buttons[1]);

    await waitFor(() =>
      expect(createEvent).toHaveBeenCalledWith(
        expect.objectContaining({ allocation_mode: "lottery" }),
      ),
    );
  });

  it("restores the previous provider token snapshot after a demo action", async () => {
    setupClock();
    const tokenSnapshot = { explicitProviderToken: "provider-token" };
    vi.mocked(captureProviderToken).mockReturnValue(tokenSnapshot);
    vi.mocked(selectMockProfile).mockResolvedValue(adminSession);
    vi.mocked(createEvent).mockResolvedValue(demoLotteryEvent);
    const onSessionChange = vi.fn();

    render(
      <DemoRunbookPage
        session={externalAdminSession}
        demoDebugAvailable
        onSessionChange={onSessionChange}
      />,
    );

    const buttons = await screen.findAllByRole("button", { name: /執行/ });
    await userEvent.click(buttons[1]);

    await waitFor(() =>
      expect(createEvent).toHaveBeenCalledWith(
        expect.objectContaining({ allocation_mode: "lottery" }),
      ),
    );
    await waitFor(() =>
      expect(onSessionChange).toHaveBeenCalledWith(externalAdminSession),
    );
    expect(onSessionChange).not.toHaveBeenCalledWith(null);
    expect(restoreProviderToken).toHaveBeenCalledWith(tokenSnapshot);
    expect(selectMockProfile).toHaveBeenCalledWith("admin-1");
  });

  it("does not mint a mock token when restoring a provider actor that matches a mock profile", async () => {
    setupClock();
    const tokenSnapshot = { explicitProviderToken: null };
    vi.mocked(captureProviderToken).mockReturnValue(tokenSnapshot);
    vi.mocked(selectMockProfile).mockResolvedValue(adminSession);
    vi.mocked(createEvent).mockResolvedValue(demoLotteryEvent);

    render(
      <DemoRunbookPage
        session={adminSession}
        demoDebugAvailable
        onSessionChange={vi.fn()}
      />,
    );

    const buttons = await screen.findAllByRole("button", { name: /執行/ });
    await userEvent.click(buttons[1]);

    await waitFor(() =>
      expect(restoreProviderToken).toHaveBeenCalledWith(tokenSnapshot),
    );
    expect(selectMockProfile).toHaveBeenCalledTimes(1);
  });

  it("restores the provider token snapshot when mock profile selection fails", async () => {
    setupClock();
    const tokenSnapshot = { explicitProviderToken: "provider-token" };
    vi.mocked(captureProviderToken).mockReturnValue(tokenSnapshot);
    vi.mocked(selectMockProfile).mockRejectedValue(
      new Error("mock auth failed"),
    );
    const onSessionChange = vi.fn();

    render(
      <DemoRunbookPage
        session={externalAdminSession}
        demoDebugAvailable
        onSessionChange={onSessionChange}
      />,
    );

    const buttons = await screen.findAllByRole("button", { name: /執行/ });
    await userEvent.click(buttons[1]);

    await waitFor(() =>
      expect(restoreProviderToken).toHaveBeenCalledWith(tokenSnapshot),
    );
    expect(onSessionChange).toHaveBeenCalledWith(externalAdminSession);
    expect(createEvent).not.toHaveBeenCalled();
    expect(await screen.findByText("mock auth failed")).toBeInTheDocument();
  });

  it("runs the demo flow through received lottery candidates and audit evidence", async () => {
    setupClock();
    vi.mocked(selectMockProfile).mockResolvedValue(adminSession);
    vi.mocked(createEvent).mockResolvedValue(demoLotteryEvent);
    vi.mocked(bookEvent).mockImplementation(async (_eventID, key) => ({
      registration: key.endsWith("E1001") ? firstReceived : secondReceived,
      remaining_capacity: 1,
      message: "received",
    }));
    vi.mocked(listRegistrations)
      .mockResolvedValueOnce([firstReceived])
      .mockResolvedValueOnce([firstReceived, secondReceived])
      .mockResolvedValueOnce([firstReceived, secondReceived])
      .mockResolvedValueOnce([confirmedWinner, waitlistedCandidate]);
    vi.mocked(updateDemoClock).mockResolvedValue({
      enabled: true,
      mode: "fixed",
      now: "2026-05-31T09:01:00Z",
      real_now: "2026-05-31T08:10:00Z",
      reason: "demo cutoff for evt_demo",
    });
    vi.mocked(runLottery).mockResolvedValue({
      run_id: "lottery-1",
      event_id: "evt_demo",
      seed: "demo-seed",
      status: "completed",
      input_snapshot_at: "2026-05-31T09:01:00Z",
      algorithm_version: "lottery-v1",
      candidate_count: 2,
      eligibility_rule_id: "rule-1",
      eligibility_rule_version: 1,
      eligibility_snapshot: {
        department: "Engineering",
        site: "Taipei HQ",
        min_grade: 5,
        employment_status: "active",
      },
      winner_count: 1,
      created_by: "admin-1",
      created_at: "2026-05-31T09:02:00Z",
    });
    vi.mocked(listTickets).mockResolvedValue([
      {
        ticket_id: "ticket-1",
        registration_id: "reg-1",
        event_id: "evt_demo",
        employee_id: "E1001",
        status: "active",
        signed_token: "signed-ticket-token",
        issued_at: "2026-05-31T09:02:00Z",
        event_title: "Demo 抽籤活動",
        non_transferable: true,
      },
    ]);
    vi.mocked(checkIn)
      .mockResolvedValueOnce({
        checkin_id: "checkin-1",
        ticket_id: "ticket-1",
        event_id: "evt_demo",
        employee_id: "E1001",
        status: "accepted",
        scanned_at: "2026-05-31T09:03:00Z",
        duplicate: false,
        family_count: 0,
      })
      .mockRejectedValueOnce(new Error("duplicate scan rejected"));
    vi.mocked(reports).mockResolvedValue([
      {
        event_id: "evt_demo",
        title: "Demo 抽籤活動",
        capacity_type: "limited",
        capacity: 1,
        confirmed_count: 1,
        waitlist_count: 1,
        employee_count: 1,
        family_count: 0,
        total_attendee_count: 1,
        ticket_count: 1,
        checkin_count: 1,
        remaining_capacity: 0,
        city_distribution: { Taipei: 1 },
        starts_at: "2026-05-31T12:00:00Z",
      },
    ]);
    vi.mocked(auditLogs).mockResolvedValue([
      {
        audit_id: "audit-1",
        actor_id: "admin-1",
        role: "activity_admin",
        action: "lottery.completed",
        entity_type: "event",
        entity_id: "evt_demo",
        metadata: "{}",
        created_at: "2026-05-31T09:02:00Z",
      },
    ]);

    render(
      <DemoRunbookPage
        session={adminSession}
        demoDebugAvailable
        onSessionChange={vi.fn()}
      />,
    );

    await runStep("載入 demo 員工", /seeded E1001/);
    await runStep("建立 lottery 活動", /created evt_demo/);
    await runStep("E1001 送出報名", /E1001: status=received/);
    await runStep("E1002 送出報名", /E1002: status=received/);
    await runStep("檢查 received 名單", "received=2; total=2");
    await runStep("快進到 cutoff 後", /fixed now=2026-05-31T09:01:00Z/);
    await runStep("執行 deterministic lottery", "candidates=2; winners=1");
    await runStep("查看 winner 票券", "E1001: ticket=ticket-1");
    await runStep("現場首次驗票", /accepted at/);
    await runStep("重複掃描拒絕", "duplicate scan rejected");
    await runStep("查報表與稽核", "reports=1; event_confirmed=1; audits=1");

    expect(runLottery).toHaveBeenCalledWith("evt_demo", {
      seed: expect.stringMatching(/^demo-/),
    });
    expect(checkIn).toHaveBeenCalledWith(
      "signed-ticket-token",
      "demo-gate-1",
      "evt_demo",
    );
    expect(screen.getByText("lottery.completed")).toBeInTheDocument();
    expect(screen.getByText("Algorithm")).toBeInTheDocument();
    expect(screen.getByText("lottery-v1")).toBeInTheDocument();
  });

  it("shows a guarded failure when debug clock controls are unavailable", async () => {
    render(
      <DemoRunbookPage
        session={adminSession}
        demoDebugAvailable={false}
        onSessionChange={vi.fn()}
      />,
    );

    expect(getDemoClock).not.toHaveBeenCalled();
    expect(screen.getByText("Real time")).toBeInTheDocument();
    expect(screen.getByText(/尚未啟用 DEMO_DEBUG_ENABLED/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "重新讀取" })).toBeDisabled();

    await runStep("快進到 cutoff 後", "DEMO_DEBUG_ENABLED 尚未開啟");
  });
});
