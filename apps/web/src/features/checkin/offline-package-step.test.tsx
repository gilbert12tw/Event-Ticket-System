import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { OfflineCheckinPackage } from "@/lib/api";
import type { StoredCheckinPackage } from "@/lib/offline/checkin-store";

const listAdminEvents = vi.fn();
const offlineCheckinPackage = vi.fn();
vi.mock("@/lib/api", async () => {
  // Keep real exports (e.g. ApiError used by errorMessage) and override calls.
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    listAdminEvents: (...a: unknown[]) => listAdminEvents(...a),
    offlineCheckinPackage: (...a: unknown[]) => offlineCheckinPackage(...a),
  };
});

const loadActivePackages = vi.fn();
const savePackage = vi.fn();
vi.mock("@/lib/offline/checkin-store", () => ({
  loadActivePackages: (...a: unknown[]) => loadActivePackages(...a),
  savePackage: (...a: unknown[]) => savePackage(...a),
}));

const isOffline = vi.fn();
vi.mock("@/lib/offline/auth-cache", () => ({
  isOffline: (...a: unknown[]) => isOffline(...a),
}));

const isPackageExpired = vi.fn();
vi.mock("@/lib/offline/checkin-logic", () => ({
  isPackageExpired: (...a: unknown[]) => isPackageExpired(...a),
}));

import { OfflinePackageStep } from "./offline-package-step";

function testPackage(
  overrides: Partial<OfflineCheckinPackage> = {},
): OfflineCheckinPackage {
  return {
    batch_id: "batch-1",
    event_id: "evt-1",
    device_id: "gate-offline-1",
    valid_until: "2999-01-01T00:00:00Z",
    package_signature: "sig-abc",
    ticket_count: 3,
    tickets: [],
    ...overrides,
  };
}

function stored(
  pkg: OfflineCheckinPackage,
  overrides: Partial<StoredCheckinPackage> = {},
): StoredCheckinPackage {
  return {
    batch_id: pkg.batch_id,
    package: pkg,
    scans: [],
    status: "active",
    downloaded_at: "2026-06-01T00:00:00Z",
    staff_id: "staff-1",
    ...overrides,
  };
}

const events = [
  { event_id: "evt-1", title: "Family Night" },
  { event_id: "evt-2", title: "Town Hall" },
];

beforeEach(() => {
  vi.clearAllMocks();
  isOffline.mockReturnValue(false);
  isPackageExpired.mockReturnValue(false);
  listAdminEvents.mockResolvedValue(events);
  loadActivePackages.mockResolvedValue([]);
});

describe("OfflinePackageStep — download flow", () => {
  it("downloads a package, saves it, and reports it ready", async () => {
    const pkg = testPackage();
    offlineCheckinPackage.mockResolvedValue(pkg);
    savePackage.mockResolvedValue(stored(pkg));
    loadActivePackages
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([stored(pkg)]);
    const onPackageReady = vi.fn();

    render(
      <OfflinePackageStep staffID="staff-1" onPackageReady={onPackageReady} />,
    );

    await waitFor(() =>
      expect(screen.getByText("Family Night")).toBeInTheDocument(),
    );
    await userEvent.click(screen.getByRole("button", { name: /下載離線名單/ }));

    await waitFor(() =>
      expect(offlineCheckinPackage).toHaveBeenCalledWith(
        "evt-1",
        "gate-offline-1",
      ),
    );
    expect(savePackage).toHaveBeenCalledWith(pkg, "staff-1");
    expect(onPackageReady).toHaveBeenCalledWith(stored(pkg));
    expect(
      await screen.findByText(/已下載批次 batch-1，共 3 張票券。/),
    ).toBeInTheDocument();
  });

  it("surfaces an error message when the download fails", async () => {
    offlineCheckinPackage.mockRejectedValue(new Error("download failed"));
    render(<OfflinePackageStep staffID="staff-1" onPackageReady={vi.fn()} />);

    await waitFor(() =>
      expect(screen.getByText("Family Night")).toBeInTheDocument(),
    );
    await userEvent.click(screen.getByRole("button", { name: /下載離線名單/ }));

    expect(await screen.findByText("download failed")).toBeInTheDocument();
    expect(savePackage).not.toHaveBeenCalled();
  });

  it("reloads events when the reload button is clicked", async () => {
    render(<OfflinePackageStep staffID="staff-1" onPackageReady={vi.fn()} />);
    await waitFor(() => expect(listAdminEvents).toHaveBeenCalledTimes(1));

    await userEvent.click(screen.getByRole("button", { name: "重新載入活動" }));
    await waitFor(() => expect(listAdminEvents).toHaveBeenCalledTimes(2));
  });
});

describe("OfflinePackageStep — restore from IDB", () => {
  it("auto-selects the only active package", async () => {
    const pkg = testPackage();
    loadActivePackages.mockResolvedValue([stored(pkg)]);
    const onPackageReady = vi.fn();

    render(
      <OfflinePackageStep staffID="staff-1" onPackageReady={onPackageReady} />,
    );

    await waitFor(() =>
      expect(onPackageReady).toHaveBeenCalledWith(stored(pkg)),
    );
    // Preview shows the restored batch metadata.
    expect(await screen.findByText("batch-1")).toBeInTheDocument();
  });

  it("offers a selector when several active packages exist", async () => {
    const pkgA = testPackage({ batch_id: "batch-a" });
    const pkgB = testPackage({ batch_id: "batch-b", ticket_count: 5 });
    loadActivePackages.mockResolvedValue([stored(pkgA), stored(pkgB)]);
    const onPackageReady = vi.fn();

    render(
      <OfflinePackageStep staffID="staff-1" onPackageReady={onPackageReady} />,
    );

    const selector = await screen.findByLabelText("已下載離線名單");
    expect(onPackageReady).not.toHaveBeenCalled();

    await userEvent.click(selector);
    await userEvent.click(
      await screen.findByRole("option", { name: /batch-b/ }),
    );
    expect(onPackageReady).toHaveBeenCalledWith(stored(pkgB));
  });

  it("ignores IDB restore failures", async () => {
    loadActivePackages.mockRejectedValue(new Error("idb down"));
    const onPackageReady = vi.fn();

    render(
      <OfflinePackageStep staffID="staff-1" onPackageReady={onPackageReady} />,
    );

    await waitFor(() => expect(listAdminEvents).toHaveBeenCalled());
    expect(onPackageReady).not.toHaveBeenCalled();
  });

  it("shows the offline notice when reload fails with a restored package", async () => {
    isOffline.mockReturnValue(true);
    const pkg = testPackage();
    loadActivePackages.mockResolvedValue([stored(pkg)]);
    listAdminEvents
      .mockResolvedValueOnce(events)
      .mockRejectedValueOnce(new Error("network down"));
    const onPackageReady = vi.fn();

    render(
      <OfflinePackageStep staffID="staff-1" onPackageReady={onPackageReady} />,
    );

    await waitFor(() => expect(onPackageReady).toHaveBeenCalled());
    await userEvent.click(screen.getByRole("button", { name: "重新載入活動" }));

    expect(
      await screen.findByText("離線模式：使用已下載的離線名單"),
    ).toBeInTheDocument();
  });

  it("warns when the restored package is expired", async () => {
    isPackageExpired.mockReturnValue(true);
    const pkg = testPackage();
    loadActivePackages.mockResolvedValue([stored(pkg)]);

    render(<OfflinePackageStep staffID="staff-1" onPackageReady={vi.fn()} />);

    expect(
      await screen.findByText(/離線名單已過期，請連線後重新下載。/),
    ).toBeInTheDocument();
  });
});
