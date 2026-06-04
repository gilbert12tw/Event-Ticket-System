import type { EventSummary, Ticket } from "@/lib/api";
import type { Tone } from "@/lib/ui/options";
import {
  addEmployeeCalendarDays,
  employeeCalendarDayNumber,
  employeeCalendarMonthKey,
  employeeCalendarMonthTitle,
  employeeCalendarWeekdayLabel,
  formatEmployeeCalendarClock,
  formatEmployeeCalendarMonthDay,
  localDateKey,
  parseEmployeeCalendarDateKey,
  startOfEmployeeCalendarDay,
  startOfEmployeeCalendarMonth,
  startOfEmployeeCalendarWeek,
} from "./employee-calendar-date";
import {
  type EmployeeCalendarDay,
  type EmployeeEventDisplayState,
  employeeEventDisplayState,
} from "./employee-calendar";

export type EmployeeCalendarViewMode = "day" | "week" | "month";

export type EmployeeCalendarRange = {
  view: EmployeeCalendarViewMode;
  anchorDate: Date;
  anchorDateKey: string;
  startDate: Date;
  endDate: Date;
  label: string;
  days: EmployeeCalendarDay[];
};

export type EmployeeRegistrationDeadlineKind =
  | "pending"
  | "today"
  | "tomorrow"
  | "soon"
  | "open"
  | "closed";

export type EmployeeRegistrationDeadlineView = {
  kind: EmployeeRegistrationDeadlineKind;
  label: string;
  detail: string;
  tone: Tone;
};

export type EmployeeCalendarEvent = {
  event: EventSummary;
  ticket?: Ticket;
  state: EmployeeEventDisplayState;
  deadline: EmployeeRegistrationDeadlineView;
};

export type EmployeeCalendarDayGroup = EmployeeCalendarDay & {
  events: EmployeeCalendarEvent[];
  isOutsideMonth: boolean;
};

const dayMs = 24 * 60 * 60 * 1000;
const viewModes = new Set<EmployeeCalendarViewMode>(["day", "week", "month"]);

export function calendarRangeForView(
  view: EmployeeCalendarViewMode,
  anchorDate: Date,
  now: Date = new Date(),
): EmployeeCalendarRange {
  const anchor = startOfEmployeeCalendarDay(anchorDate);
  const startDate =
    view === "month"
      ? startOfEmployeeCalendarWeek(startOfEmployeeCalendarMonth(anchor))
      : rangeStart(view, anchor);
  const dayCount = calendarDayCount(view);
  const days = Array.from({ length: dayCount }, (_, index) => {
    const date = addEmployeeCalendarDays(startDate, index);
    return calendarDayForDate(date, now);
  });
  return {
    view,
    anchorDate: anchor,
    anchorDateKey: localDateKey(anchor),
    startDate,
    endDate: addEmployeeCalendarDays(startDate, dayCount),
    label: rangeLabel(view, anchor, startDate, dayCount),
    days,
  };
}

function calendarDayCount(view: EmployeeCalendarViewMode) {
  if (view === "month") return 42;
  if (view === "week") return 7;
  return 1;
}

export function parseEmployeeCalendarQuery(
  searchParams: URLSearchParams | string,
  now: Date = new Date(),
): { view: EmployeeCalendarViewMode; date: Date; dateKey: string } {
  const params =
    typeof searchParams === "string"
      ? new URLSearchParams(searchParams)
      : searchParams;
  const rawView = params.get("view") || "";
  const view = viewModes.has(rawView as EmployeeCalendarViewMode)
    ? (rawView as EmployeeCalendarViewMode)
    : "week";
  const parsedDate = parseDateKey(params.get("date") || "", now);
  return {
    view,
    date: parsedDate,
    dateKey: localDateKey(parsedDate),
  };
}

export function registrationDeadlineView(
  event: EventSummary,
  now: Date = new Date(),
): EmployeeRegistrationDeadlineView {
  const today = startOfEmployeeCalendarDay(now);
  const registrationStart = parseDate(event.registration_start);
  const registrationClose = parseDate(event.registration_close);
  if (now.getTime() < registrationStart.getTime()) {
    return {
      kind: "pending",
      label: "尚未開放",
      detail: `${formatMonthDay(registrationStart)} 開放`,
      tone: "info",
    };
  }
  if (now.getTime() > registrationClose.getTime()) {
    return {
      kind: "closed",
      label: "已截止",
      detail: "報名已結束",
      tone: "neutral",
    };
  }
  const closeDay = startOfEmployeeCalendarDay(registrationClose);
  const daysLeft = Math.round((closeDay.getTime() - today.getTime()) / dayMs);
  if (daysLeft <= 0) {
    return {
      kind: "today",
      label: "今天截止",
      detail: `報名至 ${formatClock(registrationClose)}`,
      tone: "warn",
    };
  }
  if (daysLeft === 1) {
    return {
      kind: "tomorrow",
      label: "明天截止",
      detail: `報名至 ${formatMonthDay(registrationClose)}`,
      tone: "warn",
    };
  }
  if (daysLeft <= 7) {
    return {
      kind: "soon",
      label: `報名剩 ${daysLeft} 天`,
      detail: `報名至 ${formatMonthDay(registrationClose)}`,
      tone: daysLeft <= 3 ? "warn" : "info",
    };
  }
  return {
    kind: "open",
    label: `報名至 ${formatMonthDay(registrationClose)}`,
    detail: "開放報名中",
    tone: "info",
  };
}

