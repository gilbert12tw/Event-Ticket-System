import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { OfflineCheckinBoundaryPage } from "./pages";
import { listAdminEvents, offlineCheckinPackage } from "@/lib/api";
import type { OfflineCheckinPackage } from "@/lib/api";
import { eventFixture } from "@/test/event-fixtures";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    listAdminEvents: vi.fn(),
    offlineCheckinPackage: vi.fn(),
    syncOfflineCheckins: vi.fn(),
  };
});

vi.mock("@/lib/offline/auth-cache", () => ({
  isOffline: vi.fn(() => false),
}));

const mockSavePackage = vi.fn((pkg: OfflineCheckinPackage) =>
  Promise.resolve({
    batch_id: pkg.batch_id,
    package: pkg,
    scans: [],
    status: "active" as const,
    downloaded_at: "2026-06-01T00:00:00Z",
    staff_id: "",
  }),
);

vi.mock("@/lib/offline/checkin-store", () => ({
  savePackage: (...args: unknown[]) =>
    mockSavePackage(...(args as [OfflineCheckinPackage])),
  loadPackage: vi.fn(() => Promise.resolve(undefined)),
  loadActivePackages: vi.fn(() => Promise.resolve([])),
  addScanRecord: vi.fn(() => Promise.resolve()),
  markScansAsSyncing: vi.fn(() => Promise.resolve(undefined)),
  markScansSyncFailed: vi.fn(() => Promise.resolve()),
  markBatchSynced: vi.fn(() => Promise.resolve()),
  updateScansFromSync: vi.fn(() => Promise.resolve(undefined)),
}));

vi.mock("@/lib/offline/checkin-logic", async () => {
  const actual = await vi.importActual<
    typeof import("@/lib/offline/checkin-logic")
  >("@/lib/offline/checkin-logic");
  return {
    ...actual,
    judgeOfflineScan: vi.fn(() =>
      Promise.resolve({ local_status: "accepted", matched_ticket: null }),
    ),
  };
});

vi.mock("@/lib/offline/hash", () => ({
  hashToken: vi.fn(() => Promise.resolve("fakehash")),
}));

describe("OfflineCheckinBoundaryPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    globalThis.history.replaceState({}, "", "/admin/checkin/offline");
  });

  it("loads events and creates an offline package", async () => {
    listAdminEvents.mockResolvedValue([offlineEvent()]);
    offlineCheckinPackage.mockResolvedValue(testPackage());

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

    expect(mockSavePackage).toHaveBeenCalled();
  });

  it("shows scan tab after package download", async () => {
    listAdminEvents.mockResolvedValue([offlineEvent()]);
    offlineCheckinPackage.mockResolvedValue(testPackage());

    render(<OfflineCheckinBoundaryPage />);

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "下載離線名單" }),
      ).toBeEnabled(),
    );
    await userEvent.click(screen.getByRole("button", { name: "下載離線名單" }));

    await waitFor(() =>
      expect(screen.getByRole("tab", { name: "2 掃描驗票" })).toBeEnabled(),
    );
  });
});

function offlineEvent() {
  return eventFixture({
    event_id: "evt-1",
    title: "Family Night",
    description: "event",
    location: "Taipei",
    starts_at: "2026-05-10T09:00:00Z",
    registration_start: "2026-05-08T09:00:00Z",
    registration_close: "2026-05-09T09:00:00Z",
    allocation_mode: "FCFS",
    confirmed_count: 0,
    remaining_capacity: 10,
    rule: {
      department: "Engineering",
      site: "Taipei",
      min_grade: 3,
      employment_status: "active",
    },
  });
}

function testPackage() {
  return {
    batch_id: "batch-1",
    event_id: "evt-1",
    device_id: "gate-offline-1",
    valid_until: "2099-05-06T14:00:00Z",
    package_signature: "sig-1",
    ticket_count: 2,
    tickets: [
      offlineTicket("t1", "E1001", "Ariel Chen"),
      offlineTicket("t2", "E1002", "Ben Lin", 1),
    ],
  };
}

function offlineTicket(
  ticket_id: string,
  employee_id: string,
  display_name: string,
  family_count = 0,
) {
  return {
    ticket_id,
    employee_id,
    token_hash: ticket_id.replace("t", "hash-"),
    holder: { display_name, department: "Engineering", city: "Taipei" },
    family_count,
  };
}
