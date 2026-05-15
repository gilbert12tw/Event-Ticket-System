import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { reports } from "@/lib/api";
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

const mockReports = vi.mocked(reports);

describe("CheckinPage", () => {
  beforeEach(() => {
    window.history.pushState({}, "", "/admin/checkin");
    localStorage.clear();
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

    const tokenField = (await screen.findByRole("textbox", {
      name: /signed token/i,
    })) as HTMLTextAreaElement;
    expect(tokenField.value).toBe("state-token");
    expect(
      screen.getByRole("button", { name: "使用最近票券" }),
    ).toHaveAttribute("disabled");
  });

  it("shows empty token input when no handoff source exists", async () => {
    window.history.pushState({}, "", "/admin/checkin");
    mockReports.mockResolvedValue([]);
    render(<CheckinPage />);

    const tokenField = (await screen.findByRole("textbox", {
      name: /signed token/i,
    })) as HTMLTextAreaElement;
    expect(tokenField.value).toBe("");
    expect(
      screen.getByRole("button", { name: "使用最近票券" }),
    ).toHaveAttribute("disabled");
  });
});
