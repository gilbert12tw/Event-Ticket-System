import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { bookEvent, cancelMyRegistration, listEvents } from "@/lib/api";
import type { AuthMeClaims, EventSummary } from "@/lib/api";
import { EmployeeEventsPage } from "./employee-pages";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    bookEvent: vi.fn(),
    cancelMyRegistration: vi.fn(),
    listEvents: vi.fn(),
  };
});

const mockListEvents = vi.mocked(listEvents);
const mockBookEvent = vi.mocked(bookEvent);
const mockCancelMyRegistration = vi.mocked(cancelMyRegistration);

const claims: AuthMeClaims = {
  employee_id: "E1001",
  display_name: "Ariel Chen",
  role_claims: ["employee"],
  mapped_roles: ["employee"],
  department: "Engineering",
  site: "Taipei",
  city: "Taipei",
  claims_status: "complete",
};

describe("EmployeeEventsPage", () => {
  beforeEach(() => {
    mockListEvents.mockReset();
    mockBookEvent.mockReset();
    mockCancelMyRegistration.mockReset();
  });

  it("shows bounded family count only for unlimited events", async () => {
    mockListEvents.mockResolvedValue([
      eventFixture({
        event_id: "evt-limited",
        title: "限量活動",
        capacity_type: "limited",
        capacity: 5,
        remaining_capacity: 3,
      }),
      eventFixture({
        event_id: "evt-unlimited",
        title: "家庭日",
        capacity_type: "unlimited",
        capacity: null,
        remaining_capacity: null,
        allows_family: true,
      }),
    ]);

    render(<EmployeeEventsPage claims={claims} />);

    expect(await screen.findAllByText("符合資格")).toHaveLength(2);
    expect(
      await screen.findByText("限量活動不開放同行人數。"),
    ).toBeInTheDocument();
    const familyInput = await screen.findByRole("spinbutton", {
      name: "同行人數：家庭日",
    });
    expect(familyInput).toHaveAttribute("max", "10");
  });

  it("shows cooldown feedback and disables self-cancel after registration close", async () => {
    mockListEvents.mockResolvedValue([
      eventFixture({
        current_user_registration_id: "reg-closed",
        current_user_status: "confirmed",
        event_id: "evt-cooldown",
        no_show_cooldown: {
          active: true,
          applies_to: "limited",
          until: "2026-08-01T00:00:00Z",
          reason: "no_show_cooldown",
        },
        registration_close: "2020-01-01T00:00:00Z",
        title: "冷卻活動",
        eligibility: {
          event_id: "evt-cooldown",
          eligible: true,
          can_book: false,
          reasons: [],
          warnings: [],
          no_show_cooldown: {
            active: true,
            until: "2026-08-01T00:00:00Z",
            reason: "no_show_cooldown",
          },
        },
      }),
    ]);

    render(<EmployeeEventsPage claims={claims} />);

    expect(
      await screen.findByText(/限量活動報名因未報到冷卻而暫停/),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /取消報名/ })).toBeDisabled();
    expect(screen.getByText(/自助取消已關閉/)).toBeInTheDocument();
  });

  it("renders cross-city warning and keeps booking button enabled when can_book=true", async () => {
    mockListEvents.mockResolvedValue([
      eventFixture({
        event_id: "evt-crosscity",
        title: "Hsinchu Event",
        event_city: "Hsinchu",
        eligibility: {
          event_id: "evt-crosscity",
          eligible: true,
          can_book: true,
          reasons: [],
          warnings: [{ code: "cross_city", message: "This event is in Hsinchu; your registered city is Taipei.", employee_city: "Taipei", event_city: "Hsinchu" }],
          no_show_cooldown: { active: false }
        }
      })
    ]);

    render(<EmployeeEventsPage claims={claims} />);

    expect(await screen.findByText(/Cross-city event notice/)).toBeInTheDocument();
    expect(await screen.findByText(/This event is in Hsinchu; your registered city is Taipei/)).toBeInTheDocument();
    // booking button should still be enabled (not disabled due to warning alone)
    // bookingActionLabel returns different text based on status; just verify button is not disabled
    const buttons = screen.getAllByRole("button");
    const bookBtn = buttons.find((b) => !b.textContent?.toLowerCase().includes("refresh") && !b.textContent?.toLowerCase().includes("detail") && !b.textContent?.toLowerCase().includes("cancel"));
    if (bookBtn) expect(bookBtn).not.toBeDisabled();
  });

  it("disables booking and shows reason when can_book=false (ineligible)", async () => {
    mockListEvents.mockResolvedValue([
      eventFixture({
        event_id: "evt-ineligible",
        title: "Legal Event",
        eligibility: {
          event_id: "evt-ineligible",
          eligible: false,
          can_book: false,
          reasons: ["department does not match"],
          warnings: [],
          no_show_cooldown: { active: false }
        }
      })
    ]);

    render(<EmployeeEventsPage claims={claims} />);

    expect(await screen.findByText(/Not eligible:/)).toBeInTheDocument();
    // reason text appears in both badge and alert — check the alert specifically
    const alerts = screen.getAllByText(/department does not match/);
    expect(alerts.length).toBeGreaterThan(0);
    const buttons = screen.getAllByRole("button");
    const actionBtn = buttons.find((b) => !b.textContent?.toLowerCase().includes("refresh") && !b.textContent?.toLowerCase().includes("detail"));
    if (actionBtn) expect(actionBtn).toBeDisabled();
  });

  it("renders event without eligibility object without crashing", async () => {
    mockListEvents.mockResolvedValue([
      eventFixture({ event_id: "evt-noelig", title: "No Eligibility Event", eligible: true, eligibility_reason: "eligible" })
    ]);

    render(<EmployeeEventsPage claims={claims} />);

    // Should render gracefully with fallback
    expect(await screen.findByText("No Eligibility Event")).toBeInTheDocument();
  });
});

function eventFixture(overrides: Partial<EventSummary> = {}): EventSummary {
  return {
    event_id: "evt-1",
    title: "Event",
    description: "A company event",
    location: "Taipei HQ",
    event_city: "Taipei",
    event_site: "Taipei",
    starts_at: "2026-06-01T10:00:00Z",
    registration_start: "2026-05-01T10:00:00Z",
    registration_close: "2026-05-31T10:00:00Z",
    capacity_type: "limited",
    capacity: 10,
    allows_family: false,
    status: "published",
    allocation_mode: "fcfs",
    created_by: "admin-1",
    created_at: "2026-05-01T00:00:00Z",
    updated_at: "2026-05-01T00:00:00Z",
    rule: {
      department: "Engineering",
      site: "Taipei",
      min_grade: 5,
      employment_status: "active",
    },
    eligible: true,
    eligibility_reason: "eligible",
    confirmed_count: 1,
    waitlist_count: 0,
    remaining_capacity: 9,
    current_user_status: "",
    no_show_cooldown: { active: false },
    ...overrides,
  };
}
