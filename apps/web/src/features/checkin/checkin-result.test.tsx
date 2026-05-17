import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CheckinResult } from "./pages";
import type { CheckinResponse } from "@/lib/api";

describe("CheckinResult", () => {
  it("renders successful check-in state", () => {
    const result: CheckinResponse = {
      checkin_id: "C-1",
      ticket_id: "T-1",
      event_id: "EVT-1",
      employee_id: "E1001",
      status: "accepted",
      scanned_at: "2026-05-06T10:00:00Z",
      duplicate: false,
      holder: {
        display_name: "Ariel Chen",
        department: "Engineering",
        city: "Taipei",
      },
      family_count: 2,
    };

    render(<CheckinResult result={result} />);

    expect(screen.getByRole("status")).toHaveClass("ok");
    expect(
      screen.getByRole("heading", { name: "驗票成功" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Ariel Chen")).toBeInTheDocument();
    expect(screen.getByText("2 人")).toBeInTheDocument();
    expect(screen.getByText("T-1")).toBeInTheDocument();
  });

  it("renders duplicate scan metadata", () => {
    const result: CheckinResponse = {
      checkin_id: "C-2",
      ticket_id: "T-1",
      event_id: "EVT-1",
      employee_id: "E1001",
      status: "duplicate",
      scanned_at: "2026-05-06T10:05:00Z",
      first_scanned_at: "2026-05-06T10:00:00Z",
      first_scanned_by: "gate-1",
      duplicate: true,
      holder: {
        display_name: "Ariel Chen",
        department: "Engineering",
        city: "Taipei",
      },
      family_count: 0,
    };

    render(<CheckinResult result={result} />);

    expect(screen.getByRole("status")).toHaveClass("warn");
    expect(screen.getByText("重複掃描被拒絕")).toBeInTheDocument();
    expect(screen.getByText("裝置 gate-1")).toBeInTheDocument();
  });

  it("renders rejected check-in as failure, not success", () => {
    const result: CheckinResponse = {
      checkin_id: "",
      ticket_id: "T-bad",
      event_id: "EVT-1",
      employee_id: "E1001",
      status: "rejected",
      reason_code: "ticket_token_claims_mismatch",
      scanned_at: "2026-05-06T10:05:00Z",
      conflict_reason: "ticket_token_claims_mismatch",
      rejection_message: "ticket token claims do not match",
      duplicate: false,
      holder: {
        display_name: "Ariel Chen",
        department: "Engineering",
        city: "Taipei",
      },
      family_count: 0,
    };

    render(<CheckinResult result={result} />);

    expect(screen.getByRole("status")).toHaveClass("fail");
    expect(screen.getByText("驗票失敗，票券不可入場")).toBeInTheDocument();
    expect(screen.queryByText("驗票成功")).not.toBeInTheDocument();
    expect(
      screen.getByText("ticket_token_claims_mismatch"),
    ).toBeInTheDocument();
    expect(
      screen.getAllByText("票券簽章內容與票券資料不一致。").length,
    ).toBeGreaterThan(0);
  });
});
