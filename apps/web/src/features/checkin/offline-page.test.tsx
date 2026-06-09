import { render, screen, waitFor } from "@testing-library/react";
import { act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoredCheckinPackage } from "@/lib/offline/checkin-store";

const loadPackage = vi.fn();
vi.mock("@/lib/offline/checkin-store", () => ({
  loadPackage: (...a: unknown[]) => loadPackage(...a),
}));

vi.mock("@/lib/offline/auth-cache", () => ({
  loadCachedAuthSession: () => ({ actor: { id: "staff-9" } }),
}));

// Controllable sync hook so handleSync branches can be driven deterministically.
const syncNow = vi.fn();
let hookState = {
  syncState: "idle" as string,
  syncResult: null as unknown,
  syncError: null as string | null,
  canSync: true,
};
vi.mock("./use-offline-sync", () => ({
  useOfflineSync: () => ({ ...hookState, syncNow }),
}));

// Replace the child steps with light stand-ins that expose their callbacks.
vi.mock("./offline-package-step", () => ({
  OfflinePackageStep: ({
    staffID,
    onPackageReady,
  }: {
    staffID: string;
    onPackageReady: (s: StoredCheckinPackage) => void;
  }) => (
    <button
      type="button"
      data-staff={staffID}
      onClick={() => onPackageReady(storedFixture())}
    >
      ready-package
    </button>
  ),
}));

vi.mock("./offline-scan-step", () => ({
  OfflineScanStep: ({
    onScansChanged,
  }: {
    onScansChanged: (scans: unknown[]) => void;
  }) => (
    <button type="button" onClick={() => onScansChanged([{ local_scan_id: "x" }])}>
      add-scan
    </button>
  ),
}));

vi.mock("./offline-result-step", () => ({
  OfflineResultStep: ({ syncResult }: { syncResult: unknown }) => (
    <div data-testid="result-step">{syncResult ? "has-result" : "no-result"}</div>
  ),
}));

import { OfflineCheckinBoundaryPage } from "./offline-page";

function storedFixture(
  overrides: Partial<StoredCheckinPackage> = {},
): StoredCheckinPackage {
  return {
    batch_id: "batch-1",
    package: {
      batch_id: "batch-1",
      event_id: "evt-1",
      device_id: "gate-1",
      valid_until: "2999-01-01T00:00:00Z",
      package_signature: "sig",
      ticket_count: 1,
      tickets: [],
    },
    scans: [],
    status: "active",
    downloaded_at: "2026-06-01T00:00:00Z",
    staff_id: "staff-9",
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  hookState = {
    syncState: "idle",
    syncResult: null,
    syncError: null,
    canSync: true,
  };
  Object.defineProperty(navigator, "onLine", {
    value: true,
    writable: true,
    configurable: true,
  });
  globalThis.history.replaceState({}, "", "/admin/checkin/offline");
});

describe("OfflineCheckinBoundaryPage — tab gating", () => {
  it("starts on the package tab with later tabs disabled", () => {
    render(<OfflineCheckinBoundaryPage />);
    expect(screen.getByRole("tab", { name: "2 掃描驗票" })).toBeDisabled();
    expect(screen.getByRole("tab", { name: "3 同步結果" })).toBeDisabled();
    // staffID resolved from cached session is threaded to the package step.
    expect(screen.getByText("ready-package")).toHaveAttribute(
      "data-staff",
      "staff-9",
    );
  });

  it("moves to the scan tab once a package is ready", async () => {
    render(<OfflineCheckinBoundaryPage />);
    await userEvent.click(screen.getByText("ready-package"));

    await waitFor(() =>
      expect(screen.getByRole("tab", { name: "2 掃描驗票" })).toBeEnabled(),
    );
    expect(await screen.findByText("add-scan")).toBeInTheDocument();
  });

  it("enables the results tab after at least one scan", async () => {
    render(<OfflineCheckinBoundaryPage />);
    await userEvent.click(screen.getByText("ready-package"));
    await userEvent.click(await screen.findByText("add-scan"));

    await waitFor(() =>
      expect(screen.getByRole("tab", { name: "3 同步結果" })).toBeEnabled(),
    );
  });
});

describe("OfflineCheckinBoundaryPage — sync", () => {
  it("stores the synced package and shows results when sync returns an update", async () => {
    const synced = storedFixture({ status: "synced", scans: [{ local_scan_id: "x" } as never] });
    syncNow.mockResolvedValue(synced);
    hookState.syncResult = { results: [] };

    render(<OfflineCheckinBoundaryPage />);
    await userEvent.click(screen.getByText("ready-package"));
    await userEvent.click(await screen.findByText("add-scan"));
    await userEvent.click(screen.getByRole("tab", { name: "3 同步結果" }));

    await userEvent.click(screen.getByRole("button", { name: /立即同步/ }));

    await waitFor(() => expect(syncNow).toHaveBeenCalled());
    expect(loadPackage).not.toHaveBeenCalled();
    expect(screen.getByTestId("result-step")).toHaveTextContent("has-result");
  });

  it("reloads the package when sync returns no update", async () => {
    syncNow.mockResolvedValue(undefined);
    loadPackage.mockResolvedValue(storedFixture({ scans: [{ local_scan_id: "x" } as never] }));
    hookState.syncResult = { results: [] };

    render(<OfflineCheckinBoundaryPage />);
    await userEvent.click(screen.getByText("ready-package"));
    await userEvent.click(await screen.findByText("add-scan"));
    await userEvent.click(screen.getByRole("tab", { name: "3 同步結果" }));

    await userEvent.click(screen.getByRole("button", { name: /立即同步/ }));

    await waitFor(() => expect(loadPackage).toHaveBeenCalledWith("batch-1"));
  });

  it("surfaces a sync error in the results tab", async () => {
    hookState.syncError = "同步失敗了";
    hookState.syncResult = { results: [] };
    syncNow.mockResolvedValue(undefined);
    loadPackage.mockResolvedValue(undefined);

    render(<OfflineCheckinBoundaryPage />);
    await userEvent.click(screen.getByText("ready-package"));
    await userEvent.click(await screen.findByText("add-scan"));
    await userEvent.click(screen.getByRole("tab", { name: "3 同步結果" }));

    expect(screen.getByText("同步失敗了")).toBeInTheDocument();
  });
});

describe("OfflineCheckinBoundaryPage — connectivity badge", () => {
  it("reflects offline and online events", async () => {
    render(<OfflineCheckinBoundaryPage />);
    expect(screen.getByText("線上")).toBeInTheDocument();

    act(() => {
      globalThis.dispatchEvent(new Event("offline"));
    });
    expect(await screen.findByText("離線")).toBeInTheDocument();

    act(() => {
      globalThis.dispatchEvent(new Event("online"));
    });
    expect(await screen.findByText("線上")).toBeInTheDocument();
  });
});
