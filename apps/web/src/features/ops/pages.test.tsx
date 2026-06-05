import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { getOpsDashboard } from "@/lib/api";
import { OpsControlPlanePage } from "./pages";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    getOpsDashboard: vi.fn(),
  };
});

const mockGetOpsDashboard = vi.mocked(getOpsDashboard);

describe("OpsControlPlanePage", () => {
  beforeEach(() => {
    mockGetOpsDashboard.mockReset();
  });

  it("renders backend ops snapshots", async () => {
    mockGetOpsDashboard.mockResolvedValue({
      capacity_pressure: {
        events: [
          {
            event_id: "evt_hot",
            capacity_type: "limited",
            confirmed_count: 27,
            waitlist_count: 4,
            received_count: 0,
            remaining_capacity: 3,
            reservation_count: 9,
            reservation_state: "available",
            rejected_per_min: null,
            rate_limit_drop_per_min: 0,
            idempotency_replay_per_min: 2,
          },
        ],
      },
      queues: {
        queues: [
          {
            name: "notification",
            pending: 4,
            in_flight: 1,
            dead_letter: 2,
            p95_age_seconds: 75,
            last_processed_at: "2026-05-31T08:00:00Z",
          },
        ],
      },
      reports_freshness: {
        projections: [
          {
            name: "event_summary",
            last_applied: "2026-05-31T08:00:00Z",
            lag_seconds: 12,
            degraded: false,
          },
        ],
      },
      dead_letter_recent: [
        {
          delivery_id: "del_1",
          worker_kind: "notification",
          event_type: "notification.requested.v2",
          status: "dead_letter",
          retry_count: 4,
          last_error: "smtp timeout",
          created_at: "2026-05-31T08:00:00Z",
          retry_eligible: true,
          dead_letter_eligible: true,
        },
      ],
      replay_recent: [
        {
          audit_id: "aud_replay_1",
          actor_role: "hr_admin",
          kind: "notification",
          dry_run: false,
          affected_count: 2,
          enqueued_count: 2,
          created_at: "2026-05-31T08:05:00Z",
        },
      ],
    });

    render(<OpsControlPlanePage />);

    await waitFor(() => expect(mockGetOpsDashboard).toHaveBeenCalled());
    expect(
      screen.getByRole("heading", { name: "營運監控" }),
    ).toBeInTheDocument();
    expect(screen.getAllByText("evt_hot").length).toBeGreaterThan(0);
    expect(screen.getAllByText("event_summary").length).toBeGreaterThan(0);
    expect(screen.getAllByText("del_1").length).toBeGreaterThan(0);
    expect(screen.getByText("smtp timeout")).toBeInTheDocument();
    expect(screen.getByText("aud_replay_1")).toBeInTheDocument();
    expect(screen.getByText("Applied")).toBeInTheDocument();
  });
});
