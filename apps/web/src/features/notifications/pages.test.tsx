import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { NotificationDeliveryPage, UserNotificationsPage } from "./pages";
import {
  listEvents,
  listNotificationDeliveries,
  listTickets,
  retryNotificationDelivery,
} from "@/lib/api";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    listEvents: vi.fn(),
    listNotificationDeliveries: vi.fn(),
    listTickets: vi.fn(),
    retryNotificationDelivery: vi.fn(),
  };
});

describe("UserNotificationsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(listEvents).mockResolvedValue([]);
    vi.mocked(listTickets).mockResolvedValue([]);
  });

  it("combines registration and ticket notifications without exposing raw internals", async () => {
    vi.mocked(listEvents).mockResolvedValue([
      {
        event_id: "evt-waitlist",
        title: "家庭電影夜",
        description: "Demo",
        location: "Taipei HQ",
        event_city: "Taipei",
        event_site: "Taipei",
        starts_at: "2026-06-10T10:00:00Z",
        registration_start: "2026-06-01T10:00:00Z",
        registration_close: "2026-06-05T10:00:00Z",
        capacity_type: "limited",
        capacity: 10,
        allows_family: false,
        allocation_mode: "fcfs",
        status: "published",
        created_by: "admin-1",
        created_at: "2026-05-31T08:00:00Z",
        updated_at: "2026-05-31T08:00:00Z",
        rule: {
          department: "Engineering",
          site: "Taipei",
          min_grade: 5,
          employment_status: "active",
        },
        confirmed_count: 10,
        waitlist_count: 1,
        remaining_capacity: 0,
        current_user_status: "waitlisted",
        no_show_cooldown: { active: false },
      },
    ]);
    vi.mocked(listTickets).mockResolvedValue([
      {
        ticket_id: "ticket-1",
        registration_id: "reg-1",
        event_id: "evt-waitlist",
        employee_id: "E1001",
        status: "active",
        issued_at: "2026-06-05T10:00:00Z",
        event_title: "家庭電影夜",
        non_transferable: true,
      },
    ]);

    render(<UserNotificationsPage />);

    await waitFor(() =>
      expect(screen.getAllByText("家庭電影夜").length).toBeGreaterThan(0),
    );
    expect(screen.getByLabelText("通知摘要")).toHaveTextContent("通知2");
    expect(screen.getByLabelText("通知摘要")).toHaveTextContent("候補更新1");
    expect(screen.getByLabelText("通知摘要")).toHaveTextContent("票券更新1");
    expect(screen.getAllByText(/候補/).length).toBeGreaterThan(0);
    expect(screen.getAllByText("可使用").length).toBeGreaterThan(0);
    const subjectLinks = screen.getAllByRole("link", { name: "家庭電影夜" });
    expect(
      subjectLinks.some(
        (link) =>
          link.getAttribute("href") ===
          "/user/events/detail?event_id=evt-waitlist",
      ),
    ).toBe(true);
    expect(
      subjectLinks.some(
        (link) =>
          link.getAttribute("href") === "/user/tickets?ticket_id=ticket-1",
      ),
    ).toBe(true);
    expect(
      screen.getAllByText("票券已核發，可於現場驗票使用。").length,
    ).toBeGreaterThan(0);
    expect(screen.queryByText("waitlisted")).not.toBeInTheDocument();
    expect(screen.queryByText("active")).not.toBeInTheDocument();
    expect(screen.queryByText("evt-waitlist")).not.toBeInTheDocument();
    expect(screen.queryByText("ticket-1")).not.toBeInTheDocument();
    expect(screen.queryByText("E1001")).not.toBeInTheDocument();
  });

  it("surfaces loading failures and lets the user retry the API boundary", async () => {
    vi.mocked(listEvents)
      .mockRejectedValueOnce(new Error("notification API unavailable"))
      .mockResolvedValueOnce([]);
    vi.mocked(listTickets).mockResolvedValue([]);

    render(<UserNotificationsPage />);

    expect(
      await screen.findByText("notification API unavailable"),
    ).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "重新整理" }));

    await waitFor(() => expect(listEvents).toHaveBeenCalledTimes(2));
    expect(screen.getByText("尚無通知")).toBeInTheDocument();
  });
});

