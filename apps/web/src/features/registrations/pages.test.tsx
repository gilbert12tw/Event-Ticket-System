import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  cancelRegistration,
  listAdminEvents,
  listRegistrations,
  promoteWaitlist,
  revokeTicket,
  runLottery,
} from "@/lib/api";
import type { EventSummary, RegistrationDetail, Ticket } from "@/lib/api";
import { AdminRegistrationsPage } from "./pages";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    cancelRegistration: vi.fn(),
    listAdminEvents: vi.fn(),
    listRegistrations: vi.fn(),
    promoteWaitlist: vi.fn(),
    revokeTicket: vi.fn(),
    runLottery: vi.fn(),
  };
});

const mockCancelRegistration = vi.mocked(cancelRegistration);
const mockListAdminEvents = vi.mocked(listAdminEvents);
const mockListRegistrations = vi.mocked(listRegistrations);
const mockPromoteWaitlist = vi.mocked(promoteWaitlist);
const mockRevokeTicket = vi.mocked(revokeTicket);
const mockRunLottery = vi.mocked(runLottery);

const event: EventSummary = {
  event_id: "evt-1",
  title: "Annual Summit",
  description: "Company gathering",
  location: "Taipei HQ",
  starts_at: "2026-06-01T10:00:00Z",
  ends_at: "2026-06-01T12:00:00Z",
  registration_start: "2026-05-01T00:00:00Z",
  registration_close: "2026-05-25T00:00:00Z",
  capacity_type: "limited",
  capacity: 2,
  allows_family: false,
  status: "published",
  allocation_mode: "manual",
  created_by: "admin-1",
  created_at: "2026-04-01T00:00:00Z",
  updated_at: "2026-04-02T00:00:00Z",
  rule: {
    department: "*",
    site: "*",
    min_grade: 0,
    employment_status: "active",
  },
  confirmed_count: 1,
  waitlist_count: 1,
  remaining_capacity: 0,
  current_user_status: "",
};

const activeTicket: Ticket = {
  ticket_id: "tkt-1",
  registration_id: "reg-confirmed",
  event_id: "evt-1",
  employee_id: "E1001",
  status: "active",
  sequence_number: 1,
  issued_at: "2026-05-20T10:00:00Z",
  non_transferable: true,
};

const revokedTicket: Ticket = {
  ...activeTicket,
  ticket_id: "tkt-revoked",
  registration_id: "reg-cancelled",
  employee_id: "E1003",
  status: "revoked",
  revoked_reason: "security review",
};

const rows: RegistrationDetail[] = [
  {
    registration_id: "reg-confirmed",
    event_id: "evt-1",
    employee_id: "E1001",
    employee_name: "Ariel Chen",
    status: "confirmed",
    idempotency_key: "book-1",
    created_at: "2026-05-20T10:00:00Z",
    ticket: activeTicket,
  },
  {
    registration_id: "reg-waitlist",
    event_id: "evt-1",
    employee_id: "E1002",
    employee_name: "Ben Lin",
    status: "waitlisted",
    idempotency_key: "book-2",
    created_at: "2026-05-20T10:05:00Z",
  },
  {
    registration_id: "reg-cancelled",
    event_id: "evt-1",
    employee_id: "E1003",
    employee_name: "Casey Wu",
    status: "cancelled",
    idempotency_key: "book-3",
    cancel_reason: "manager request",
    created_at: "2026-05-20T10:10:00Z",
    ticket: revokedTicket,
  },
];

