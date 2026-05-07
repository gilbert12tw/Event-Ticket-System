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
      duplicate: false
    };

    render(<CheckinResult result={result} />);

    expect(screen.getByRole("status")).toHaveClass("ok");
    expect(screen.getByText("驗票成功")).toBeInTheDocument();
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
      duplicate: true
    };

    render(<CheckinResult result={result} />);

    expect(screen.getByRole("status")).toHaveClass("warn");
    expect(screen.getByText("重複掃描被拒絕")).toBeInTheDocument();
    expect(screen.getByText("gate-1")).toBeInTheDocument();
  });
});
