import type {
  CheckinResponse,
  OfflineCheckinPackage,
  OfflineTicket,
} from "@/lib/api";
import { getAllItems, getItem, putItem } from "./db";

const STORE = "checkin-packages";

export type OfflineScanRecord = {
  local_scan_id: string;
  signed_token: string;
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

export async function addScanRecord(
  batchID: string,
  record: OfflineScanRecord,
): Promise<void> {
  const stored = await loadPackage(batchID);
  if (!stored) return;
  stored.scans.push(record);
  await putItem(STORE, stored);
}

export async function updateScansFromSync(
  batchID: string,
  results: CheckinResponse[],
): Promise<StoredCheckinPackage | undefined> {
  const stored = await loadPackage(batchID);
  if (!stored) return undefined;

  const resultByHash = new Map<string, CheckinResponse>();
  for (const r of results) {
    const matchingScan = stored.scans.find(
      (s) => s.sync_status === "syncing" && !resultByHash.has(s.token_hash),
    );
    if (matchingScan) {
      resultByHash.set(matchingScan.token_hash, r);
    }
  }

  let resultIdx = 0;
  for (const scan of stored.scans) {
    if (scan.sync_status !== "syncing") continue;
    const result = results[resultIdx++];
    if (!result) continue;
    scan.server_status = result.status;
    scan.server_reason_code = result.reason_code;
    scan.server_conflict_reason = result.conflict_reason;
    scan.server_checkin_id = result.checkin_id;
    scan.sync_status = "synced";
    scan.updated_at = new Date().toISOString();
  }

  await putItem(STORE, stored);
  return stored;
}

export async function markScansAsSyncing(
  batchID: string,
): Promise<StoredCheckinPackage | undefined> {
  const stored = await loadPackage(batchID);
  if (!stored) return undefined;
  for (const scan of stored.scans) {
    if (scan.sync_status === "queued") {
      scan.sync_status = "syncing";
      scan.updated_at = new Date().toISOString();
    }
  }
  await putItem(STORE, stored);
  return stored;
}

export async function markScansSyncFailed(batchID: string): Promise<void> {
  const stored = await loadPackage(batchID);
  if (!stored) return;
  for (const scan of stored.scans) {
    if (scan.sync_status === "syncing") {
      scan.sync_status = "sync_failed";
      scan.updated_at = new Date().toISOString();
    }
  }
  await putItem(STORE, stored);
}

export async function markBatchSynced(batchID: string): Promise<void> {
  const stored = await loadPackage(batchID);
  if (!stored) return;
  stored.status = "synced";
  await putItem(STORE, stored);
}
