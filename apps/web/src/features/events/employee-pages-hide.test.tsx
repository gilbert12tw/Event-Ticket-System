import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  eventPosterBlob,
  hideEvent,
  listEvents,
  listTickets,
  unhideEvent,
} from "@/lib/api";
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
    hideEvent: vi.fn(),
    unhideEvent: vi.fn(),
  };
});

const mockListEvents = vi.mocked(listEvents);
const mockListTickets = vi.mocked(listTickets);
const mockEventPosterBlob = vi.mocked(eventPosterBlob);
const mockHideEvent = vi.mocked(hideEvent);
const mockUnhideEvent = vi.mocked(unhideEvent);
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

describe("EmployeeEventsPage hide event", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    mockListEvents.mockReset();
    mockListTickets.mockReset();
    mockListTickets.mockResolvedValue([]);
    mockEventPosterBlob.mockReset();
    mockEventPosterBlob.mockResolvedValue(null);
    mockHideEvent.mockReset();
    mockHideEvent.mockResolvedValue(undefined);
    mockUnhideEvent.mockReset();
    mockUnhideEvent.mockResolvedValue(undefined);
    globalThis.history.replaceState(
      {},
      "",
      `/user/events?view=week&date=${testTodayKey}`,
    );
  });

  it("hides an event from the calendar but keeps it flagged in the list", async () => {
    showEvents({ event_id: "evt-planner", title: "午餐講座" });

    renderEmployeeEventsPage();

    const agenda = await screen.findByRole("region", { name: "選取日期活動" });
    await userEvent.click(
      within(agenda).getByRole("button", { name: "隱藏活動：午餐講座" }),
    );

    expect(mockHideEvent).toHaveBeenCalledWith("evt-planner");
    expect(within(agenda).queryByText("午餐講座")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "活動列表" }));

    const listPanel = await screen.findByRole("tabpanel", { name: "活動列表" });
    expect(within(listPanel).getByText("午餐講座")).toBeInTheDocument();
    expect(within(listPanel).getByText("已隱藏")).toBeInTheDocument();

    await userEvent.click(
      within(listPanel).getByRole("button", {
        name: "取消隱藏活動：午餐講座",
      }),
    );

    expect(mockUnhideEvent).toHaveBeenCalledWith("evt-planner");
    expect(within(listPanel).queryByText("已隱藏")).not.toBeInTheDocument();
  });

  it("hides an event from the week calendar grid block", async () => {
    showEvents({ event_id: "evt-planner", title: "午餐講座" });

    renderEmployeeEventsPage();

    const weekGrid = await screen.findByLabelText("週行事曆");
    await userEvent.click(
      within(weekGrid).getByRole("button", { name: "隱藏活動：午餐講座" }),
    );

    expect(mockHideEvent).toHaveBeenCalledWith("evt-planner");
    expect(within(weekGrid).queryByText("午餐講座")).not.toBeInTheDocument();
  });

  it("hides an event from a month calendar event row", async () => {
    showEvents({ event_id: "evt-planner", title: "午餐講座" });
    globalThis.history.replaceState(
      {},
      "",
      `/user/events?view=month&date=${testTodayKey}`,
    );

    renderEmployeeEventsPage();

    const monthGrid = await screen.findByLabelText("月行事曆");
    await userEvent.click(
      within(monthGrid).getByRole("button", { name: "隱藏活動：午餐講座" }),
    );

    expect(mockHideEvent).toHaveBeenCalledWith("evt-planner");
    expect(within(monthGrid).queryByText("午餐講座")).not.toBeInTheDocument();
  });
});
