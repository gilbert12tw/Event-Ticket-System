import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, checkIn, reports } from "@/lib/api";
import { CheckinPage } from "./pages";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    ApiError: actual.ApiError,
    checkIn: vi.fn(),
    listAdminEvents: vi.fn(),
    offlineCheckinPackage: vi.fn(),
    reports: vi.fn(),
    syncOfflineCheckins: vi.fn(),
  };
});

const mockCheckIn = vi.mocked(checkIn);
const mockReports = vi.mocked(reports);

describe("CheckinPage", () => {
  beforeEach(() => {
    window.history.pushState({}, "", "/admin/checkin");
    localStorage.clear();
    mockCheckIn.mockClear();
    mockReports.mockClear();
  });

  it("prefills token from navigation state and ignores production localStorage tokens by default", async () => {
    mockReports.mockResolvedValue([]);
    localStorage.setItem("cets:lastTicketToken", "legacy-token");

    window.history.pushState(
      { cetsCheckinToken: "state-token" },
      "",
      "/admin/checkin",
    );
    render(<CheckinPage />);

    const tokenField = (await screen.findByLabelText(
      /掃描或貼上票券/,
    )) as HTMLInputElement;
    expect(tokenField.value).toBe("state-token");
    expect(
      screen.getByRole("button", { name: "使用最近票券" }),
    ).toHaveAttribute("disabled");
  });

  it("shows empty token input when no handoff source exists", async () => {
    window.history.pushState({}, "", "/admin/checkin");
    mockReports.mockResolvedValue([]);
    render(<CheckinPage />);

    const tokenField = (await screen.findByLabelText(
      /掃描或貼上票券/,
    )) as HTMLInputElement;
    expect(tokenField.value).toBe("");
    expect(
      screen.getByRole("button", { name: "使用最近票券" }),
    ).toHaveAttribute("disabled");
  });

  it("keeps accepted check-in visible when summary refresh is forbidden", async () => {
    mockReports.mockRejectedValue(
      new ApiError(403, {
        success: false,
        data: null,
        error: "role is not allowed",
      }),
    );
    mockCheckIn.mockResolvedValue({
      checkin_id: "chk-live",
      ticket_id: "tkt-live",
      event_id: "evt-live",
      employee_id: "E1001",
      status: "accepted",
      scanned_at: "2026-05-16T10:00:00Z",
      duplicate: false,
      holder: {
        display_name: "Ariel Chen",
        department: "Engineering",
        city: "Taipei",
      },
      family_count: 0,
    });

    render(<CheckinPage />);
    await userEvent.type(
      await screen.findByLabelText(/掃描或貼上票券/),
      "signed-token",
    );
    await userEvent.click(screen.getByRole("button", { name: "送出驗票" }));

    expect(mockCheckIn).toHaveBeenCalledWith("signed-token", "gate-1");
    expect(
      await screen.findByRole("heading", { name: "驗票成功" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("role is not allowed")).not.toBeInTheDocument();
  });
});
