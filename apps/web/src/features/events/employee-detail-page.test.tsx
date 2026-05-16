import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  bookEvent,
  cancelMyRegistration,
  getEvent,
  listEvents,
} from "@/lib/api";
import { EmployeeEventDetailPage } from "./employee-detail-page";
import { claims, eventFixture } from "./employee-pages-test-helpers";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    bookEvent: vi.fn(),
    cancelMyRegistration: vi.fn(),
    getEvent: vi.fn(),
    listEvents: vi.fn(),
  };
});

const mockListEvents = vi.mocked(listEvents);
const mockGetEvent = vi.mocked(getEvent);
const mockBookEvent = vi.mocked(bookEvent);
const mockCancelMyRegistration = vi.mocked(cancelMyRegistration);

describe("EmployeeEventDetailPage", () => {
  beforeEach(() => {
    mockListEvents.mockReset();
    mockGetEvent.mockReset();
    mockBookEvent.mockReset();
    mockCancelMyRegistration.mockReset();
    window.history.replaceState({}, "", "/user/events/detail?event_id=evt-1");
  });

  it("shows compact confirmation and waitlist policy on event detail", async () => {
    const event = eventFixture({
      allocation_mode: "fcfs",
      remaining_capacity: 0,
      waitlist_count: 3,
    });
    mockListEvents.mockResolvedValue([event]);
    mockGetEvent.mockResolvedValue(event);

    render(<EmployeeEventDetailPage claims={claims} />);

    expect(await screen.findByText("送出前確認")).toBeInTheDocument();
    expect(screen.getAllByText("候補政策").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/目前不顯示候補順位/).length).toBeGreaterThan(0);
    expect(screen.getByText("取消期限")).toBeInTheDocument();
  });

  it("keeps event detail actions without embedding the ticket QR", async () => {
    const event = eventFixture({
      current_user_ticket: {
        ticket_id: "T-2",
        registration_id: "R-2",
        event_id: "evt-1",
        employee_id: "E1001",
        status: "active",
        signed_token: "signed-secret",
        issued_at: "2026-05-06T10:00:00Z",
      },
      current_user_status: "confirmed",
    });
    mockListEvents.mockResolvedValue([event]);
    mockGetEvent.mockResolvedValue(event);

    render(<EmployeeEventDetailPage claims={claims} />);

    expect(await screen.findByText("主要操作")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "查看這張票券" }),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("票券二維碼")).not.toBeInTheDocument();
    expect(screen.queryByText("signed-secret")).not.toBeInTheDocument();
  });

  it("opens the exact ticket detail from the event detail handoff", async () => {
    const event = eventFixture({
      current_user_ticket: {
        ticket_id: "T-3",
        registration_id: "R-3",
        event_id: "evt-1",
        employee_id: "E1001",
        status: "active",
        signed_token: "signed-secret",
        issued_at: "2026-05-06T10:00:00Z",
      },
      current_user_status: "confirmed",
    });
    mockListEvents.mockResolvedValue([event]);
    mockGetEvent.mockResolvedValue(event);

    render(<EmployeeEventDetailPage claims={claims} />);

    await userEvent.click(
      await screen.findByRole("link", { name: "查看這張票券" }),
    );

    expect(window.location.pathname).toBe("/user/tickets");
    expect(window.location.search).toBe("?ticket_id=T-3");
  });

  it("opens the exact ticket detail after booking success", async () => {
    const event = eventFixture({
      current_user_status: "",
      remaining_capacity: 3,
    });
    mockListEvents.mockResolvedValue([event]);
    mockGetEvent.mockResolvedValue(event);
    mockBookEvent.mockResolvedValue({
      registration: {
        registration_id: "R-4",
        event_id: "evt-1",
        employee_id: "E1001",
        status: "confirmed",
        idempotency_key: "book-evt-1-E1001",
        created_at: "2026-05-16T10:00:00Z",
      },
      ticket: {
        ticket_id: "T-4",
        registration_id: "R-4",
        event_id: "evt-1",
        employee_id: "E1001",
        status: "active",
        signed_token: "signed-secret",
        issued_at: "2026-05-16T10:00:00Z",
      },
      remaining_capacity: 2,
      message: "confirmed",
    });

    render(<EmployeeEventDetailPage claims={claims} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "立即報名" }),
    );
    await userEvent.click(
      await screen.findByRole("link", { name: "查看票券" }),
    );

    expect(window.location.pathname).toBe("/user/tickets");
    expect(window.location.search).toBe("?ticket_id=T-4");
  });

  it("shows duplicate confirmed booking as existing state with exact ticket handoff", async () => {
    const event = eventFixture({
      current_user_status: "",
      remaining_capacity: 3,
    });
    mockListEvents.mockResolvedValue([event]);
    mockGetEvent.mockResolvedValue(event);
    mockBookEvent.mockResolvedValue({
      registration: {
        registration_id: "R-duplicate",
        event_id: "evt-1",
        employee_id: "E1001",
        status: "confirmed",
        idempotency_key: "book-evt-1-E1001",
        created_at: "2026-05-16T10:00:00Z",
      },
      ticket: {
        ticket_id: "T-duplicate",
        registration_id: "R-duplicate",
        event_id: "evt-1",
        employee_id: "E1001",
        status: "active",
        signed_token: "signed-secret",
        issued_at: "2026-05-16T10:00:00Z",
      },
      remaining_capacity: 2,
      message: "booking confirmed and ticket issued",
      duplicate: true,
    });

    render(<EmployeeEventDetailPage claims={claims} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "立即報名" }),
    );

    expect(await screen.findByText("你已經報名此活動")).toBeInTheDocument();
    expect(screen.getByText(/未建立新的報名/)).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "立即報名" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "查看這張票券" }),
    ).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("link", { name: "查看票券" }));

    expect(window.location.pathname).toBe("/user/tickets");
    expect(window.location.search).toBe("?ticket_id=T-duplicate");
  });

  it("shows duplicate waitlist booking without fresh success wording", async () => {
    const event = eventFixture({
      current_user_status: "",
      remaining_capacity: 0,
      waitlist_count: 1,
    });
    mockListEvents.mockResolvedValue([event]);
    mockGetEvent.mockResolvedValue(event);
    mockBookEvent.mockResolvedValue({
      registration: {
        registration_id: "R-wait-duplicate",
        event_id: "evt-1",
        employee_id: "E1001",
        status: "waitlisted",
        idempotency_key: "book-evt-1-E1001",
        created_at: "2026-05-16T10:00:00Z",
      },
      remaining_capacity: 0,
      message: "event is full; employee joined waitlist",
      duplicate: true,
    });

    render(<EmployeeEventDetailPage claims={claims} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "加入候補" }),
    );

    expect(await screen.findByText("你已在候補中")).toBeInTheDocument();
    expect(screen.getByText(/未建立新的候補/)).toBeInTheDocument();
    expect(screen.queryByText("報名成功，票券已核發")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "加入候補" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "候補中" })).toBeDisabled();
    expect(
      screen.getByRole("link", { name: "查看通知中心" }),
    ).toBeInTheDocument();
  });

  it("shows duplicate cancelled booking as blocked recovery guidance", async () => {
    const event = eventFixture({
      current_user_status: "",
      remaining_capacity: 3,
    });
    mockListEvents.mockResolvedValue([event]);
    mockGetEvent.mockResolvedValue(event);
    mockBookEvent.mockResolvedValue({
      registration: {
        registration_id: "R-cancelled-duplicate",
        event_id: "evt-1",
        employee_id: "E1001",
        status: "cancelled",
        idempotency_key: "book-evt-1-E1001",
        created_at: "2026-05-16T10:00:00Z",
      },
      ticket: {
        ticket_id: "T-revoked-duplicate",
        registration_id: "R-cancelled-duplicate",
        event_id: "evt-1",
        employee_id: "E1001",
        status: "revoked",
        issued_at: "2026-05-16T10:00:00Z",
        revoked_at: "2026-05-17T10:00:00Z",
        revoked_reason: "registration cancelled",
      },
      remaining_capacity: 3,
      message: "booking already cancelled",
      duplicate: true,
    });

    render(<EmployeeEventDetailPage claims={claims} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "立即報名" }),
    );

    expect(await screen.findByText("報名已取消")).toBeInTheDocument();
    expect(screen.getByText(/未建立新的報名/)).toBeInTheDocument();
    expect(screen.getAllByText(/請聯絡活動主辦/).length).toBeGreaterThan(0);
    expect(screen.queryByText("報名成功，票券已核發")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "查看票券" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "已取消" })).toBeDisabled();
  });

  it("does not show active ticket handoff for cancelled revoked tickets", async () => {
    const event = eventFixture({
      current_user_registration_id: "R-revoked",
      current_user_status: "cancelled",
      current_user_ticket: {
        ticket_id: "T-revoked",
        registration_id: "R-revoked",
        event_id: "evt-1",
        employee_id: "E1001",
        status: "revoked",
        issued_at: "2026-05-16T10:00:00Z",
        revoked_at: "2026-05-17T10:00:00Z",
        revoked_reason: "registration cancelled",
      },
    });
    mockListEvents.mockResolvedValue([event]);
    mockGetEvent.mockResolvedValue(event);

    render(<EmployeeEventDetailPage claims={claims} />);

    expect(
      await screen.findByRole("button", { name: "已取消" }),
    ).toBeDisabled();
    expect(
      screen.queryByText("二維碼已移到我的票券詳細頁，入場時再開啟即可。"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "查看這張票券" }),
    ).not.toBeInTheDocument();
  });

  it("does not submit from event detail when the current user is already registered", async () => {
    const event = eventFixture({
      current_user_registration_id: "R-existing",
      current_user_status: "confirmed",
    });
    mockListEvents.mockResolvedValue([event]);
    mockGetEvent.mockResolvedValue(event);

    render(<EmployeeEventDetailPage claims={claims} />);

    const existingButton = await screen.findByRole("button", {
      name: "已報名",
    });
    expect(existingButton).toBeDisabled();

    await userEvent.click(existingButton);

    expect(mockBookEvent).not.toHaveBeenCalled();
  });

  it("explains the cancellation outcome after a successful cancel", async () => {
    const event = eventFixture({
      current_user_registration_id: "R-cancel",
      current_user_status: "confirmed",
      title: "台北家庭電影夜",
      current_user_ticket: {
        ticket_id: "T-cancel",
        registration_id: "R-cancel",
        event_id: "evt-1",
        employee_id: "E1001",
        status: "active",
        signed_token: "signed-secret",
        issued_at: "2026-05-06T10:00:00Z",
      },
    });
    mockListEvents.mockResolvedValue([event]);
    mockGetEvent.mockResolvedValue(event);
    mockCancelMyRegistration.mockResolvedValue({
      registration: {
        registration_id: "R-cancel",
        event_id: "evt-1",
        employee_id: "E1001",
        status: "cancelled",
        idempotency_key: "book-evt-1-E1001",
        cancel_idempotency_key: "cancel-R-cancel-E1001",
        cancelled_at: "2026-05-16T10:00:00Z",
        cancel_reason: "employee cancellation",
        created_at: "2026-05-06T10:00:00Z",
      },
      remaining_capacity: 4,
      message: "cancelled",
    });

    render(<EmployeeEventDetailPage claims={claims} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "取消報名" }),
    );

    const dialog = await screen.findByRole("alertdialog");
    expect(dialog).toBeInTheDocument();
    expect(within(dialog).getByText("台北家庭電影夜")).toBeInTheDocument();
    expect(
      screen.getByText("已核發票券會同步失效，不能再用於入場。"),
    ).toBeInTheDocument();
    expect(mockCancelMyRegistration).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("button", { name: "確認取消報名" }));

    expect(await screen.findByText("報名已取消")).toBeInTheDocument();
    expect(screen.getByText(/已核發票券會同步失效/)).toBeInTheDocument();
    expect(screen.getByText(/請聯絡活動主辦/)).toBeInTheDocument();
  });
});
