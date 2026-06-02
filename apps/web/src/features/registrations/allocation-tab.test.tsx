import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { runLottery } from "@/lib/api";
import type { EventSummary } from "@/lib/api";
import { AllocationTab } from "./allocation-tab";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    runLottery: vi.fn(),
  };
});

describe("AllocationTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("explains that FCFS events do not run lottery allocation", () => {
    render(
      <AllocationTab
        busy={false}
        confirmedCount={1}
        event={eventFixture({ allocation_mode: "first_come_first_served" })}
        waitlistCount={0}
        onAllocated={vi.fn()}
      />,
    );

    expect(screen.getByText("FCFS")).toBeInTheDocument();
    expect(screen.getByText(/Lottery 不適用/)).toBeInTheDocument();
  });

  it("requires a seed and confirmation before running lottery", async () => {
    const onAllocated = vi.fn();
    vi.mocked(runLottery).mockResolvedValue({
      run_id: "lot-1",
      event_id: "evt-1",
      seed: "seed-1",
      status: "completed",
      input_snapshot_at: "2026-05-06T10:00:00Z",
      algorithm_version: "deterministic-sha256-v1",
      candidate_count: 3,
      eligibility_rule_id: "rule-1",
      eligibility_rule_version: 1,
      eligibility_snapshot: {
        department: "*",
        site: "*",
        min_grade: 0,
        employment_status: "active",
      },
      winner_count: 2,
      created_by: "admin-1",
      created_at: "2026-05-06T10:00:00Z",
    });

    render(
      <AllocationTab
        busy={false}
        confirmedCount={2}
        event={eventFixture({ allocation_mode: "lottery" })}
        waitlistCount={3}
        onAllocated={onAllocated}
      />,
    );

    expect(screen.getByRole("button", { name: "執行抽籤" })).toBeDisabled();
    await userEvent.type(screen.getByLabelText(/抽籤 seed/), "seed-1");
    await userEvent.click(screen.getByRole("button", { name: "執行抽籤" }));
    await userEvent.click(screen.getByRole("button", { name: "確認抽籤" }));

    await waitFor(() =>
      expect(runLottery).toHaveBeenCalledWith("evt-1", { seed: "seed-1" }),
    );
    expect(onAllocated).toHaveBeenCalled();
    expect(await screen.findByText("lot-1")).toBeInTheDocument();
  });
});

function eventFixture(overrides: Partial<EventSummary> = {}): EventSummary {
  return {
    event_id: "evt-1",
    title: "活動",
    description: "公司活動",
    location: "台北總部禮堂",
    starts_at: "2026-06-01T10:00:00Z",
    registration_start: "2026-05-01T10:00:00Z",
    registration_close: "2026-05-31T10:00:00Z",
    capacity_type: "limited",
    capacity: 10,
    allows_family: false,
    status: "published",
    allocation_mode: "lottery",
    created_by: "admin-1",
    created_at: "2026-05-01T00:00:00Z",
    updated_at: "2026-05-01T00:00:00Z",
    rule: {
      department: "Engineering",
      site: "Taipei",
      min_grade: 5,
      employment_status: "active",
    },
    confirmed_count: 1,
    waitlist_count: 2,
    remaining_capacity: 2,
    current_user_status: "",
    ...overrides,
  };
}
