import type { EventSummary, Ticket } from "@/lib/api";
import { getEligibilityDecision } from "@/lib/api/contracts";
import type { Tone } from "@/lib/ui/options";

const dayMs = 24 * 60 * 60 * 1000;
const scarceSeatThreshold = 5;

export type EmployeeEventDisplayKind =
  | "entry-ready"
  | "registered"
  | "waitlisted"
  | "bookable"
  | "waitlist-available"
  | "not-open"
  | "closed"
  | "unavailable";

export type EmployeeEventDisplayState = {
  kind: EmployeeEventDisplayKind;
  label: string;
  tone: Tone;
  primaryLabel: string;
  showInMain: boolean;
  isRegistered: boolean;
};

export type EmployeeCalendarDay = {
  date: Date;
  dateKey: string;
  weekdayLabel: string;
  dayNumber: string;
  isToday: boolean;
  eventCount: number;
  hasRegistration: boolean;
};

export function groupEventsByCalendarDay(
  events: EventSummary[],
  now: Date = new Date(),
): EmployeeCalendarDay[] {
  const today = startOfLocalDay(now);
  const firstDay = startOfWeek(today);
  return Array.from({ length: 7 }, (_, index) => {
    const date = addDays(firstDay, index);
    const dateKey = localDateKey(date);
    const relevantEvents = events.filter(
      (event) =>
        localDateKey(parseDate(event.starts_at)) === dateKey &&
        employeeEventDisplayState(event, undefined, now).showInMain,
    );
    return {
      date,
      dateKey,
      weekdayLabel: date.toLocaleDateString("zh-TW", { weekday: "short" }),
      dayNumber: String(date.getDate()),
      isToday: dateKey === localDateKey(today),
      eventCount: relevantEvents.length,
      hasRegistration: relevantEvents.some(
        (event) =>
          employeeEventDisplayState(event, undefined, now).isRegistered,
      ),
    };
  });
}

export function groupEventsByMonth(
  events: EventSummary[],
  selectedDate: Date,
  now: Date = new Date(),
): EmployeeCalendarDay[] {
  const firstOfMonth = new Date(
    selectedDate.getFullYear(),
    selectedDate.getMonth(),
    1,
  );
  const gridStart = startOfWeek(firstOfMonth);
  return Array.from({ length: 42 }, (_, index) => {
    const date = addDays(gridStart, index);
    const dateKey = localDateKey(date);
    const relevantEvents = events.filter(
      (event) =>
        localDateKey(parseDate(event.starts_at)) === dateKey &&
        employeeEventDisplayState(event, undefined, now).showInMain,
    );
    return {
      date,
      dateKey,
      weekdayLabel: date.toLocaleDateString("zh-TW", { weekday: "short" }),
      dayNumber: String(date.getDate()),
      isToday: dateKey === localDateKey(startOfLocalDay(now)),
      eventCount: relevantEvents.length,
      hasRegistration: relevantEvents.some(
        (event) =>
          employeeEventDisplayState(event, undefined, now).isRegistered,
      ),
    };
  });
}

export function selectEmployeeAgenda(
  events: EventSummary[],
  selectedDate: Date | string,
  tickets: Ticket[],
  now: Date = new Date(),
): EventSummary[] {
  const selectedKey =
    typeof selectedDate === "string"
      ? selectedDate
      : localDateKey(startOfLocalDay(selectedDate));
  return events
    .filter((event) => localDateKey(parseDate(event.starts_at)) === selectedKey)
    .filter(
      (event) =>
        employeeEventDisplayState(
          event,
          ticketForEvent(tickets, event.event_id),
          now,
        ).showInMain,
    )
    .sort(
      (left, right) =>
        agendaPriority(left, tickets, now) -
        agendaPriority(right, tickets, now),
    );
}

export function employeeEventDisplayState(
  event: EventSummary,
  ticket?: Ticket,
  now: Date = new Date(),
): EmployeeEventDisplayState {
  return (
    registeredDisplayState(event, ticket, now) ??
    unavailableDisplayState(event, now) ??
    bookableDisplayState(event)
  );
}

