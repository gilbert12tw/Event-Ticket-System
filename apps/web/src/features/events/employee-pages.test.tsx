import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  type Ticket,
  bookEvent,
  eventPosterBlob,
  listEvents,
  listTickets,
} from "@/lib/api";
import {
  canBook,
  messageTone,
  registrationIDFor,
} from "./employee-event-components";
import { EmployeeEventsPage } from "./employee-pages";
import { claims, eventFixture } from "@/test/event-fixtures";

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

type EventOverrides = Parameters<typeof eventFixture>[0];

function ticket(
  ticket_id: string,
  registration_id = ticket_id.replace("T", "R"),
): Ticket {
  return {
    employee_id: "E1001",
    event_id: "evt-current",
    event_location: "Taipei HQ",
    event_starts_at: isoFromNowHours(-1),
    event_title: "現在入場活動",
    expires_at: isoFromNowHours(4),
    issued_at: isoFromNowHours(-2),
    non_transferable: true,
    registration_id,
    signed_token: "signed-secret",
    status: "active",
    ticket_id,
  };
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

function isoOnDay(dayOffset: number, hour: number) {
  const date = new Date();
  date.setHours(hour, 0, 0, 0);
  date.setDate(date.getDate() + dayOffset);
  return date.toISOString();
}

function isoFromNowHours(hourOffset: number) {
  const date = new Date();
  date.setHours(date.getHours() + hourOffset, 0, 0, 0);
  return date.toISOString();
}

describe("EmployeeEventsPage", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    mockListEvents.mockReset();
    mockListTickets.mockReset();
    mockListTickets.mockResolvedValue([]);
    mockBookEvent.mockReset();
    mockEventPosterBlob.mockReset();
    mockEventPosterBlob.mockResolvedValue(null);
    window.history.replaceState({}, "", "/user/events");
  });

  it("renders a calendar-first employee home without technical IDs", async () => {
    showEvents(
      {
        current_user_status: "cancelled",
        event_id: "evt-cancelled",
        title: "已取消活動",
      },
      {
        capacity: null,
        capacity_type: "unlimited",
        description: "一起參加公司家庭日與交流活動。",
        event_id: "evt-unlimited",
        remaining_capacity: null,
        title: "不限量活動",
      },
    );

    const { container } = render(<EmployeeEventsPage claims={claims} />);

    expect(
      await screen.findByRole("heading", { name: "活動首頁" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("週行事曆")).toBeInTheDocument();
    expect(
      within(screen.getByRole("group", { name: "日曆視圖" })).getByRole(
        "button",
        { name: "週" },
      ),
    ).toHaveAttribute("aria-pressed", "true");
    expect(
      screen.getByRole("heading", { name: "選取日期活動" }),
    ).toBeInTheDocument();
    expect((await screen.findAllByText("不限量活動")).length).toBeGreaterThan(0);
    expect(
      screen.getAllByText(/報名至|今天截止|明天截止|報名剩/).length,
    ).toBeGreaterThan(0);
    expect(screen.getByRole("link", { name: "報名活動" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "重新整理活動" }),
    ).toHaveAttribute("data-size", "icon");
    expect(screen.queryByRole("button", { name: "重新整理" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "今天" })).toHaveAttribute(
      "data-size",
      "sm",
    );
    expect(screen.getByRole("button", { name: "上一週" })).toHaveAttribute(
      "data-size",
      "icon-sm",
    );
    expect(screen.getByRole("button", { name: "下一週" })).toHaveAttribute(
      "data-size",
      "icon-sm",
    );
    expect(screen.queryByText("活動列表")).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: /可報名/ })).not.toBeInTheDocument();
    expect(screen.queryByText("資格規則")).not.toBeInTheDocument();
    expect(screen.queryByText("已取消活動")).not.toBeInTheDocument();
    expect(container).not.toHaveTextContent("evt-unlimited");
    expect(container).not.toHaveTextContent("E1001");
  });

  it("shows a clear empty state without zero-stat tabs", async () => {
    showEvents();

    render(<EmployeeEventsPage claims={claims} />);

    expect(await screen.findByText("這天沒有活動")).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: /可報名 0/ })).not.toBeInTheDocument();
    expect(screen.queryByText("我的報名 0")).not.toBeInTheDocument();
  });

  it("switches day, week, and month views while keeping the URL shareable", async () => {
    showEvents({
      event_id: "evt-planner",
      title: "午餐講座",
    });

    render(<EmployeeEventsPage claims={claims} />);

    expect(await screen.findByLabelText("週行事曆")).toBeInTheDocument();
    const modeGroup = screen.getByRole("group", { name: "日曆視圖" });

    await userEvent.click(within(modeGroup).getByRole("button", { name: "月" }));

    expect(screen.getByLabelText("月行事曆")).toBeInTheDocument();
    expect(window.location.search).toContain("view=month");
    expect(window.location.search).toContain("date=");

    await userEvent.click(within(modeGroup).getByRole("button", { name: "日" }));

    expect(screen.getByLabelText("日行程")).toBeInTheDocument();
    expect(window.location.search).toContain("view=day");
  });

  it("hides cancelled and ineligible rows from the main agenda", async () => {
    showEvents(
      {
        current_user_status: "cancelled",
        event_id: "evt-cancelled",
        title: "已取消活動",
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

    render(<EmployeeEventsPage claims={claims} />);

    expect(await screen.findByText("這天沒有活動")).toBeInTheDocument();
    expect(screen.queryByText("已取消活動")).not.toBeInTheDocument();
    expect(screen.queryByText("不適合你的活動")).not.toBeInTheDocument();
    expect(screen.queryByText(/department does not match/)).not.toBeInTheDocument();
    expect(mockBookEvent).not.toHaveBeenCalled();
  });

  it("prioritizes registered events and keeps cancellation out of the home page", async () => {
    showEvents(
      {
        event_id: "evt-bookable",
        starts_at: isoOnDay(0, 11),
        title: "開放報名活動",
      },
      {
        current_user_registration_id: "R-registered",
        current_user_status: "confirmed",
        event_id: "evt-registered",
        starts_at: isoOnDay(0, 12),
        title: "已報名活動",
      },
    );

    render(<EmployeeEventsPage claims={claims} />);

    expect(await screen.findByText("我的報名")).toBeInTheDocument();
    expect(
      screen.getAllByRole("button", { name: "加入行事曆：已報名活動" }).length,
    ).toBeGreaterThan(0);
    expect(
      screen.queryByRole("button", { name: "加入行事曆：開放報名活動" }),
    ).not.toBeInTheDocument();
    const titles = await screen.findAllByRole("heading", { level: 3 });
    expect(titles.map((title) => title.textContent)).toEqual([
      "已報名活動",
      "開放報名活動",
      "已報名活動",
    ]);
    expect(screen.queryByRole("button", { name: "取消報名" })).not.toBeInTheDocument();
  });

  it("downloads an ICS file for confirmed events", async () => {
    showEvents({
      current_user_registration_id: "R-registered",
      current_user_status: "confirmed",
      description: "團隊交流",
      event_id: "evt-registered",
      location: "Taipei HQ",
      starts_at: "2026-06-04T13:00:00+08:00",
      title: "已報名活動",
    });
    const createObjectURL = vi.fn(() => "blob:calendar");
    const revokeObjectURL = vi.fn();
    vi.stubGlobal(
      "URL",
      Object.assign(URL, {
        createObjectURL,
        revokeObjectURL,
      }),
    );
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => {});

    render(<EmployeeEventsPage claims={claims} />);

    const addButtons = await screen.findAllByRole("button", {
      name: "加入行事曆：已報名活動",
    });
    await userEvent.click(addButtons[0]);

    expect(click).toHaveBeenCalled();
    expect(createObjectURL).toHaveBeenCalledTimes(1);
    const blob = createObjectURL.mock.calls[0][0] as Blob;
    expect(blob.type).toBe("text/calendar;charset=utf-8");
    await expect(blob.text()).resolves.toContain("BEGIN:VCALENDAR");
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:calendar");
  });

  it("keeps cross-city warnings out of the main card while allowing detail handoff", async () => {
    showEvents({
      eligibility: {
        can_book: true,
        eligible: true,
        event_id: "evt-crosscity",
        no_show_cooldown: { active: false },
        reasons: [],
        warnings: [
          {
            code: "cross_city",
            employee_city: "Taipei",
            event_city: "Hsinchu",
            message: "This event is in Hsinchu; your city is Taipei.",
          },
        ],
      },
      event_city: "Hsinchu",
      event_id: "evt-crosscity",
      title: "Hsinchu Event",
    });

    render(<EmployeeEventsPage claims={claims} />);

    expect((await screen.findAllByText("Hsinchu Event")).length).toBeGreaterThan(
      0,
    );
    expect(screen.queryByText(/跨城市活動提醒/)).not.toBeInTheDocument();
    expect(
      screen.queryByText(/This event is in Hsinchu/),
    ).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("link", { name: "報名活動" }));

    expect(window.location.pathname).toBe("/user/events/detail");
    expect(window.location.search).toBe("?event_id=evt-crosscity");
  });

  it("renders poster fallback when no poster is available", async () => {
    showEvents({
      event_id: "evt-family",
      title: "家庭日",
    });

    const { container } = render(<EmployeeEventsPage claims={claims} />);

    expect((await screen.findAllByText("家庭日")).length).toBeGreaterThan(0);
    expect(
      container.querySelector(".employee-event-poster-fallback"),
    ).toHaveTextContent("家");
    expect(mockEventPosterBlob).toHaveBeenCalledWith("evt-family");
  });

  it("shows the current ticket QR without exposing compact ticket IDs", async () => {
    showEvents({
      current_user_status: "confirmed",
      event_id: "evt-current",
      starts_at: isoFromNowHours(-1),
      title: "現在入場活動",
    });
    mockListTickets.mockResolvedValue([ticket("T-current", "R-current")]);

    const { container } = render(<EmployeeEventsPage claims={claims} />);

    expect(await screen.findByText("目前活動票券")).toBeInTheDocument();
    expect(screen.getByLabelText("票券二維碼")).toBeInTheDocument();
    expect(container).not.toHaveTextContent("T-current");
    expect(container).not.toHaveTextContent("E1001");
    expect(container).not.toHaveTextContent("signed-secret");
  });

  it("renders event without eligibility object without crashing", async () => {
    showEvents({
      eligibility: undefined,
      eligible: true,
      eligibility_reason: "eligible",
      event_id: "evt-noelig",
      title: "No Eligibility Event",
    });

    render(<EmployeeEventsPage claims={claims} />);

    expect(
      (await screen.findAllByText("No Eligibility Event")).length,
    ).toBeGreaterThan(0);
  });

  it("classifies reusable event action helpers", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-05-20T00:00:00Z"));

    try {
      expect(messageTone("booking confirmed")).toBe("ok");
      expect(messageTone("報名已取消")).toBe("ok");
      expect(messageTone("waitlist closed")).toBe("warn");
      expect(messageTone("冷卻中")).toBe("warn");
      expect(messageTone("failed: not eligible")).toBe("fail");
      expect(messageTone("不符合資格")).toBe("fail");
      expect(messageTone("pending review")).toBe("info");

      expect(
        registrationIDFor(
          eventFixture({ current_user_registration_id: "R-direct" }),
        ),
      ).toBe("R-direct");
      expect(
        registrationIDFor(
          eventFixture({
            current_user_registration_id: "",
            current_user_ticket: ticket("T-ticket", "R-ticket"),
          }),
        ),
      ).toBe("R-ticket");
      expect(
        registrationIDFor(
          eventFixture({
            current_user_registration_id: "",
            current_user_ticket: undefined,
          }),
        ),
      ).toBe("");

      expect(canBook(eventFixture({ remaining_capacity: 1 }))).toBe(true);
      expect(canBook(eventFixture({ remaining_capacity: 0 }))).toBe(true);
      expect(canBook(eventFixture({ status: "draft" }))).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });
});
