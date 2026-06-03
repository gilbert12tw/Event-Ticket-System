import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createReportExport,
  downloadReportExport,
  getReportExport,
  reports,
} from "@/lib/api";
import type { ReportExport, ReportRow } from "@/lib/api";
import { HrReportsPage } from "./pages";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    createReportExport: vi.fn(),
    downloadReportExport: vi.fn(),
    getReportExport: vi.fn(),
    reports: vi.fn(),
  };
});

const mockCreateReportExport = vi.mocked(createReportExport);
const mockDownloadReportExport = vi.mocked(downloadReportExport);
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

const reportRows: ReportRow[] = [
  {
    event_id: "evt-full",
    title: "家庭電影夜",
    capacity_type: "limited",
    capacity: 10,
    confirmed_count: 8,
    waitlist_count: 2,
    employee_count: 6,
    family_count: 2,
    total_attendee_count: 8,
    ticket_count: 8,
    checkin_count: 1,
    remaining_capacity: 2,
    city_distribution: { Taipei: 6, Hsinchu: 2, Tainan: 0 },
    starts_at: "2026-06-10T10:00:00Z",
  },
  {
    event_id: "evt-open",
    title: "全公司交流活動",
    capacity_type: "unlimited",
    capacity: null,
    confirmed_count: 2,
    waitlist_count: 0,
    employee_count: 2,
    family_count: 0,
    total_attendee_count: 2,
    ticket_count: 2,
    checkin_count: 2,
    remaining_capacity: null,
    city_distribution: {},
    starts_at: "2026-06-11T10:00:00Z",
  },
];

describe("HrReportsPage", () => {
  beforeEach(() => {
    window.history.pushState({}, "", "/admin/reports");
    mockReports.mockReset();
    mockCreateReportExport.mockReset();
    mockDownloadReportExport.mockReset();
    mockGetReportExport.mockReset();
    mockReports.mockResolvedValue([]);
    mockDownloadReportExport.mockResolvedValue(new Blob(["event_id,title\n"]));
    vi.spyOn(globalThis.URL, "createObjectURL").mockReturnValue(
      "blob:report-export",
    );
    vi.spyOn(globalThis.URL, "revokeObjectURL").mockImplementation(() => {});
  });

  it("downloads ready exports and shows pending copy instead of zero time", async () => {
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
    await waitFor(() =>
      expect(mockDownloadReportExport).toHaveBeenCalledWith("exp-pending"),
    );
    await userEvent.click(screen.getByRole("tab", { name: "匯出狀態" }));

    expect(screen.getByText("尚未完成")).toBeInTheDocument();
    expect(screen.queryByText(/0001-01-01/)).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "下載 CSV" }),
    ).toBeInTheDocument();
  });

  it("summarizes participation rows and filters by report preset", async () => {
    mockReports.mockResolvedValue(reportRows);

    render(<HrReportsPage />);

    expect((await screen.findAllByText("家庭電影夜")).length).toBeGreaterThan(
      0,
    );
    expect(screen.getByLabelText("人資報表摘要")).toHaveTextContent("已報名10");
    expect(screen.getByLabelText("人資報表摘要")).toHaveTextContent("候補2");
    expect(screen.getByLabelText("人資報表摘要")).toHaveTextContent(
      "到場率30%",
    );
    expect(
      screen.getAllByText("Taipei: 6 / Hsinchu: 2").length,
    ).toBeGreaterThan(0);
    expect(screen.getAllByText("不限量").length).toBeGreaterThan(0);

    await userEvent.click(screen.getByRole("combobox", { name: "報表模板" }));
    await userEvent.click(screen.getByRole("option", { name: "驗票例外" }));

    expect(screen.getAllByText("家庭電影夜").length).toBeGreaterThan(0);
    expect(screen.queryByText("全公司交流活動")).not.toBeInTheDocument();

    await userEvent.clear(screen.getByLabelText("篩選報表表格"));
    await userEvent.type(screen.getByLabelText("篩選報表表格"), "missing");
    expect(screen.getByText("沒有符合條件的報表")).toBeInTheDocument();
  });

  it("shows failed export status without polling external services", async () => {
    mockCreateReportExport.mockResolvedValue(pendingExport);
    mockGetReportExport.mockResolvedValue({
      ...pendingExport,
      status: "failed",
      completed_at: "2026-05-20T08:01:00Z",
      object_key: "",
    });

    render(<HrReportsPage />);
    await screen.findByRole("button", { name: /匯出完整參與報表/ });
    await userEvent.click(
      screen.getByRole("button", { name: /匯出完整參與報表/ }),
    );
    await userEvent.click(screen.getByRole("tab", { name: "匯出狀態" }));

    expect(await screen.findByText("失敗")).toBeInTheDocument();
    expect(mockDownloadReportExport).not.toHaveBeenCalled();
    expect(screen.getByText("匯出失敗，請重新產生。")).toBeInTheDocument();
    expect(screen.getByText("尚未產生")).toBeInTheDocument();
  });
});
