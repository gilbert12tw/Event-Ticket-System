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
        reason: "employee departed",
        created_at: "2026-05-06T10:00:00Z",
      },
    ]);

    render(<HrSyncSettingsPage />);

    await waitFor(() => expect(screen.getByText("rev-1")).toBeInTheDocument());
    expect(screen.getByText("employee departed")).toBeInTheDocument();
    const row = screen.getByText("rev-1").closest("tr");
    expect(row).not.toBeNull();
    expect(
      within(row as HTMLTableRowElement).getByText("open"),
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
        reason: "eligibility mismatch",
        created_at: "2026-05-06T10:02:00Z",
      },
    ]);
    resolveEligibilityImpactReview.mockResolvedValue({
      review_id: "rev-2",
      event_id: "evt-2",
      employee_id: "E1002",
      ticket_id: "tk-2",
      status: "resolved",
      reason: "approved by HR",
      created_at: "2026-05-06T10:02:00Z",
      resolved_at: "2026-05-06T10:03:00Z",
    });

    render(<HrSyncSettingsPage />);

    const reasonInput = await screen.findByRole("textbox", {
      name: /resolve reason/i,
    });
    await userEvent.clear(reasonInput);
    await userEvent.type(reasonInput, "approved by HR");
    await userEvent.click(screen.getByRole("button", { name: "Resolve" }));

    await waitFor(() =>
      expect(resolveEligibilityImpactReview).toHaveBeenCalledWith("rev-2", {
        reason: "approved by HR",
      }),
    );
    expect(screen.getByText("已解析 review rev-2。")).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.queryByText("目前沒有影響項目")).toBeInTheDocument(),
    );
    expect(screen.queryByText("rev-2")).not.toBeInTheDocument();
  });
});
