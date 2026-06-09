import { describe, expect, it } from "vitest";
import type { OfflineCheckinPackage } from "@/lib/api";
import type { OfflineScanRecord, StoredCheckinPackage } from "./checkin-store";
import {
  buildSyncPayload,
  computeScanCounts,
  isPackageExpired,
  judgeOfflineScan,
} from "./checkin-logic";
import { hashToken } from "./hash";

function fakePkg(
  overrides: Partial<OfflineCheckinPackage> = {},
): OfflineCheckinPackage {
  return {
    batch_id: "batch-1",
    event_id: "evt-1",
    device_id: "gate-1",
    valid_until: "2099-12-31T23:59:59Z",
    package_signature: "sig",
    ticket_count: 2,
    tickets: [
      {
        ticket_id: "t1",
        employee_id: "E1001",
        token_hash: "",
        holder: { display_name: "Alice", department: "Eng", city: "Taipei" },
        family_count: 0,
      },
      {
        ticket_id: "t2",
        employee_id: "E1002",
        token_hash: "",
        holder: { display_name: "Bob", department: "HR", city: "Hsinchu" },
        family_count: 1,
      },
    ],
    ...overrides,
  };
}

async function pkgWithHashes(
  tokens: string[],
  overrides: Partial<OfflineCheckinPackage> = {},
): Promise<OfflineCheckinPackage> {
  const base = fakePkg(overrides);
  for (let i = 0; i < tokens.length && i < base.tickets.length; i++) {
    base.tickets[i].token_hash = await hashToken(tokens[i]);
  }
  return base;
}

function scanRecord(overrides: Partial<OfflineScanRecord>): OfflineScanRecord {
  return {
    local_scan_id: "scan-1",
    signed_token: "tok",
    token_hash: "hash",
    scanned_at: "2026-06-01T10:00:00Z",
    local_status: "accepted",
    sync_status: "queued",
    updated_at: "2026-06-01T10:00:00Z",
    ...overrides,
  };
}

describe("judgeOfflineScan", () => {
  it("accepts a valid token on first scan", async () => {
    const pkg = await pkgWithHashes(["token-a", "token-b"]);
    const result = await judgeOfflineScan("token-a", pkg, []);
    expect(result.local_status).toBe("accepted");
    expect(result.matched_ticket?.ticket_id).toBe("t1");
  });

  it("detects duplicate on second scan of same token", async () => {
    const pkg = await pkgWithHashes(["token-a", "token-b"]);
    const hash = await hashToken("token-a");
    const existing = [
      scanRecord({ token_hash: hash, local_status: "accepted" }),
    ];

    const result = await judgeOfflineScan("token-a", pkg, existing);
    expect(result.local_status).toBe("duplicate");
    expect(result.local_reason).toBe("duplicate_in_local_batch");
  });

  it("returns conflict for token not in package", async () => {
    const pkg = await pkgWithHashes(["token-a", "token-b"]);
    const result = await judgeOfflineScan("unknown-token", pkg, []);
    expect(result.local_status).toBe("conflict");
    expect(result.local_reason).toBe("not_in_offline_package");
  });

  it("returns conflict for expired package", async () => {
    const pkg = await pkgWithHashes(["token-a"], {
      valid_until: "2020-01-01T00:00:00Z",
    });
    const result = await judgeOfflineScan("token-a", pkg, []);
    expect(result.local_status).toBe("conflict");
    expect(result.local_reason).toBe("package_expired");
  });

  it("returns conflict for empty token", async () => {
    const pkg = await pkgWithHashes(["token-a"]);
    const result = await judgeOfflineScan("", pkg, []);
    expect(result.local_status).toBe("conflict");
    expect(result.local_reason).toBe("empty_token");
  });

  it("trims whitespace from token", async () => {
    const pkg = await pkgWithHashes(["token-a"]);
    const result = await judgeOfflineScan("  token-a  ", pkg, []);
    expect(result.local_status).toBe("accepted");
  });

  it("does not count conflict scans as duplicates", async () => {
    const pkg = await pkgWithHashes(["token-a"]);
    const hash = await hashToken("token-a");
    const existing = [
      scanRecord({ token_hash: hash, local_status: "conflict" }),
    ];

    const result = await judgeOfflineScan("token-a", pkg, existing);
    expect(result.local_status).toBe("accepted");
  });
});