describe("AdminRegistrationsPage", () => {
  beforeEach(() => {
    globalThis.history.pushState({}, "", "/admin/registrations");
    vi.clearAllMocks();
    mockListAdminEvents.mockResolvedValue([event]);
    mockListRegistrations.mockResolvedValue(rows);
    mockCancelRegistration.mockResolvedValue({
      registration: rows[0],
      remaining_capacity: 1,
      message: "registration cancelled",
    });
    mockPromoteWaitlist.mockResolvedValue({
      remaining_capacity: 0,
      message: "waitlist promoted",
    });
    mockRevokeTicket.mockResolvedValue({
      ...activeTicket,
      status: "revoked",
      revoked_reason: "security review",
    });
    mockRunLottery.mockResolvedValue({
      run_id: "lot-1",
      event_id: "evt-1",
      seed: "seed-1",
      status: "completed",
      input_snapshot_at: "2026-05-21T10:00:00Z",
      algorithm_version: "deterministic-sha256-v1",
      candidate_count: 1,
      eligibility_rule_id: "rule-1",
      eligibility_rule_version: 1,
      eligibility_snapshot: {
        department: "*",
        site: "*",
        min_grade: 0,
        employment_status: "active",
      },
      winner_count: 1,
      created_by: "admin-1",
      created_at: "2026-05-21T10:00:00Z",
    });
  });

  it("renders governance tabs and confirms an admin cancellation", async () => {
    const user = userEvent.setup();

    render(<AdminRegistrationsPage />);

    expect(
      await screen.findByRole("heading", { name: "Annual Summit" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("報名治理摘要")).toHaveTextContent("需處理3");
    expect(screen.getByText("reg-confirmed")).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: "候補名單" }));
    expect(screen.getByText("reg-waitlist")).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "票券狀態" }));
    expect(screen.getByText("tkt-1")).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "取消/撤銷紀錄" }));
    expect(screen.getByText("只讀紀錄")).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: "報名名單" }));
    await user.click(screen.getByRole("button", { name: "取消" }));
    expect(
      screen.getByRole("heading", { name: "確認取消報名" }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "返回" }));
    expect(
      screen.queryByRole("heading", { name: "確認取消報名" }),
    ).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "取消" }));
    await user.click(screen.getByRole("combobox", { name: "處置原因" }));
    await user.click(screen.getByRole("option", { name: "主管要求" }));
    await user.click(screen.getByRole("button", { name: "確認取消報名" }));

    await waitFor(() =>
      expect(mockCancelRegistration).toHaveBeenCalledWith(
        "evt-1",
        "reg-confirmed",
        "manager request",
        "cancel-reg-confirmed",
      ),
    );
  });

  it("runs waitlist promotion and ticket revocation actions", async () => {
    const user = userEvent.setup();

    render(<AdminRegistrationsPage />);
    await screen.findByRole("heading", { name: "Annual Summit" });

    await user.click(screen.getByRole("tab", { name: "候補名單" }));
    await user.click(screen.getByRole("button", { name: "提升候補" }));
    await waitFor(() =>
      expect(mockPromoteWaitlist).toHaveBeenCalledWith("evt-1"),
    );

    await user.click(screen.getByRole("tab", { name: "票券狀態" }));
    const revokeButton = screen
      .getAllByRole("button", { name: "撤銷票券" })
      .find((button) => !button.hasAttribute("disabled"));
    expect(revokeButton).toBeDefined();
    await user.click(revokeButton as HTMLButtonElement);
    expect(
      screen.getByRole("heading", { name: "確認撤銷票券" }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("combobox", { name: "處置原因" }));
    await user.click(screen.getByRole("option", { name: "安全審核" }));
    await user.click(screen.getByRole("button", { name: "確認撤銷" }));

    await waitFor(() =>
      expect(mockRevokeTicket).toHaveBeenCalledWith("tkt-1", "security review"),
    );
  });

  it("runs deterministic lottery allocation and refreshes rows", async () => {
    const user = userEvent.setup();
    mockListAdminEvents.mockResolvedValue([
      { ...event, allocation_mode: "lottery" },
    ]);

    render(<AdminRegistrationsPage />);

    expect(
      await screen.findByRole("heading", { name: "Annual Summit" }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "配票抽籤" }));
    expect(
      screen.getByRole("heading", { name: "配票抽籤" }),
    ).toBeInTheDocument();

    await user.type(await screen.findByLabelText(/抽籤 seed/), "seed-1");
    await user.click(screen.getByRole("button", { name: "執行抽籤" }));
    expect(
      screen.getByRole("heading", {
        name: "確認執行 deterministic lottery",
      }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "確認抽籤" }));

    await waitFor(() =>
      expect(mockRunLottery).toHaveBeenCalledWith("evt-1", { seed: "seed-1" }),
    );
    expect(await screen.findByText("lot-1")).toBeInTheDocument();
    expect(mockListRegistrations.mock.calls.length).toBeGreaterThanOrEqual(2);
  });
});
