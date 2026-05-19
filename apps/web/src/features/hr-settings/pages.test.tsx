import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { HrSyncSettingsPage } from "./pages";
import {
  listEligibilityImpactReviews,
  resolveEligibilityImpactReview,
} from "@/lib/api";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    listEligibilityImpactReviews: vi.fn(),
    resolveEligibilityImpactReview: vi.fn(),
  };
});

describe("HrSyncSettingsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("loads and renders impact reviews", async () => {
    listEligibilityImpactReviews.mockResolvedValue([
      {
        review_id: "rev-1",
        event_id: "evt-1",
        employee_id: "E1001",
        ticket_id: "tk-1",
        status: "open",
        reason: "員工離職",
        created_at: "2026-05-06T10:00:00Z",
      },
    ]);

    render(<HrSyncSettingsPage />);

    await waitFor(() =>
      expect(screen.getAllByText("rev-1")[0]).toBeInTheDocument(),
    );
    const row = screen.getAllByText("rev-1")[0].closest("tr");
    expect(row).not.toBeNull();
    expect(
      within(row as HTMLTableRowElement).getByText("員工離職"),
    ).toBeInTheDocument();
    expect(
      within(row as HTMLTableRowElement).getByText("待處理"),
    ).toBeInTheDocument();
  });

  it("resolves an impact review with operator reason", async () => {
    listEligibilityImpactReviews.mockResolvedValueOnce([
      {
        review_id: "rev-2",
        event_id: "evt-2",
        employee_id: "E1002",
        ticket_id: "tk-2",
        status: "open",
        reason: "資格不一致",
        created_at: "2026-05-06T10:02:00Z",
      },
    ]);
    resolveEligibilityImpactReview.mockResolvedValue({
      review_id: "rev-2",
      event_id: "evt-2",
      employee_id: "E1002",
      ticket_id: "tk-2",
      status: "resolved",
      reason: "人資已確認",
      created_at: "2026-05-06T10:02:00Z",
      resolved_at: "2026-05-06T10:03:00Z",
    });

    render(<HrSyncSettingsPage />);

    const templateSelect = await screen.findByRole("combobox", {
      name: "處置模板",
    });
    await userEvent.click(templateSelect);
    await userEvent.click(
      await screen.findByRole("option", { name: "自訂處置原因" }),
    );
    const reasonInput = await screen.findByRole("textbox", {
      name: "自訂處置原因",
    });
    await userEvent.clear(reasonInput);
    await userEvent.type(reasonInput, "人資已確認");
    await userEvent.click(screen.getByRole("button", { name: "標記已處理" }));

    await waitFor(() =>
      expect(resolveEligibilityImpactReview).toHaveBeenCalledWith("rev-2", {
        reason: "人資已確認",
      }),
    );
    expect(screen.getByText("已處理影響項目 rev-2。")).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.queryByText("目前沒有影響項目")).toBeInTheDocument(),
    );
    expect(screen.queryByText("rev-2")).not.toBeInTheDocument();
  });
});