function registeredDisplayState(
  event: EventSummary,
  ticket: Ticket | undefined,
  now: Date,
) {
  const eventTicket = ticket ?? event.current_user_ticket ?? undefined;
  if (eventTicket?.status === "active" && eventIsCurrent(event, now)) {
    return displayState("entry-ready", "可入場", "ok", "查看票券", true, true);
  }
  if (eventTicket?.status === "active") {
    return displayState("registered", "已報名", "ok", "查看票券", true, true);
  }
  if (event.current_user_status === "confirmed") {
    return displayState("registered", "已報名", "ok", "查看詳情", true, true);
  }
  if (event.current_user_status === "waitlisted") {
    return displayState("waitlisted", "候補中", "warn", "查看詳情", true, true);
  }
  return null;
}

function unavailableDisplayState(event: EventSummary, now: Date) {
  if (event.current_user_status === "cancelled") {
    return displayState(
      "unavailable",
      "已取消",
      "neutral",
      "查看詳情",
      false,
      false,
    );
  }
  if (event.status !== "published") {
    return displayState(
      "unavailable",
      "目前不能報名",
      "neutral",
      "查看詳情",
      false,
      false,
    );
  }

  const decision = getEligibilityDecision(event);
  const cooldown = decision?.no_show_cooldown ?? event.no_show_cooldown;
  const eligible = decision ? decision.can_book : (event.eligible ?? false);
  if (cooldown?.active || !eligible) {
    return displayState(
      "unavailable",
      "目前不能報名",
      cooldown?.active ? "warn" : "fail",
      "查看詳情",
      false,
      false,
    );
  }

  if (now.getTime() < parseDate(event.registration_start).getTime()) {
    return displayState(
      "not-open",
      "尚未開放",
      "info",
      "查看詳情",
      true,
      false,
    );
  }

  if (now.getTime() > parseDate(event.registration_close).getTime()) {
    return displayState(
      "closed",
      "報名截止",
      "neutral",
      "查看詳情",
      false,
      false,
    );
  }
  return null;
}

function bookableDisplayState(event: EventSummary) {
  if (event.capacity_type === "unlimited") {
    return displayState("bookable", "可報名", "ok", "報名活動", true, false);
  }
  const remaining = event.remaining_capacity ?? 0;
  if (remaining > 0) {
    return displayState(
      "bookable",
      shouldShowCapacityHint(event) ? `剩 ${remaining} 席` : "可報名",
      "ok",
      "報名活動",
      true,
      false,
    );
  }
  return displayState(
    "waitlist-available",
    "名額已滿",
    "warn",
    "查看詳情",
    true,
    false,
  );
}

export function shouldShowCapacityHint(event: EventSummary) {
  if (event.capacity_type !== "limited") return false;
  const remaining = event.remaining_capacity ?? 0;
  return remaining > 0 && remaining <= scarceSeatThreshold;
}

export function localDateKey(date: Date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function ticketForEvent(tickets: Ticket[], eventID: string) {
  return tickets.find((ticket) => ticket.event_id === eventID);
}

function agendaPriority(event: EventSummary, tickets: Ticket[], now: Date) {
  const state = employeeEventDisplayState(
    event,
    ticketForEvent(tickets, event.event_id),
    now,
  );
  const start = parseDate(event.starts_at).getTime();
  const stateScore = {
    "entry-ready": 0,
    registered: 1,
    waitlisted: 2,
    bookable: 3,
    "waitlist-available": 4,
    "not-open": 5,
    closed: 6,
    unavailable: 7,
  } satisfies Record<EmployeeEventDisplayKind, number>;
  const currentBonus = eventIsCurrent(event, now) ? -10 : 0;
  return currentBonus + stateScore[state.kind] * dayMs + Math.max(start, 0);
}

function displayState(
  kind: EmployeeEventDisplayKind,
  label: string,
  tone: Tone,
  primaryLabel: string,
  showInMain: boolean,
  isRegistered: boolean,
): EmployeeEventDisplayState {
  return { kind, label, tone, primaryLabel, showInMain, isRegistered };
}

function eventIsCurrent(event: EventSummary, now: Date) {
  const startsAt = parseDate(event.starts_at).getTime();
  return (
    startsAt > 0 &&
    startsAt <= now.getTime() &&
    now.getTime() < startsAt + dayMs
  );
}

function startOfWeek(date: Date) {
  const day = date.getDay();
  const mondayOffset = day === 0 ? -6 : 1 - day;
  return addDays(date, mondayOffset);
}

function startOfLocalDay(date: Date) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}

function addDays(date: Date, days: number) {
  const next = new Date(date);
  next.setDate(next.getDate() + days);
  return startOfLocalDay(next);
}

function parseDate(value?: string) {
  const date = new Date(value || "");
  if (Number.isNaN(date.getTime())) return new Date(0);
  return date;
}
