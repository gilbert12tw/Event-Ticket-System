import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createReportExport, getReportExport, reports } from "@/lib/api";
import type { ReportExport } from "@/lib/api";
import { HrReportsPage } from "./pages";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    createReportExport: vi.fn(),
    getReportExport: vi.fn(),
    reports: vi.fn(),
  };
});

const mockCreateReportExport = vi.mocked(createReportExport);
const mockGetReportExport = vi.mocked(getReportExport);
const mockReports = vi.mocked(reports);

const pendingExport: ReportExport = {
  export_id: "exp-pending",
  requested_by: "hr-1",
  report_type: "participation",
  format: "csv",
  status: "pending",
  object_key: "exports/exp-pending.csv",
  created_at: "2026-05-20T08:00:00Z",
  completed_at: "0001-01-01T00:00:00Z",
};

describe("HrReportsPage", () => {
  beforeEach(() => {
    window.history.pushState({}, "", "/admin/reports");
    mockReports.mockReset();
    mockCreateReportExport.mockReset();
    mockGetReportExport.mockReset();
    mockReports.mockResolvedValue([]);
  });

  it("shows pending copy instead of zero time for unfinished exports", async () => {
    mockCreateReportExport.mockResolvedValue(pendingExport);
    mockGetReportExport.mockResolvedValue({
      ...pendingExport,
      status: "ready",
    });

    render(<HrReportsPage />);
    await screen.findByRole("button", { name: /匯出完整參與報表/ });
    await userEvent.click(
      screen.getByRole("button", { name: /匯出完整參與報表/ }),
    );
    await waitFor(() => expect(mockGetReportExport).toHaveBeenCalled());
    await userEvent.click(screen.getByRole("tab", { name: "匯出狀態" }));

    expect(screen.getByText("尚未完成")).toBeInTheDocument();
    expect(screen.queryByText(/0001-01-01/)).not.toBeInTheDocument();
  });
});
