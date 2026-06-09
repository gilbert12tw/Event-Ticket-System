import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
  OfflineScanRecord,
  StoredCheckinPackage,
} from "@/lib/offline/checkin-store";

const addScanRecord = vi.fn();
vi.mock("@/lib/offline/checkin-store", () => ({
  addScanRecord: (...a: unknown[]) => addScanRecord(...a),
}));

const judgeOfflineScan = vi.fn();
const computeScanCounts = vi.fn();
const isPackageExpired = vi.fn();
vi.mock("@/lib/offline/checkin-logic", () => ({
  judgeOfflineScan: (...a: unknown[]) => judgeOfflineScan(...a),
  computeScanCounts: (...a: unknown[]) => computeScanCounts(...a),
  isPackageExpired: (...a: unknown[]) => isPackageExpired(...a),
}));

const hashToken = vi.fn();
vi.mock("@/lib/offline/hash", () => ({
  hashToken: (...a: unknown[]) => hashToken(...a),
}));

// Capture the scanner callback so tests can drive handleToken directly,
// including the rapid double-scan path that exercises the processing guard.
let scannerCallback: ((token: string) => void) | null = null;
vi.mock("./mobile-qr-scanner", () => ({
  MobileQrScanner: ({
    onTokenDetected,
  }: {
    onTokenDetected: (token: string) => void;
  }) => {
    scannerCallback = onTokenDetected;
    return <div data-testid="scanner" />;
  },
}));

import { OfflineScanStep } from "./offline-scan-step";

const ZERO_COUNTS = {
  accepted: 0,
  duplicate: 0,
  conflict: 0,
  synced: 0,
  queued: 0,
  failed: 0,
};

function ticket(overrides: Record<string, unknown> = {}) {
  return {
    ticket_id: "t-1",
    employee_id: "E1001",
    token_hash: "hash-1",
    holder: { display_name: "Alice", department: "Eng", city: "Taipei" },
    family_count: 2,
    ...overrides,
  };
}

function storedPackage(
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
      tickets: [ticket()],
    },
    scans: [],
    status: "active",
    downloaded_at: "2026-06-01T00:00:00Z",
    staff_id: "staff-1",
    ...overrides,
  };
}

function scanRecord(
  overrides: Partial<OfflineScanRecord> = {},
): OfflineScanRecord {
  return {
    local_scan_id: crypto.randomUUID(),
    signed_token: "token-value",
    token_hash: "hash-1",
    scanned_at: "2026-06-09T01:02:03Z",
    local_status: "accepted",
    matched_ticket: ticket(),
    sync_status: "queued",
    updated_at: "2026-06-09T01:02:03Z",
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  scannerCallback = null;
  isPackageExpired.mockReturnValue(false);
  computeScanCounts.mockReturnValue({ ...ZERO_COUNTS });
  addScanRecord.mockResolvedValue(undefined);
  hashToken.mockResolvedValue("hash-1");
  judgeOfflineScan.mockResolvedValue({
    local_status: "accepted",
    matched_ticket: ticket(),
  });
});

describe("OfflineScanStep — handleToken judgment flow", () => {
  it("records an accepted scan and shows the success banner", async () => {
    const onScansChanged = vi.fn();
    render(
      <OfflineScanStep stored={storedPackage()} onScansChanged={onScansChanged} />,
    );

    scannerCallback!("signed-token");

    await waitFor(() => expect(addScanRecord).toHaveBeenCalledTimes(1));
    const [batchID, record] = addScanRecord.mock.calls[0];
    expect(batchID).toBe("batch-1");
    expect(record.local_status).toBe("accepted");
    expect(record.token_hash).toBe("hash-1");
    expect(record.signed_token).toBe("signed-token");
    expect(onScansChanged).toHaveBeenCalledWith([expect.objectContaining({
      local_status: "accepted",
    })]);
    expect(await screen.findByText(/本地通過，等待同步/)).toBeInTheDocument();
  });

  it("shows the duplicate banner for a duplicate judgment", async () => {
    judgeOfflineScan.mockResolvedValue({
      local_status: "duplicate",
      matched_ticket: ticket(),
    });
    render(<OfflineScanStep stored={storedPackage()} onScansChanged={vi.fn()} />);

    scannerCallback!("dupe-token");

    expect(
      await screen.findByText(/重複掃描/),
    ).toBeInTheDocument();
  });

  it("maps a conflict reason to its message", async () => {
    judgeOfflineScan.mockResolvedValue({
      local_status: "conflict",
      local_reason: "not_in_offline_package",
    });
    render(<OfflineScanStep stored={storedPackage()} onScansChanged={vi.fn()} />);

    scannerCallback!("conflict-token");

    expect(
      await screen.findByText("衝突：不在離線名單"),
    ).toBeInTheDocument();
  });

  it("falls back to the generic conflict message for an unknown reason", async () => {
    judgeOfflineScan.mockResolvedValue({
      local_status: "conflict",
      local_reason: "something_else",
    });
    render(<OfflineScanStep stored={storedPackage()} onScansChanged={vi.fn()} />);

    scannerCallback!("conflict-token");

    expect(await screen.findByText("衝突：驗票失敗")).toBeInTheDocument();
  });

  it("ignores blank tokens without judging", async () => {
    render(<OfflineScanStep stored={storedPackage()} onScansChanged={vi.fn()} />);

    scannerCallback!("   ");

    await Promise.resolve();
    expect(judgeOfflineScan).not.toHaveBeenCalled();
    expect(addScanRecord).not.toHaveBeenCalled();
  });

  it("swallows judgment errors without recording a scan", async () => {
    judgeOfflineScan.mockRejectedValue(new Error("boom"));
    const onScansChanged = vi.fn();
    render(
      <OfflineScanStep stored={storedPackage()} onScansChanged={onScansChanged} />,
    );

    scannerCallback!("token");

    await waitFor(() => expect(judgeOfflineScan).toHaveBeenCalled());
    expect(addScanRecord).not.toHaveBeenCalled();
    expect(onScansChanged).not.toHaveBeenCalled();
  });
});