describe("isPackageExpired", () => {
  it("returns false for future date", () => {
    const pkg = fakePkg({ valid_until: "2099-12-31T23:59:59Z" });
    expect(isPackageExpired(pkg)).toBe(false);
  });

  it("returns true for past date", () => {
    const pkg = fakePkg({ valid_until: "2020-01-01T00:00:00Z" });
    expect(isPackageExpired(pkg)).toBe(true);
  });

  it("returns true when exactly at valid_until", () => {
    const now = new Date("2026-06-01T12:00:00Z");
    const pkg = fakePkg({ valid_until: "2026-06-01T12:00:00Z" });
    expect(isPackageExpired(pkg, now)).toBe(true);
  });
});

describe("buildSyncPayload", () => {
  it("preserves per-scan scanned_at timestamps", () => {
    const stored: StoredCheckinPackage = {
      batch_id: "batch-1",
      package: fakePkg(),
      scans: [
        scanRecord({
          signed_token: "tok-a",
          scanned_at: "2026-06-01T10:01:00Z",
          sync_status: "syncing",
        }),
        scanRecord({
          local_scan_id: "scan-2",
          signed_token: "tok-b",
          scanned_at: "2026-06-01T10:02:00Z",
          sync_status: "syncing",
        }),
        scanRecord({
          local_scan_id: "scan-3",
          signed_token: "tok-c",
          scanned_at: "2026-06-01T10:03:00Z",
          sync_status: "queued",
        }),
      ],
      status: "active",
      downloaded_at: "2026-06-01T09:00:00Z",
      staff_id: "staff-1",
    };

    const payload = buildSyncPayload(stored);
    expect(payload.batch_id).toBe("batch-1");
    expect(payload.scans).toHaveLength(2);
    expect(payload.scans[0].scanned_at).toBe("2026-06-01T10:01:00Z");
    expect(payload.scans[1].scanned_at).toBe("2026-06-01T10:02:00Z");
  });

  it("excludes queued scans from payload", () => {
    const stored: StoredCheckinPackage = {
      batch_id: "batch-1",
      package: fakePkg(),
      scans: [scanRecord({ sync_status: "queued" })],
      status: "active",
      downloaded_at: "2026-06-01T09:00:00Z",
      staff_id: "staff-1",
    };
    const payload = buildSyncPayload(stored);
    expect(payload.scans).toHaveLength(0);
  });
});

describe("computeScanCounts", () => {
  it("tallies all categories", () => {
    const scans: OfflineScanRecord[] = [
      scanRecord({ local_status: "accepted", sync_status: "queued" }),
      scanRecord({ local_status: "accepted", sync_status: "synced" }),
      scanRecord({ local_status: "duplicate", sync_status: "queued" }),
      scanRecord({ local_status: "conflict", sync_status: "sync_failed" }),
    ];
    const counts = computeScanCounts(scans);
    expect(counts.accepted).toBe(2);
    expect(counts.duplicate).toBe(1);
    expect(counts.conflict).toBe(1);
    expect(counts.synced).toBe(1);
    expect(counts.queued).toBe(2);
    expect(counts.failed).toBe(1);
  });

  it("returns zeros for empty array", () => {
    const counts = computeScanCounts([]);
    expect(counts).toEqual({
      accepted: 0,
      duplicate: 0,
      conflict: 0,
      synced: 0,
      queued: 0,
      failed: 0,
    });
  });
});