describe("NotificationDeliveryPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders delivery rows from the API", async () => {
    listNotificationDeliveries.mockResolvedValue([
      {
        delivery_id: "del-1",
        outbox_id: "out-1",
        employee_ref: "E100****",
        channel: "email",
        status: "sent",
        attempts: 1,
        last_error: "",
        created_at: "2026-05-06T10:00:00Z",
        updated_at: "2026-05-06T10:00:00Z",
      },
    ]);

    render(<NotificationDeliveryPage />);

    await waitFor(() =>
      expect(screen.getAllByText("del-1").length).toBeGreaterThan(0),
    );
    expect(screen.getAllByText("E100****").length).toBeGreaterThan(0);
    expect(screen.queryByText("E1001")).not.toBeInTheDocument();
    expect(screen.getAllByText("電子郵件").length).toBeGreaterThan(0);
  });

  it("retries a failed delivery and refreshes the row", async () => {
    listNotificationDeliveries.mockResolvedValueOnce([
      {
        delivery_id: "del-2",
        outbox_id: "out-2",
        employee_ref: "E100****",
        channel: "email",
        status: "failed",
        attempts: 1,
        last_error: "timeout",
        created_at: "2026-05-06T10:00:00Z",
        updated_at: "2026-05-06T10:01:00Z",
      },
    ]);
    retryNotificationDelivery.mockResolvedValue({
      delivery_id: "del-2",
      outbox_id: "out-2",
      employee_ref: "E100****",
      channel: "email",
      status: "pending",
      attempts: 2,
      last_error: "",
      created_at: "2026-05-06T10:00:00Z",
      updated_at: "2026-05-06T10:01:30Z",
    });

    render(<NotificationDeliveryPage />);

    const retryButtons = await screen.findAllByRole("button", {
      name: "重試第 2 次",
    });
    await userEvent.click(retryButtons[0]);
    await userEvent.click(screen.getByRole("button", { name: "確認重試" }));

    await waitFor(() =>
      expect(retryNotificationDelivery).toHaveBeenCalledWith("del-2"),
    );
    await waitFor(() => {
      const row = screen
        .getAllByText("del-2")
        .find((element) => element.closest("tr"))
        ?.closest("tr");
      expect(row).not.toBeNull();
      const scoped = within(row as HTMLTableRowElement);
      expect(scoped.getByText("待處理")).toBeInTheDocument();
    });
    expect(screen.getByText("已重試投遞 del-2。")).toBeInTheDocument();
  });

  it("disables retry for pending and non-email deliveries", async () => {
    listNotificationDeliveries.mockResolvedValue([
      {
        delivery_id: "del-pending",
        outbox_id: "out-pending",
        employee_ref: "E100****",
        channel: "email",
        status: "pending",
        attempts: 1,
        last_error: "",
        created_at: "2026-05-06T10:00:00Z",
        updated_at: "2026-05-06T10:01:00Z",
      },
      {
        delivery_id: "del-in-app",
        outbox_id: "out-in-app",
        employee_ref: "E200****",
        channel: "in_app",
        status: "failed",
        attempts: 1,
        last_error: "timeout",
        created_at: "2026-05-06T10:00:00Z",
        updated_at: "2026-05-06T10:01:00Z",
      },
    ]);

    render(<NotificationDeliveryPage />);

    const disabledRetryButtons = await screen.findAllByRole("button", {
      name: "不可重試",
    });
    expect(disabledRetryButtons.length).toBeGreaterThanOrEqual(2);
    disabledRetryButtons.forEach((button) => expect(button).toBeDisabled());
    expect(screen.getAllByText("待處理中，不可重試").length).toBeGreaterThan(0);
    expect(screen.getAllByText("僅電子郵件投遞可重試").length).toBeGreaterThan(
      0,
    );
  });
});
