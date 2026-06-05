import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  bookEvent,
  eventPosterBlob,
  listEvents,
  listTickets,
} from "@/lib/api";
import { claims, eventFixture } from "@/test/event-fixtures";
import { EmployeeEventsPage } from "./employee-pages";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    bookEvent: vi.fn(),
    eventPosterBlob: vi.fn(),
    getEvent: vi.fn(),
    listEvents: vi.fn(),
    listTickets: vi.fn(),
  };
});

const mockListEvents = vi.mocked(listEvents);
const mockListTickets = vi.mocked(listTickets);
const mockBookEvent = vi.mocked(bookEvent);
const mockEventPosterBlob = vi.mocked(eventPosterBlob);
const testNow = new Date("2026-06-04T10:00:00+08:00");
const testTodayKey = "2026-06-04";

type EventOverrides = Parameters<typeof eventFixture>[0];

function isoOnDay(dayOffset: number, hour: number) {
  return new Date(Date.UTC(2026, 5, 4 + dayOffset, hour - 8)).toISOString();
}

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

function renderEmployeeEventsPage() {
  return render(<EmployeeEventsPage claims={claims} now={testNow} />);
}

describe("EmployeeEventsPage discovery controls", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    mockListEvents.mockReset();
    mockListTickets.mockReset();
    mockListTickets.mockResolvedValue([]);
    mockBookEvent.mockReset();
    mockEventPosterBlob.mockReset();
    mockEventPosterBlob.mockResolvedValue(null);
    globalThis.history.replaceState(
      {},
      "",
      `/user/events?view=week&date=${testTodayKey}`,
    );
  });

  it("renders one compact discovery search strip without an advanced panel", async () => {
    showEvents({ event_id: "evt-compact", title: "午餐講座" });

    renderEmployeeEventsPage();

    expect(
      await screen.findByRole("region", { name: "活動搜尋與篩選" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("searchbox", { name: "搜尋活動" }),
    ).toHaveAttribute("placeholder", "搜尋活動、地點或標籤…");
    expect(screen.getAllByRole("searchbox")).toHaveLength(1);
    expect(
      screen.getByRole("group", { name: "活動狀態" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "地點" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "名額" })).toBeInTheDocument();
    expect(screen.getByText("本週有 1 場活動")).toBeInTheDocument();
    expect(screen.queryByText("進階搜尋")).not.toBeInTheDocument();
  });

  it("updates the URL and calendar results from lightweight search", async () => {
    showEvents(
      {
        event_id: "evt-yoga",
        tags: ["wellbeing"],
        title: "瑜伽放鬆課",
      },
      {
        event_id: "evt-lunch",
        title: "午餐講座",
      },
    );

    renderEmployeeEventsPage();

    expect((await screen.findAllByText("午餐講座")).length).toBeGreaterThan(0);

    await userEvent.type(
      screen.getByRole("searchbox", { name: "搜尋活動" }),
      "wellbeing",
    );

    await waitFor(() =>
      expect(new URLSearchParams(globalThis.location.search).get("q")).toBe(
        "wellbeing",
      ),
    );
    expect(screen.queryByText("午餐講座")).not.toBeInTheDocument();
    expect(screen.getAllByText("瑜伽放鬆課").length).toBeGreaterThan(0);
    expect(screen.getByText("本週符合 1 場")).toBeInTheDocument();
  });

  it("filters registered events with a status chip while preserving date state", async () => {
    showEvents(
      {
        event_id: "evt-bookable",
        title: "開放報名活動",
      },
      {
        current_user_registration_id: "R-registered",
        current_user_status: "confirmed",
        event_id: "evt-registered",
        title: "已報名活動",
      },
    );

    renderEmployeeEventsPage();

    expect(
      (await screen.findAllByText("開放報名活動")).length,
    ).toBeGreaterThan(0);

    await userEvent.click(screen.getByRole("button", { name: "已報名" }));

    await waitFor(() => {
      const params = new URLSearchParams(globalThis.location.search);
      expect(params.get("status")).toBe("registered");
      expect(params.get("view")).toBe("week");
      expect(params.get("date")).toBe(testTodayKey);
    });
    expect(screen.queryByText("開放報名活動")).not.toBeInTheDocument();
    expect(screen.getAllByText("已報名活動").length).toBeGreaterThan(0);
  });

  it("shows a recoverable no-result state for discovery filters", async () => {
    showEvents({
      event_id: "evt-reset",
      title: "可復原活動",
    });

    renderEmployeeEventsPage();

    expect(
      (await screen.findAllByText("可復原活動")).length,
    ).toBeGreaterThan(0);

    await userEvent.type(
      screen.getByRole("searchbox", { name: "搜尋活動" }),
      "不存在",
    );

    expect(await screen.findByText("這天沒有活動")).toBeInTheDocument();
    expect(screen.getByText("本週符合 0 場")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /清除/ }));

    await waitFor(() =>
      expect(
        new URLSearchParams(globalThis.location.search).get("q"),
      ).toBeNull(),
    );
    expect(screen.getByRole("searchbox", { name: "搜尋活動" })).toHaveValue("");
    expect(screen.getAllByText("可復原活動").length).toBeGreaterThan(0);
  });
});
