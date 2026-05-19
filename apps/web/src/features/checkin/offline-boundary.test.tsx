import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { OfflineCheckinBoundaryPage } from "./pages";
import {
  listAdminEvents,
  offlineCheckinPackage,
  syncOfflineCheckins,
} from "@/lib/api";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    listAdminEvents: vi.fn(),
    offlineCheckinPackage: vi.fn(),
    syncOfflineCheckins: vi.fn(),
  };
});

describe("OfflineCheckinBoundaryPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("loads events and creates an offline package", async () => {
    listAdminEvents.mockResolvedValue([
      {
        event_id: "evt-1",
        title: "Family Night",
        description: "event",
        location: "Taipei",
        starts_at: "2026-05-10T09:00:00Z",
        registration_start: "2026-05-08T09:00:00Z",
        registration_close: "2026-05-09T09:00:00Z",
        capacity: 10,
        status: "published",
        allocation_mode: "FCFS",
        created_by: "admin-1",
        created_at: "2026-05-06T10:00:00Z",
        updated_at: "2026-05-06T10:00:00Z",
        rule: {
          department: "Engineering",
          site: "Taipei",
          min_grade: 3,
          employment_status: "active",
        },
        eligible: true,
        eligibility_reason: "ok",
        confirmed_count: 0,
        waitlist_count: 0,
        remaining_capacity: 10,
        current_user_status: "",
      },
    ]);
    offlineCheckinPackage.mockResolvedValue({
      batch_id: "batch-1",
      event_id: "evt-1",
      device_id: "gate-1",
      valid_until: "2026-05-06T14:00:00Z",
      package_signature: "sig-1",
      ticket_count: 2,
      tickets: [
        {
          ticket_id: "t1",
          employee_id: "E1001",
          token_hash: "hash-1",
          holder: {
            display_name: "Ariel Chen",
            department: "Engineering",
            city: "Taipei",
          },
          family_count: 0,
        },
        {
          ticket_id: "t2",
          employee_id: "E1002",
          token_hash: "hash-2",
          holder: {
            display_name: "Ben Lin",
            department: "Engineering",
            city: "Taipei",
          },
          family_count: 1,
        },
      ],
    });

    render(<OfflineCheckinBoundaryPage />);

    await waitFor(() =>
      expect(screen.getByText("Family Night")).toBeInTheDocument(),
    );
    const downloadButton = await screen.findByRole("button", {
      name: "下載離線名單",
    });
    await userEvent.click(downloadButton);

    await waitFor(() =>
      expect(offlineCheckinPackage).toHaveBeenCalledWith(
        "evt-1",
        "gate-offline-1",
      ),
    );
    await waitFor(() =>
      expect(screen.getByText("Ariel Chen")).toBeInTheDocument(),
    );
    expect(screen.getByText(/E1001/)).toBeInTheDocument();
    expect(screen.queryByText("hash-1")).not.toBeInTheDocument();
  });

  it("submits a scan batch and renders summary", async () => {
    listAdminEvents.mockResolvedValue([
      {
        event_id: "evt-1",
        title: "Family Night",
        description: "event",
        location: "Taipei",
        starts_at: "2026-05-10T09:00:00Z",
        registration_start: "2026-05-08T09:00:00Z",
        registration_close: "2026-05-09T09:00:00Z",
        capacity: 10,
        status: "published",
        allocation_mode: "FCFS",
        created_by: "admin-1",
        created_at: "2026-05-06T10:00:00Z",
        updated_at: "2026-05-06T10:00:00Z",
        rule: {
          department: "Engineering",
          site: "Taipei",
          min_grade: 3,
          employment_status: "active",
        },
        eligible: true,
        eligibility_reason: "ok",
        confirmed_count: 0,
        waitlist_count: 0,
        remaining_capacity: 10,
        current_user_status: "",
      },
    ]);
    offlineCheckinPackage.mockResolvedValue({
      batch_id: "batch-2",
      event_id: "evt-1",
      device_id: "gate-1",
      valid_until: "2026-05-06T14:00:00Z",
      package_signature: "sig-2",
      ticket_count: 1,
      tickets: [
        {
          ticket_id: "t1",
          employee_id: "E1001",
          token_hash: "hash-1",
          holder: {
            display_name: "Ariel Chen",
            department: "Engineering",
            city: "Taipei",
          },
          family_count: 0,
        },
      ],
    });
    syncOfflineCheckins.mockResolvedValue({
      batch_id: "batch-2",
      accepted: 1,
      duplicate: 0,
      conflict: 0,
      results: [
        {
          checkin_id: "c1",
          ticket_id: "t1",
          event_id: "evt-1",
          event_title: "Family Night",
          employee_id: "E1001",
          status: "accepted",
          scanned_at: "2026-05-06T10:10:00Z",
          duplicate: false,
          holder: {
            display_name: "Ariel Chen",
            department: "Engineering",
            city: "Taipei",
          },
          family_count: 0,
        },
      ],
    });

    render(<OfflineCheckinBoundaryPage />);

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "下載離線名單" }),
      ).toBeEnabled(),
    );
    await userEvent.click(screen.getByRole("button", { name: "下載離線名單" }));
    await waitFor(() =>
      expect(screen.getByText("Ariel Chen")).toBeInTheDocument(),
    );

    const batchInput = screen.getByRole("textbox", { name: "掃描批次" });
    await userEvent.clear(batchInput);
    await userEvent.type(batchInput, "signed-token-abc\n");
    await userEvent.click(screen.getByRole("button", { name: "同步名單" }));

    await waitFor(() =>
      expect(syncOfflineCheckins).toHaveBeenCalledWith({
        batch_id: "batch-2",
        event_id: "evt-1",
        device_id: "gate-1",
        package_signature: "sig-2",
        scans: [
          { signed_token: "signed-token-abc", scanned_at: expect.any(String) },
        ],
      }),
    );
    await userEvent.click(screen.getByRole("tab", { name: "3 同步結果" }));
    await waitFor(() => {
      const syncPanel = screen.getByRole("tabpanel").closest(".panel");
      expect(syncPanel).not.toBeNull();
      const tables = within(syncPanel as HTMLElement).getAllByRole("table");
      expect(tables).toHaveLength(1);
      const resultTable = tables[0];
      expect(within(resultTable).getByText("t1")).toBeInTheDocument();
      expect(within(resultTable).getByText(/E1001/)).toBeInTheDocument();
      expect(within(resultTable).getByText("Family Night")).toBeInTheDocument();
      expect(within(resultTable).getByText(/同行 0 人/)).toBeInTheDocument();
      expect(within(resultTable).getByText("驗票成功")).toBeInTheDocument();
    });
  });
});
