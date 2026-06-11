import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { CheckinResponse, OfflineCheckinSyncResponse } from "@/lib/api";
import { OfflineResultStep } from "./offline-result-step";

function result(overrides: Partial<CheckinResponse> = {}): CheckinResponse {
  return {
    checkin_id: "chk-1",
    ticket_id: "tic-1",
    event_id: "evt-1",
    event_title: "Family Night",
    employee_id: "E1001",
    status: "accepted",
    reason_code: "accepted",
    scanned_at: "2026-06-10T10:00:00Z",
    duplicate: false,
    holder: { display_name: "Alice", department: "Eng", city: "Taipei" },
    family_count: 0,
    ...overrides,
  };
}

function syncResult(results: CheckinResponse[]): OfflineCheckinSyncResponse {
  return {
    batch_id: "batch-1",
    accepted: 0,
    duplicate: 0,
    conflict: 0,
    results,
  };
}

describe("OfflineResultStep — reason column", () => {
  it("shows the empty state without a sync result", () => {
    render(<OfflineResultStep syncResult={null} />);
    expect(screen.getByText("尚無同步結果")).toBeInTheDocument();
  });

  it("maps the specific conflict_reason instead of the generic reason_code", () => {
    render(
      <OfflineResultStep
        syncResult={syncResult([
          result({
            checkin_id: "",
            status: "conflict",
            reason_code: "offline_conflict",
            conflict_reason: "event_not_started",
          }),
        ])}
      />,
    );

    expect(
      screen.getByText("活動尚未開始，請於活動開始時間後再驗票。"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("離線驗票同步發生衝突。"),
    ).not.toBeInTheDocument();
    // conflict status renders a localized badge, not the raw English value.
    expect(screen.queryByText("conflict")).not.toBeInTheDocument();
    expect(screen.getAllByText("衝突").length).toBeGreaterThan(0);
  });

  it("translates every offline conflict reason the backend can return", () => {
    const reasons: Record<string, RegExp> = {
      ticket_expired: /票券已逾期/,
      ticket_not_active: /票券目前不可使用/,
      invalid_scan_timestamp: /掃描時間異常/,
      invalid_ticket_token: /票券簽章碼無效/,
      ticket_not_found: /查無此票券/,
      offline_scan_event_mismatch: /不屬於此離線批次的活動/,
      ticket_token_claims_mismatch: /簽章內容與票券資料不一致/,
    };
    render(
      <OfflineResultStep
        syncResult={syncResult(
          Object.keys(reasons).map((reason, index) =>
            result({
              checkin_id: "",
              ticket_id: `tic-${index}`,
              status: "conflict",
              reason_code: "offline_conflict",
              conflict_reason: reason,
            }),
          ),
        )}
      />,
    );

    for (const pattern of Object.values(reasons)) {
      expect(screen.getByText(pattern)).toBeInTheDocument();
    }
  });

  it("spells out where and when a duplicate ticket was first redeemed", () => {
    render(
      <OfflineResultStep
        syncResult={syncResult([
          result({
            status: "duplicate",
            reason_code: "duplicate_scan",
            conflict_reason: "ticket_already_redeemed",
            duplicate: true,
            first_scanned_at: "2026-06-10T09:30:00Z",
            first_scanned_by: "staff-7",
          }),
        ])}
      />,
    );

    expect(
      screen.getByText(/已於 .+ 先完成驗票（驗票人 staff-7）/),
    ).toBeInTheDocument();
  });

  it("still explains a duplicate without first-scan metadata", () => {
    render(
      <OfflineResultStep
        syncResult={syncResult([
          result({
            status: "duplicate",
            reason_code: "duplicate_scan",
            duplicate: true,
          }),
        ])}
      />,
    );

    expect(screen.getByText(/已先在其他裝置完成驗票/)).toBeInTheDocument();
  });
});