export function selectEmployeeCalendarEvents(
  events: EventSummary[],
  tickets: Ticket[],
  range: EmployeeCalendarRange,
  now: Date = new Date(),
): EmployeeCalendarEvent[] {
  return events
    .map((event) => {
      const ticket = ticketForEvent(tickets, event.event_id);
      return {
        event,
        ticket,
        state: employeeEventDisplayState(event, ticket, now),
        deadline: registrationDeadlineView(event, now),
      };
    })
    .filter(({ event, state }) => {
      const startsAt = parseDate(event.starts_at);
      return (
        state.showInMain &&
        startsAt.getTime() >= range.startDate.getTime() &&
        startsAt.getTime() < range.endDate.getTime()
      );
    })
    .sort(calendarEventSort);
}

export function groupEmployeeCalendarEventsByDay(
  events: EmployeeCalendarEvent[],
  range: EmployeeCalendarRange,
): EmployeeCalendarDayGroup[] {
  return range.days.map((day) => {
    const dayEvents = events.filter(
      ({ event }) => localDateKey(parseDate(event.starts_at)) === day.dateKey,
    );
    return {
      ...day,
      eventCount: dayEvents.length,
      hasRegistration: dayEvents.some(({ state }) => state.isRegistered),
      events: dayEvents,
      isOutsideMonth:
        range.view === "month" &&
        employeeCalendarMonthKey(day.date) !==
          employeeCalendarMonthKey(range.anchorDate),
    };
  });
}

export function shiftCalendarDate(
  view: EmployeeCalendarViewMode,
  anchorDate: Date,
  direction: -1 | 1,
) {
  if (view === "month") {
    return addEmployeeCalendarMonths(
      startOfEmployeeCalendarMonth(anchorDate),
      direction,
    );
  }
  return addEmployeeCalendarDays(
    anchorDate,
    view === "week" ? direction * 7 : direction,
  );
}

export function employeeCalendarPath(
  view: EmployeeCalendarViewMode,
  dateKey: string,
) {
  const params = new URLSearchParams();
  params.set("view", view);
  params.set("date", dateKey);
  return `/user/events?${params.toString()}`;
}

function rangeStart(view: EmployeeCalendarViewMode, anchor: Date) {
  return view === "week" ? startOfEmployeeCalendarWeek(anchor) : anchor;
}

function calendarDayForDate(date: Date, now: Date): EmployeeCalendarDay {
  return {
    date,
    dateKey: localDateKey(date),
    weekdayLabel: employeeCalendarWeekdayLabel(date),
    dayNumber: employeeCalendarDayNumber(date),
    isToday: localDateKey(date) === localDateKey(now),
    eventCount: 0,
    hasRegistration: false,
  };
}

function rangeLabel(
  view: EmployeeCalendarViewMode,
  anchorDate: Date,
  startDate: Date,
  dayCount: number,
) {
  if (view === "month") {
    return employeeCalendarMonthTitle(anchorDate);
  }
  if (view === "day") return formatMonthDay(startDate);
  const endDate = addEmployeeCalendarDays(startDate, dayCount - 1);
  return `${formatMonthDay(startDate)} - ${formatMonthDay(endDate)}`;
}

function calendarEventSort(
  left: EmployeeCalendarEvent,
  right: EmployeeCalendarEvent,
) {
  const stateScore = (event: EmployeeCalendarEvent) => {
    if (event.state.kind === "entry-ready") return 0;
    if (event.state.isRegistered) return 1;
    if (event.state.kind === "bookable") return 2;
    if (event.state.kind === "waitlist-available") return 3;
    if (event.state.kind === "not-open") return 4;
    return 5;
  };
  const scoreDelta = stateScore(left) - stateScore(right);
  if (scoreDelta !== 0) return scoreDelta;
  return (
    parseDate(left.event.starts_at).getTime() -
    parseDate(right.event.starts_at).getTime()
  );
}

function ticketForEvent(tickets: Ticket[], eventID: string) {
  return tickets.find((ticket) => ticket.event_id === eventID);
}

function parseDateKey(value: string, fallback: Date) {
  return parseEmployeeCalendarDateKey(value, fallback);
}

function parseDate(value?: string) {
  const date = new Date(value || "");
  if (Number.isNaN(date.getTime())) return new Date(0);
  return date;
}

function formatMonthDay(date: Date) {
  return formatEmployeeCalendarMonthDay(date);
}

function formatClock(date: Date) {
  return formatEmployeeCalendarClock(date);
}

function addEmployeeCalendarMonths(date: Date, months: number) {
  const shifted = new Date(date);
  shifted.setUTCMonth(shifted.getUTCMonth() + months);
  return startOfEmployeeCalendarMonth(shifted);
}
