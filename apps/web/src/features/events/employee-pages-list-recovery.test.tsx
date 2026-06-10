import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { eventPosterBlob, listEvents, listTickets } from "@/lib/api";
import { claims, eventFixture } from "@/test/event-fixtures";
import { EmployeeEventsPage } from "./employee-pages";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    eventPosterBlob: vi.fn(),
    getEvent: vi.fn(),
    listEvents: vi.fn(),
    listTickets: vi.fn(),
  };
});

const mockListEvents = vi.mocked(listEvents);
const mockListTickets = vi.mocked(listTickets);
const mockEventPosterBlob = vi.mocked(eventPosterBlob);
const testNow = new Date("2026-06-04T10:00:00+08:00");
const testTodayKey = "2026-06-04";

type EventOverrides = Parameters<typeof eventFixture>[0];

function showEvents(...events: EventOverrides[]) {
  mockListEvents.mockResolvedValue(
    events.map((event) =>
      eventFixture({
        registration_close: isoOnDay(30, 23),
        registration_start: isoOnDay(-7, 9),
        starts_at: isoOnDay(0, 10),
        ...event,
      }),
    ),
  );
}

function isoOnDay(dayOffset: number, hour: number) {
  return new Date(Date.UTC(2026, 5, 4 + dayOffset, hour - 8)).toISOString();
}

function renderEmployeeEventsPage() {
  return render(<EmployeeEventsPage claims={claims} now={testNow} />);
}

describe("EmployeeEventsPage discovery list recovery rows", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    mockListEvents.mockReset();
    mockListTickets.mockReset();
    mockListTickets.mockResolvedValue([]);
    mockEventPosterBlob.mockReset();
    mockEventPosterBlob.mockResolvedValue(null);
    globalThis.history.replaceState(
      {},
      "",
      `/user/events?view=week&date=${testTodayKey}`,
    );
  });

  it("surfaces cancelled and registration-closed rows in list mode only", async () => {
    showEvents(
      {
        current_user_status: "cancelled",
        event_id: "evt-cancelled",
        title: "已取消活動",
      },
      {
        event_id: "evt-closed",
        registration_close: isoOnDay(-1, 18),
        title: "報名截止活動",
      },
      {
        eligibility: {
          can_book: false,
          eligible: false,
          event_id: "evt-ineligible",
          no_show_cooldown: { active: false },
          reasons: ["department does not match"],
          warnings: [],
        },
        event_id: "evt-ineligible",
        title: "不適合你的活動",
      },
    );

    renderEmployeeEventsPage();

    expect(await screen.findByText("這天沒有活動")).toBeInTheDocument();
    expect(screen.queryByText("已取消活動")).not.toBeInTheDocument();
    expect(screen.queryByText("報名截止活動")).not.toBeInTheDocument();
    expect(screen.queryByText("不適合你的活動")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "活動列表" }));

    expect(await screen.findByText("已取消活動")).toBeInTheDocument();
    expect(screen.getByText("報名截止活動")).toBeInTheDocument();
    expect(screen.getByText("已取消")).toBeInTheDocument();
    expect(screen.getByText("報名截止")).toBeInTheDocument();
    expect(screen.getByText("已截止")).toBeInTheDocument();
    expect(screen.queryByText("不適合你的活動")).not.toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "查看詳情：已取消活動" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "查看詳情：報名截止活動" }),
    ).toBeInTheDocument();
  });

  it("routes registered events with a ticket to the event detail page", async () => {
    showEvents({
      current_user_status: "confirmed",
      current_user_ticket: {
        employee_id: "E1001",
        event_id: "evt-ticketed",
        issued_at: isoOnDay(0, 9),
        registration_id: "reg-evt-ticketed",
        status: "active",
        ticket_id: "tkt-1",
      },
      event_id: "evt-ticketed",
      starts_at: isoOnDay(7, 10),
      title: "已報名活動",
    });

    renderEmployeeEventsPage();

    await userEvent.click(screen.getByRole("tab", { name: "活動列表" }));

    const link = await screen.findByRole("link", {
      name: "查看詳情：已報名活動",
    });
    expect(link).toHaveAttribute(
      "href",
      "/user/events/detail?event_id=evt-ticketed",
    );
  });
});
