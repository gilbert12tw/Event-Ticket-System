import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const syncOfflineCheckins = vi.fn();
vi.mock("@/lib/api", () => ({
  syncOfflineCheckins: (...args: unknown[]) => syncOfflineCheckins(...args),
}));

const markScansAsSyncing = vi.fn();
const markScansSyncFailed = vi.fn();
const markBatchSynced = vi.fn();
const updateScansFromSync = vi.fn();
const loadPackage = vi.fn();
vi.mock("@/lib/offline/checkin-store", () => ({
  markScansAsSyncing: (...a: unknown[]) => markScansAsSyncing(...a),
  markScansSyncFailed: (...a: unknown[]) => markScansSyncFailed(...a),
  markBatchSynced: (...a: unknown[]) => markBatchSynced(...a),
  updateScansFromSync: (...a: unknown[]) => updateScansFromSync(...a),
  loadPackage: (...a: unknown[]) => loadPackage(...a),
}));

const buildSyncPayload = vi.fn();
vi.mock("@/lib/offline/checkin-logic", () => ({
  buildSyncPayload: (...a: unknown[]) => buildSyncPayload(...a),
}));

import { useOfflineSync } from "./use-offline-sync";

const BATCH = "batch-1";

function storedWith(scanCount: number) {
  return {
    batch_id: BATCH,
    package: { batch_id: BATCH },
    scans: Array.from({ length: scanCount }, (_, i) => ({ local_scan_id: i })),
    status: "active" as const,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  Object.defineProperty(navigator, "onLine", {
    value: true,
    writable: true,
    configurable: true,
  });
  markScansAsSyncing.mockResolvedValue(storedWith(1));
  markScansSyncFailed.mockResolvedValue(undefined);
  markBatchSynced.mockResolvedValue(undefined);
  buildSyncPayload.mockReturnValue({ scans: [{ signed_token: "t" }] });
  loadPackage.mockResolvedValue({ ...storedWith(1), status: "synced" });
});

describe("useOfflineSync", () => {
  it("stays idle and skips the network when there is nothing to sync", async () => {
    buildSyncPayload.mockReturnValue({ scans: [] });
    const { result } = renderHook(() => useOfflineSync(BATCH));

    await act(async () => {
      await result.current.syncNow();
    });

    expect(syncOfflineCheckins).not.toHaveBeenCalled();
    expect(result.current.syncState).toBe("idle");
  });

  it("closes and marks the batch synced when every scan got a result", async () => {
    syncOfflineCheckins.mockResolvedValue({ results: [{ status: "ok" }] });
    updateScansFromSync.mockResolvedValue({
      stored: storedWith(1),
      complete: true,
    });

    const { result } = renderHook(() => useOfflineSync(BATCH));
    await act(async () => {
      await result.current.syncNow();
    });

    expect(markBatchSynced).toHaveBeenCalledWith(BATCH);
    await waitFor(() => expect(result.current.syncState).toBe("synced"));
  });

  it("does NOT close the batch and surfaces an error on a partial result", async () => {
    syncOfflineCheckins.mockResolvedValue({ results: [] });
    updateScansFromSync.mockResolvedValue({
      stored: storedWith(1),
      complete: false,
    });

    const { result } = renderHook(() => useOfflineSync(BATCH));
    await act(async () => {
      await result.current.syncNow();
    });

    expect(markBatchSynced).not.toHaveBeenCalled();
    await waitFor(() => expect(result.current.syncState).toBe("error"));
    expect(result.current.syncError).toContain("部分");
  });

  it("marks scans as sync_failed when the request throws", async () => {
    syncOfflineCheckins.mockRejectedValue(new Error("network down"));

    const { result } = renderHook(() => useOfflineSync(BATCH));
    await act(async () => {
      await result.current.syncNow();
    });

    expect(markScansSyncFailed).toHaveBeenCalledWith(BATCH);
    await waitFor(() => expect(result.current.syncState).toBe("error"));
    expect(result.current.syncError).toBe("network down");
  });

  it("errors when the offline package cannot be loaded", async () => {
    markScansAsSyncing.mockResolvedValue(undefined);

    const { result } = renderHook(() => useOfflineSync(BATCH));
    await act(async () => {
      await result.current.syncNow();
    });

    expect(syncOfflineCheckins).not.toHaveBeenCalled();
    await waitFor(() => expect(result.current.syncState).toBe("error"));
  });

  it("canSync is false without a batch id and true when online with one", () => {
    const without = renderHook(() => useOfflineSync(undefined));
    expect(without.result.current.canSync).toBe(false);

    const withBatch = renderHook(() => useOfflineSync(BATCH));
    expect(withBatch.result.current.canSync).toBe(true);
  });
});
