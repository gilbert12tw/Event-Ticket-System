import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { EmployeeCalendarDay } from "./employee-calendar";
import {
  agendaTitle,
  EmployeeAgenda,
  EmployeeCalendarStrip,
  EmployeeEventDetailHero,
  EmployeeEventPosterCard,
} from "./employee-calendar-components";
import { eventFixture } from "@/test/event-fixtures";
import type { Ticket } from "@/lib/api";

const NOW = new Date("2026-06-10T08:00:00Z");

function dayFixture(
  overrides: Partial<EmployeeCalendarDay> = {},
): EmployeeCalendarDay {
  return {
    date: new Date("2026-06-10T00:00:00"),
    dateKey: "2026-06-10",
    weekdayLabel: "週三",
    dayNumber: "10",
    isToday: false,
    eventCount: 0,
    hasRegistration: false,
    ...overrides,
  };
}

function ticketFixture(overrides: Partial<Ticket> = {}): Ticket {
  return {
    ticket_id: "t-1",
    registration_id: "r-1",
    event_id: "evt-1",
    employee_id: "E1001",
    status: "active",
    issued_at: "2026-06-01T00:00:00Z",
    non_transferable: true,
    ...overrides,
  };
}

describe("EmployeeCalendarStrip", () => {
  it("renders week days and selects a day on click", async () => {
    const user = userEvent.setup();
    const onSelectDate = vi.fn();
    render(
      <EmployeeCalendarStrip
        days={[
          dayFixture({ isToday: true }),
          dayFixture({
            dateKey: "2026-06-11",
            dayNumber: "11",
            weekdayLabel: "週四",
            eventCount: 2,
            hasRegistration: true,
          }),
        ]}
        monthDays={[]}
        monthOpen={false}
        selectedDateKey="2026-06-10"
        onSelectDate={onSelectDate}
        onToggleMonth={vi.fn()}
      />,
    );

    expect(screen.queryByLabelText("月曆")).not.toBeInTheDocument();
    expect(screen.getByLabelText(/2 場活動，已報名/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /週四|6月11日/ }));
    expect(onSelectDate).toHaveBeenCalledWith("2026-06-11");
  });

  it("shows the month grid with muted out-of-month days when open", async () => {
    const user = userEvent.setup();
    const onToggleMonth = vi.fn();
    render(
      <EmployeeCalendarStrip
        days={[dayFixture()]}
        monthDays={[
          dayFixture(),
          dayFixture({
            dateKey: "2026-07-01",
            dayNumber: "01",
            weekdayLabel: "週三",
          }),
        ]}
        monthOpen
        selectedDateKey="2026-06-10"
        onSelectDate={vi.fn()}
        onToggleMonth={onToggleMonth}
      />,
    );

    expect(screen.getByLabelText("月曆")).toBeInTheDocument();
    expect(screen.getByLabelText(/非本月日期/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "收合月曆" }));
    expect(onToggleMonth).toHaveBeenCalled();
  });
});

describe("EmployeeAgenda", () => {
  it("renders the empty state when there are no events", () => {
    render(
      <EmployeeAgenda events={[]} now={NOW} tickets={[]} title="今日活動" />,
    );

    expect(screen.getByText("這天沒有活動")).toBeInTheDocument();
  });

  it("renders a poster card per event with its matching ticket", () => {
    render(
      <EmployeeAgenda
        events={[eventFixture({ current_user_status: "confirmed" })]}
        now={NOW}
        tickets={[ticketFixture()]}
        title="今日活動"
      />,
    );

    expect(screen.getByRole("heading", { name: "活動" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "加入行事曆：活動" }),
    ).toBeInTheDocument();
  });
});

describe("EmployeeEventPosterCard", () => {
  it("offers hide control and navigates via the primary action", async () => {
    const user = userEvent.setup();
    const onHide = vi.fn();
    render(
      <EmployeeEventPosterCard
        event={eventFixture()}
        now={NOW}
        onHide={onHide}
      />,
    );

    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/user/events/detail?event_id=evt-1");

    await user.click(screen.getByRole("button", { name: "隱藏活動：活動" }));
    expect(onHide).toHaveBeenCalledWith("evt-1");
  });

  it("marks hidden cards and offers unhide", async () => {
    const user = userEvent.setup();
    const onUnhide = vi.fn();
    render(
      <EmployeeEventPosterCard
        event={eventFixture()}
        hidden
        now={NOW}
        onUnhide={onUnhide}
      />,
    );

    expect(screen.getByText("已隱藏")).toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: "取消隱藏活動：活動" }),
    );
    expect(onUnhide).toHaveBeenCalledWith("evt-1");
  });

  it("renders no corner control without hide handlers", () => {
    render(<EmployeeEventPosterCard event={eventFixture()} now={NOW} />);

    expect(
      screen.queryByRole("button", { name: /隱藏/ }),
    ).not.toBeInTheDocument();
  });

  it("falls back to a default description when missing", () => {
    render(
      <EmployeeEventPosterCard
        event={eventFixture({ description: "" })}
        now={NOW}
      />,
    );

    expect(screen.getByText("未提供活動介紹。")).toBeInTheDocument();
  });
});

describe("EmployeeEventDetailHero", () => {
  it("renders date tile, location facts, and the provided action", () => {
    render(
      <EmployeeEventDetailHero
        action={<button type="button">立即報名</button>}
        event={eventFixture()}
        now={NOW}
      />,
    );

    expect(screen.getByLabelText("活動資訊")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "立即報名" }),
    ).toBeInTheDocument();
  });

  it("falls back when the start time cannot be parsed", () => {
    render(
      <EmployeeEventDetailHero
        event={eventFixture({ starts_at: "", ends_at: "" })}
        now={NOW}
      />,
    );

    expect(screen.getByText("時間待公布")).toBeInTheDocument();
    expect(screen.getByText("--")).toBeInTheDocument();
  });

  it("shows the unset-site label when no site is set", () => {
    render(
      <EmployeeEventDetailHero
        event={eventFixture({ location: "", event_site: "", event_city: "" })}
        now={NOW}
      />,
    );

    expect(screen.getByText("未設定")).toBeInTheDocument();
  });
});

describe("agendaTitle", () => {
  it("labels today's agenda as 今日活動", () => {
    expect(agendaTitle("2026-06-10", new Date("2026-06-10T08:00:00"))).toBe(
      "今日活動",
    );
  });

  it("labels other days with their date", () => {
    expect(agendaTitle("2026-06-12", new Date("2026-06-10T08:00:00"))).toBe(
      "06/12 活動",
    );
  });
});
