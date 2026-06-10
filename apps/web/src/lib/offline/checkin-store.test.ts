import { beforeEach, describe, expect, it, vi } from "vitest";
import type { CheckinResponse } from "@/lib/api";

// In-memory IndexedDB stand-in so the store logic can be tested without jsdom IDB.
const store = new Map<string, unknown>();

vi.mock("./db", () => ({
  putItem: vi.fn(async (_name: string, item: { batch_id: string }) => {
    store.set(item.batch_id, structuredClone(item));
  }),
  getItem: vi.fn(async (_name: string, key: string) =>
    store.has(key) ? structuredClone(store.get(key)) : undefined,
  ),
  getAllItems: vi.fn(async () =>
    Array.from(store.values()).map((v) => structuredClone(v)),
  ),
}));

import {
  addScanRecord,
  markBatchSynced,
  markScansAsSyncing,
  markScansSyncFailed,
  savePackage,
  updateScansFromSync,
  type OfflineScanRecord,
} from "./checkin-store";
import { buildSyncPayload } from "./checkin-logic";
import type { OfflineCheckinPackage } from "@/lib/api";

const BATCH = "batch-1";

function testPackage(): OfflineCheckinPackage {
  return {
    batch_id: BATCH,
    event_id: "evt-1",
    device_id: "gate-1",
    valid_until: "2999-01-01T00:00:00Z",
    package_signature: "sig",
    ticket_count: 2,
    tickets: [
      {
        ticket_id: "t-1",
        employee_id: "E1001",
        token_hash: "hash-1",
        holder: { display_name: "Alice", department: "Eng", city: "Taipei" },
        family_count: 0,
      },
    ],
  };
}

function scan(overrides: Partial<OfflineScanRecord> = {}): OfflineScanRecord {
  return {
    local_scan_id: crypto.randomUUID(),
    signed_token: "signed-token-value",
    token_hash: "hash-1",
    scanned_at: "2026-06-09T00:00:00Z",
    local_status: "accepted",
    matched_ticket: {
      ticket_id: "t-1",
      employee_id: "E1001",
      token_hash: "hash-1",
      holder: { display_name: "Alice", department: "Eng", city: "Taipei" },
      family_count: 0,
    },
    sync_status: "queued",
    updated_at: "2026-06-09T00:00:00Z",
    ...overrides,
  };
}

function result(): CheckinResponse {
  return {
    checkin_id: "c-1",
    ticket_id: "t-1",
    event_id: "evt-1",
    employee_id: "E1001",
    status: "checked_in",
    reason_code: "accepted",
  } as CheckinResponse;
}

beforeEach(() => {
  store.clear();
});

describe("addScanRecord — closed-batch guard", () => {
  it("persists into an active batch and reports success", async () => {
    await savePackage(testPackage(), "staff-1");
    expect(await addScanRecord(BATCH, scan())).toBe(true);
  });

  it("rejects scans for a missing or already-synced batch", async () => {
    expect(await addScanRecord("missing", scan())).toBe(false);

    await savePackage(testPackage(), "staff-1");
    await markBatchSynced(BATCH);
    expect(await addScanRecord(BATCH, scan())).toBe(false);
  });
});

describe("markScansAsSyncing — #1 retry", () => {
  it("re-promotes sync_failed scans so they can be retried", async () => {
    await savePackage(testPackage(), "staff-1");
    await addScanRecord(BATCH, scan({ sync_status: "queued" }));

    // First sync attempt fails.
    await markScansAsSyncing(BATCH);
    await markScansSyncFailed(BATCH);

    // Second attempt must re-promote the failed scan.
    const updated = await markScansAsSyncing(BATCH);
    expect(updated?.scans[0].sync_status).toBe("syncing");
    expect(buildSyncPayload(updated!).scans).toHaveLength(1);
  });

  it("promotes queued scans to syncing", async () => {
    await savePackage(testPackage(), "staff-1");
    await addScanRecord(BATCH, scan({ sync_status: "queued" }));
    const updated = await markScansAsSyncing(BATCH);
    expect(updated?.scans[0].sync_status).toBe("syncing");
  });
});

describe("updateScansFromSync — #3 positional mapping", () => {
  it("maps results positionally and reports complete", async () => {
    await savePackage(testPackage(), "staff-1");
    await addScanRecord(BATCH, scan({ sync_status: "syncing" }));
    const update = await updateScansFromSync(BATCH, [result()]);
    expect(update?.complete).toBe(true);
    expect(update?.stored.scans[0].sync_status).toBe("synced");
    expect(update?.stored.scans[0].server_checkin_id).toBe("c-1");
  });

  it("leaves trailing scans syncing when fewer results returned", async () => {
    await savePackage(testPackage(), "staff-1");
    await addScanRecord(BATCH, scan({ sync_status: "syncing" }));
    await addScanRecord(BATCH, scan({ sync_status: "syncing" }));
    const update = await updateScansFromSync(BATCH, [result()]);
    expect(update?.complete).toBe(false);
    expect(update?.stored.scans[0].sync_status).toBe("synced");
    expect(update?.stored.scans[1].sync_status).toBe("syncing");
  });
});

describe("markBatchSynced — #4 PII purge", () => {
  it("closes the batch and strips plaintext token, holder, and name list", async () => {
    await savePackage(testPackage(), "staff-1");
    await addScanRecord(BATCH, scan({ sync_status: "synced" }));
    await markBatchSynced(BATCH);

    const stored = store.get(BATCH) as {
      status: string;
      scans: OfflineScanRecord[];
      package: OfflineCheckinPackage;
    };
    expect(stored.status).toBe("synced");
    expect(stored.package.tickets).toHaveLength(0);
    expect(stored.scans[0].signed_token).toBeUndefined();
    expect(stored.scans[0].matched_ticket).toBeUndefined();
    // Retained for the audit / result view.
    expect(stored.scans[0].token_hash).toBe("hash-1");
  });
});