describe("OfflineScanStep — processing guard", () => {
  it("drops a second scan while the first is still processing", async () => {
    let resolveJudge: (value: unknown) => void = () => {};
    judgeOfflineScan.mockImplementation(
      () => new Promise((resolve) => (resolveJudge = resolve)),
    );
    render(<OfflineScanStep stored={storedPackage()} onScansChanged={vi.fn()} />);

    scannerCallback!("first");
    scannerCallback!("second");

    expect(judgeOfflineScan).toHaveBeenCalledTimes(1);
    resolveJudge({ local_status: "accepted", matched_ticket: ticket() });
    await waitFor(() => expect(addScanRecord).toHaveBeenCalledTimes(1));
  });
});

describe("OfflineScanStep — manual batch parsing", () => {
  it("processes each non-empty line and clears the textarea", async () => {
    render(<OfflineScanStep stored={storedPackage()} onScansChanged={vi.fn()} />);

    await userEvent.click(screen.getByText("手動輸入簽章碼"));
    const textarea = screen.getByLabelText("簽章碼批次");
    await userEvent.type(textarea, "alpha{Enter}{Enter}  beta  {Enter}");
    await userEvent.click(screen.getByRole("button", { name: /加入掃描/ }));

    await waitFor(() => expect(addScanRecord).toHaveBeenCalledTimes(2));
    expect(judgeOfflineScan).toHaveBeenCalledWith(
      "alpha",
      expect.anything(),
      expect.anything(),
    );
    expect(judgeOfflineScan).toHaveBeenCalledWith(
      "beta",
      expect.anything(),
      expect.anything(),
    );
    expect((textarea as HTMLTextAreaElement).value).toBe("");
  });
});

describe("OfflineScanStep — gating", () => {
  it("blocks scanning and hides the scanner once the batch is synced", async () => {
    render(
      <OfflineScanStep
        stored={storedPackage({ status: "synced" })}
        onScansChanged={vi.fn()}
      />,
    );

    expect(
      screen.getByText(/此批次已同步完成/),
    ).toBeInTheDocument();
    expect(screen.queryByTestId("scanner")).not.toBeInTheDocument();
  });

  it("blocks scanning when the package is expired", () => {
    isPackageExpired.mockReturnValue(true);
    render(<OfflineScanStep stored={storedPackage()} onScansChanged={vi.fn()} />);

    expect(screen.getByText(/離線名單已過期/)).toBeInTheDocument();
    expect(screen.queryByTestId("scanner")).not.toBeInTheDocument();
  });
});

describe("OfflineScanStep — scan table", () => {
  it("renders existing scans with holder, family count, and badges", () => {
    computeScanCounts.mockReturnValue({ ...ZERO_COUNTS, failed: 1 });
    const stored = storedPackage({
      scans: [
        scanRecord({ local_status: "accepted", sync_status: "synced" }),
        scanRecord({
          local_status: "duplicate",
          sync_status: "sync_failed",
          matched_ticket: ticket({ holder: undefined, employee_id: "E2002" }),
        }),
        scanRecord({
          local_status: "conflict",
          sync_status: "syncing",
          matched_ticket: undefined,
        }),
      ],
    });
    render(<OfflineScanStep stored={stored} onScansChanged={vi.fn()} />);

    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("E2002")).toBeInTheDocument();
    // Badge labels collide with KPI labels, so assert at least one appears.
    expect(screen.getAllByText("本地通過").length).toBeGreaterThan(0);
    expect(screen.getAllByText("重複").length).toBeGreaterThan(0);
    expect(screen.getAllByText("衝突").length).toBeGreaterThan(0);
    expect(screen.getAllByText("同步失敗").length).toBeGreaterThan(0);
    expect(screen.getAllByText("已同步").length).toBeGreaterThan(0);
    expect(screen.getByText("同步中")).toBeInTheDocument();
  });

  it("shows the empty state when there are no scans", () => {
    render(<OfflineScanStep stored={storedPackage()} onScansChanged={vi.fn()} />);
    expect(screen.getByText("尚無掃描")).toBeInTheDocument();
  });
});
