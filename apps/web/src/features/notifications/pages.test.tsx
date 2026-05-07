import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { NotificationDeliveryPage } from "./pages";
import { listNotificationDeliveries, retryNotificationDelivery } from "@/lib/api";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    listNotificationDeliveries: vi.fn(),
    retryNotificationDelivery: vi.fn()
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
        employee_id: "E1001",
        channel: "email",
        status: "sent",
        attempts: 1,
        last_error: "",
        created_at: "2026-05-06T10:00:00Z",
        updated_at: "2026-05-06T10:00:00Z"
      }
    ]);

    render(<NotificationDeliveryPage />);

    await waitFor(() => expect(screen.getByText("del-1")).toBeInTheDocument());
    expect(screen.getByText("E1001")).toBeInTheDocument();
    expect(screen.getByText("email")).toBeInTheDocument();
  });

  it("retries a failed delivery and refreshes the row", async () => {
    listNotificationDeliveries.mockResolvedValueOnce([
      {
        delivery_id: "del-2",
        outbox_id: "out-2",
        employee_id: "E1002",
        channel: "in-app",
        status: "failed",
        attempts: 1,
        last_error: "timeout",
        created_at: "2026-05-06T10:00:00Z",
        updated_at: "2026-05-06T10:01:00Z"
      }
    ]);
    retryNotificationDelivery.mockResolvedValue({
      delivery_id: "del-2",
      outbox_id: "out-2",
      employee_id: "E1002",
      channel: "in-app",
      status: "pending",
      attempts: 2,
      last_error: "",
      created_at: "2026-05-06T10:00:00Z",
      updated_at: "2026-05-06T10:01:30Z"
    });

    render(<NotificationDeliveryPage />);

    const retryButton = await screen.findByRole("button", { name: "Retry" });
    await userEvent.click(retryButton);

    await waitFor(() => expect(retryNotificationDelivery).toHaveBeenCalledWith("del-2"));
    await waitFor(() => {
      const row = screen.getByText("del-2").closest("tr");
      expect(row).not.toBeNull();
      const scoped = within(row as HTMLTableRowElement);
      expect(scoped.getByText("pending")).toBeInTheDocument();
    });
    expect(screen.getByText("已重試投遞 del-2。")).toBeInTheDocument();
  });
});
