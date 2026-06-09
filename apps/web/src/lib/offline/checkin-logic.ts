import type { OfflineCheckinPackage, OfflineTicket } from "@/lib/api";
import type { OfflineScanRecord, StoredCheckinPackage } from "./checkin-store";
import { hashToken } from "./hash";

export type LocalScanJudgment = {
  local_status: "accepted" | "duplicate" | "conflict";
  local_reason?: string;
  matched_ticket?: OfflineTicket;
};

export async function judgeOfflineScan(
  token: string,
  pkg: OfflineCheckinPackage,
  existingScans: OfflineScanRecord[],
  now?: Date,
): Promise<LocalScanJudgment> {
  const trimmed = token.trim();
  if (!trimmed) {
    return { local_status: "conflict", local_reason: "empty_token" };
  }

  if (isPackageExpired(pkg, now)) {
    return { local_status: "conflict", local_reason: "package_expired" };
  }

  const tokenHash = await hashToken(trimmed);

  const matched = pkg.tickets.find((t) => t.token_hash === tokenHash);
  if (!matched) {
    return {
      local_status: "conflict",
      local_reason: "not_in_offline_package",
    };
  }

  const alreadyScanned = existingScans.some(
    (s) => s.token_hash === tokenHash && s.local_status !== "conflict",
  );
  if (alreadyScanned) {
    return {
      local_status: "duplicate",
      local_reason: "duplicate_in_local_batch",
      matched_ticket: matched,
    };
  }

  return { local_status: "accepted", matched_ticket: matched };
}

export function isPackageExpired(
  pkg: OfflineCheckinPackage,
  now: Date = new Date(),
): boolean {
  return new Date(pkg.valid_until) <= now;
}

export function buildSyncPayload(stored: StoredCheckinPackage) {
  return {
    batch_id: stored.package.batch_id,
    event_id: stored.package.event_id,
    device_id: stored.package.device_id,
    package_signature: stored.package.package_signature,
    scans: stored.scans
      .filter((s) => s.sync_status === "syncing")
      .map((s) => ({
        signed_token: s.signed_token,
        scanned_at: s.scanned_at,
      })),
  };
}

export type ScanCounts = {
  accepted: number;
  duplicate: number;
  conflict: number;
  synced: number;
  queued: number;
  failed: number;
};

export function computeScanCounts(scans: OfflineScanRecord[]): ScanCounts {
  const counts: ScanCounts = {
    accepted: 0,
    duplicate: 0,
    conflict: 0,
    synced: 0,
    queued: 0,
    failed: 0,
  };
  for (const s of scans) {
    if (s.local_status === "accepted") counts.accepted++;
    if (s.local_status === "duplicate") counts.duplicate++;
    if (s.local_status === "conflict") counts.conflict++;
    if (s.sync_status === "synced") counts.synced++;
    if (s.sync_status === "queued") counts.queued++;
    if (s.sync_status === "sync_failed") counts.failed++;
  }
  return counts;
}
