import type {
  CheckinResponse,
  OfflineCheckinPackage,
  OfflineTicket,
} from "@/lib/api";
import { getAllItems, getItem, putItem } from "./db";

const STORE = "checkin-packages";

export type OfflineScanRecord = {
  local_scan_id: string;
  // Cleared after a successful sync to avoid retaining plaintext tokens on the device.
  signed_token?: string;
  token_hash: string;
  scanned_at: string;
  local_status: "accepted" | "duplicate" | "conflict";
  local_reason?: string;
  matched_ticket?: OfflineTicket;
  sync_status: "queued" | "syncing" | "synced" | "sync_failed";
  server_status?: string;
  server_reason_code?: string;
  server_conflict_reason?: string;
  server_checkin_id?: string;
  updated_at: string;
};

export type StoredCheckinPackage = {
  batch_id: string;
  package: OfflineCheckinPackage;
  scans: OfflineScanRecord[];
  status: "active" | "synced";
  downloaded_at: string;
  staff_id: string;
};

export async function savePackage(
  pkg: OfflineCheckinPackage,
  staffID: string,
): Promise<StoredCheckinPackage> {
  const stored: StoredCheckinPackage = {
    batch_id: pkg.batch_id,
    package: pkg,
    scans: [],
    status: "active",
    downloaded_at: new Date().toISOString(),
    staff_id: staffID,
  };
  await putItem(STORE, stored);
  return stored;
}

export async function loadPackage(
  batchID: string,
): Promise<StoredCheckinPackage | undefined> {
  return getItem<StoredCheckinPackage>(STORE, batchID);
}

export async function loadActivePackages(): Promise<StoredCheckinPackage[]> {
  const all = await getAllItems<StoredCheckinPackage>(STORE);
  return all.filter((p) => p.status === "active");
}

// Returns false when the scan was not persisted (missing batch, or the batch
// was already closed by a sync racing ahead of the UI state).
export async function addScanRecord(
  batchID: string,
  record: OfflineScanRecord,
): Promise<boolean> {
  const stored = await loadPackage(batchID);
  if (!stored || stored.status === "synced") return false;
  const updated: StoredCheckinPackage = {
    ...stored,
    scans: [...stored.scans, record],
  };
  await putItem(STORE, updated);
  return true;
}

export type SyncUpdateResult = {
  stored: StoredCheckinPackage;
  complete: boolean;
};

// The backend replays scans in request order (see replayOfflineSync), so
// results map positionally onto the scans we sent as "syncing". `complete` is
// true only when every syncing scan received a result; otherwise unmatched
// scans stay "syncing" so the next sync retries them and the batch is not
// closed prematurely.
export async function updateScansFromSync(
  batchID: string,
  results: CheckinResponse[],
): Promise<SyncUpdateResult | undefined> {
  const stored = await loadPackage(batchID);
  if (!stored) return undefined;

  const now = new Date().toISOString();
  let resultIdx = 0;
  const scans = stored.scans.map((scan) => {
    if (scan.sync_status !== "syncing") return scan;
    const result = results[resultIdx++];
    if (!result) return scan;
    return {
      ...scan,
      server_status: result.status,
      server_reason_code: result.reason_code,
      server_conflict_reason: result.conflict_reason,
      server_checkin_id: result.checkin_id,
      sync_status: "synced" as const,
      updated_at: now,
    };
  });

  const complete = !scans.some((s) => s.sync_status === "syncing");
  const updated: StoredCheckinPackage = { ...stored, scans };
  await putItem(STORE, updated);
  return { stored: updated, complete };
}

export async function markScansAsSyncing(
  batchID: string,
): Promise<StoredCheckinPackage | undefined> {
  const stored = await loadPackage(batchID);
  if (!stored) return undefined;
  const now = new Date().toISOString();
  const scans = stored.scans.map((scan) =>
    scan.sync_status === "queued" || scan.sync_status === "sync_failed"
      ? { ...scan, sync_status: "syncing" as const, updated_at: now }
      : scan,
  );
  const updated: StoredCheckinPackage = { ...stored, scans };
  await putItem(STORE, updated);
  return updated;
}

export async function markScansSyncFailed(batchID: string): Promise<void> {
  const stored = await loadPackage(batchID);
  if (!stored) return;
  const now = new Date().toISOString();
  const scans = stored.scans.map((scan) =>
    scan.sync_status === "syncing"
      ? { ...scan, sync_status: "sync_failed" as const, updated_at: now }
      : scan,
  );
  await putItem(STORE, { ...stored, scans });
}

// Close a fully-synced batch and purge plaintext PII from the device: the
// signed tokens and holder snapshots are no longer needed once results are in.
// token_hash + server_* fields are retained for the audit/result view.
export async function markBatchSynced(batchID: string): Promise<void> {
  const stored = await loadPackage(batchID);
  if (!stored) return;
  const scans = stored.scans.map(
    ({
      signed_token: _signed_token,
      matched_ticket: _matched_ticket,
      ...rest
    }) => rest,
  );
  const purgedPackage: OfflineCheckinPackage = {
    ...stored.package,
    tickets: [],
  };
  const updated: StoredCheckinPackage = {
    ...stored,
    status: "synced",
    scans,
    package: purgedPackage,
  };
  await putItem(STORE, updated);
}
