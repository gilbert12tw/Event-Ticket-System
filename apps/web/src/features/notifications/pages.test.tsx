import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { NotificationDeliveryPage } from "./pages";
import {
  listNotificationDeliveries,
  retryNotificationDelivery,
} from "@/lib/api";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    listNotificationDeliveries: vi.fn(),
    retryNotificationDelivery: vi.fn(),
  };
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
