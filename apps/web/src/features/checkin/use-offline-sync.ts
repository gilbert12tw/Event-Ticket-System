import { useCallback, useEffect, useRef, useState } from "react";
import { syncOfflineCheckins } from "@/lib/api";
import type { OfflineCheckinSyncResponse } from "@/lib/api";
import {
  loadPackage,
  markBatchSynced,
  markScansAsSyncing,
  markScansSyncFailed,
  updateScansFromSync,
} from "@/lib/offline/checkin-store";
import type { StoredCheckinPackage } from "@/lib/offline/checkin-store";
import { buildSyncPayload } from "@/lib/offline/checkin-logic";

export type SyncState = "idle" | "syncing" | "synced" | "error";

export function useOfflineSync(
  batchID: string | undefined,
  onAutoSynced?: (stored: StoredCheckinPackage) => void,
) {
  const [syncState, setSyncState] = useState<SyncState>("idle");
  const [syncResult, setSyncResult] =
    useState<OfflineCheckinSyncResponse | null>(null);
  const [syncError, setSyncError] = useState("");
  const syncingRef = useRef(false);
  // Ref so the listeners effect does not re-subscribe on every parent render.
  const onAutoSyncedRef = useRef(onAutoSynced);
  onAutoSyncedRef.current = onAutoSynced;

  const syncNow = useCallback(async (): Promise<
    StoredCheckinPackage | undefined
  > => {
    if (!batchID || syncingRef.current) return undefined;
    syncingRef.current = true;
    setSyncState("syncing");
    setSyncError("");

    try {
      const stored = await markScansAsSyncing(batchID);
      if (!stored) {
        setSyncState("error");
        setSyncError("找不到離線名單");
        return undefined;
      }
      if (stored.status === "synced") {
        setSyncState("synced");
        return stored;
      }

      const payload = buildSyncPayload(stored);
      if (payload.scans.length === 0) {
        setSyncState("idle");
        return stored;
      }

      const result = await syncOfflineCheckins(payload);
      setSyncResult(result);

      const update = await updateScansFromSync(batchID, result.results);
      // Only close (and purge) the batch when every syncing scan got a result;
      // otherwise leave unmatched scans as "syncing" and surface a retry prompt
      // instead of falsely reporting the sync as complete.
      if (update?.complete) {
        await markBatchSynced(batchID);
        setSyncState("synced");
        return await loadPackage(batchID);
      }
      setSyncState("error");
      setSyncError("部分掃描未收到伺服器回應，請重新同步。");
      return update?.stored;
    } catch (error) {
      await markScansSyncFailed(batchID).catch(() => {});
      setSyncState("error");
      setSyncError(
        error instanceof Error ? error.message : "同步失敗，請稍後重試",
      );
      return undefined;
    } finally {
      syncingRef.current = false;
    }
  }, [batchID]);

  useEffect(() => {
    if (!batchID) return;

    // Auto-sync runs outside the page's event handlers, so push the refreshed
    // package back up — otherwise the page keeps rendering stale scan state.
    const trySync = () => {
      if (!navigator.onLine || syncingRef.current) return;
      void (async () => {
        await syncNow();
        const refreshed = await loadPackage(batchID);
        if (refreshed) onAutoSyncedRef.current?.(refreshed);
      })();
    };

    const onVisible = () => {
      if (document.visibilityState === "visible") trySync();
    };

    globalThis.addEventListener("online", trySync);
    document.addEventListener("visibilitychange", onVisible);

    return () => {
      globalThis.removeEventListener("online", trySync);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [batchID, syncNow]);

  const canSync = Boolean(
    batchID && syncState !== "syncing" && navigator.onLine,
  );

  return { syncState, syncResult, syncError, syncNow, canSync, loadPackage };
}
