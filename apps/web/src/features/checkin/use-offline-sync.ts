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

export function useOfflineSync(batchID: string | undefined) {
  const [syncState, setSyncState] = useState<SyncState>("idle");
  const [syncResult, setSyncResult] =
    useState<OfflineCheckinSyncResponse | null>(null);
  const [syncError, setSyncError] = useState("");
  const syncingRef = useRef(false);

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

      const payload = buildSyncPayload(stored);
      if (payload.scans.length === 0) {
        setSyncState("idle");
        return stored;
      }

      const result = await syncOfflineCheckins(payload);
      setSyncResult(result);

      const updated = await updateScansFromSync(batchID, result.results);
      await markBatchSynced(batchID);
      setSyncState("synced");
      return updated;
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

    const trySync = () => {
      if (navigator.onLine && !syncingRef.current) {
        void syncNow();
      }
    };

    globalThis.addEventListener("online", trySync);
    document.addEventListener("visibilitychange", () => {
      if (document.visibilityState === "visible") trySync();
    });

    return () => {
      globalThis.removeEventListener("online", trySync);
    };
  }, [batchID, syncNow]);

  const canSync = Boolean(
    batchID && syncState !== "syncing" && navigator.onLine,
  );

  return { syncState, syncResult, syncError, syncNow, canSync, loadPackage };
}
