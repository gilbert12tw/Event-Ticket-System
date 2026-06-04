import type { ReactNode } from "react";
import { navigate, ticketDetailPath } from "@/app/routes";
import { EmptyState, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";
import type { EventSummary, Ticket } from "@/lib/api";
import { formatDate } from "@/lib/formatting";
import { runClientNavigation } from "@/lib/navigation";
import { siteLabel } from "@/lib/ui/options";
import {
  type EmployeeCalendarDay,
  employeeEventDisplayState,
  localDateKey,
} from "./employee-calendar";
import { registrationDeadlineView } from "./employee-calendar-planner";
import { canAddToCalendar } from "./employee-calendar-export";
import { EmployeeAddToCalendarButton } from "./employee-add-to-calendar-button";
import { EventPoster } from "./employee-event-poster";
import { EmployeeRegistrationDeadlineChip } from "./employee-registration-deadline-chip";

export function EmployeeCalendarStrip({
  days,
  monthDays,
  monthOpen,
  onSelectDate,
  onToggleMonth,
  selectedDateKey,
}: Readonly<{
  days: EmployeeCalendarDay[];
  monthDays: EmployeeCalendarDay[];
  monthOpen: boolean;
  onSelectDate: (dateKey: string) => void;
  onToggleMonth: () => void;
  selectedDateKey: string;
}>) {
  return (
    <section className="employee-calendar" aria-label="活動行事曆">
      <div className="employee-calendar-header">
        <div>
          <h2>本週活動</h2>
          <p>先看哪一天有適合你的活動。</p>
        </div>
        <Button
          aria-pressed={monthOpen}
          aria-label={monthOpen ? "收合月曆" : "開啟月曆"}
          size="icon"
          type="button"
          variant="outline"
          onClick={onToggleMonth}
        >
          <Icon name="calendar" />
        </Button>
      </div>
      <div className="employee-week-strip">
        {days.map((day) => (
          <CalendarDayButton
            day={day}
            key={day.dateKey}
            selected={day.dateKey === selectedDateKey}
            onSelectDate={onSelectDate}
          />
        ))}
      </div>
      {monthOpen && (
        <div className="employee-month-grid" aria-label="月曆">
          {monthDays.map((day) => (
            <CalendarDayButton
              day={day}
              key={day.dateKey}
              month
              selected={day.dateKey === selectedDateKey}
              muted={!sameMonth(day.dateKey, selectedDateKey)}
              onSelectDate={onSelectDate}
            />
          ))}
        </div>
      )}
    </section>
  );
}

export function EmployeeAgenda({
  emptyAction = "換一天看看，或等活動主辦發布新活動。",
  emptyTitle = "這天沒有活動",
  events,
  now,
  tickets,
  title,
}: Readonly<{
  emptyAction?: string;
  emptyTitle?: string;
  events: EventSummary[];
  now: Date;
  tickets: Ticket[];
  title: string;
}>) {
  return (
    <section className="employee-agenda" aria-label={title}>
      <div className="employee-section-title">
        <h2>{title}</h2>
      </div>
      {events.length === 0 ? (
        <EmptyState title={emptyTitle} action={emptyAction} />
      ) : (
        <div className="employee-event-card-list">
          {events.map((event) => (
            <EmployeeEventPosterCard
              event={event}
              key={event.event_id}
              now={now}
              ticket={ticketForEvent(tickets, event.event_id)}
            />
          ))}
        </div>
      )}
    </section>
  );
}

export function EmployeeEventPosterCard({
  event,
  now,
  ticket,
}: Readonly<{
  event: EventSummary;
  now: Date;
  ticket?: Ticket;
}>) {
  const state = employeeEventDisplayState(event, ticket, now);
  const deadline = registrationDeadlineView(event, now);
  const href = primaryHref(event, ticket, state.kind);
  return (
    <article className="employee-event-card">
      <EventPoster eventID={event.event_id} title={event.title} />
      <div className="employee-event-card-body">
        <div className="employee-event-card-main">
          <div className="employee-event-card-badges">
            <StatusBadge tone={state.tone}>{state.label}</StatusBadge>
            <EmployeeRegistrationDeadlineChip deadline={deadline} />
            {canAddToCalendar(event, ticket) && (
              <EmployeeAddToCalendarButton event={event} />
            )}
          </div>
          <h3>{event.title}</h3>
          <p className="employee-event-meta">
            {formatDate(event.starts_at)} ·{" "}
            {siteLabel(event.location || event.event_site)}
          </p>
          <p className="employee-event-description">
            {eventDescription(event)}
          </p>
        </div>
        <Button asChild className="employee-event-primary-action">
          <a
            href={href}
            onClick={(clickEvent) =>
              runClientNavigation(clickEvent, () => navigate(href))
            }
          >
            {state.primaryLabel}
          </a>
        </Button>
      </div>
    </article>
  );
}

export function EmployeeEventDetailHero({
  action,
  event,
  now,
}: Readonly<{
  action?: ReactNode;
  event: EventSummary;
  now: Date;
}>) {
  const state = employeeEventDisplayState(event, undefined, now);
  const deadline = registrationDeadlineView(event, now);
  return (
    <section className="employee-event-detail-hero">
      <EventPoster
        eventID={event.event_id}
        title={event.title}
        variant="hero"
      />
      <div className="employee-event-detail-copy">
        <div className="employee-event-card-badges">
          <StatusBadge tone={state.tone}>{state.label}</StatusBadge>
          <EmployeeRegistrationDeadlineChip deadline={deadline} />
        </div>
        <h2>{event.title}</h2>
        <p className="employee-event-meta">
          {formatDate(event.starts_at)} ·{" "}
          {siteLabel(event.location || event.event_site)}
        </p>
        <p>{eventDescription(event)}</p>
        {action}
      </div>
    </section>
  );
}

function CalendarDayButton({
  day,
  month = false,
  muted = false,
  onSelectDate,
  selected,
}: Readonly<{
  day: EmployeeCalendarDay;
  month?: boolean;
  muted?: boolean;
  onSelectDate: (dateKey: string) => void;
  selected: boolean;
}>) {
  const className = [
    month ? "employee-calendar-day month" : "employee-calendar-day",
    selected ? "selected" : "",
    day.isToday ? "today" : "",
    muted ? "muted" : "",
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <button
      aria-pressed={selected}
      className={className}
      type="button"
      onClick={() => onSelectDate(day.dateKey)}
    >
      <span>{day.weekdayLabel}</span>
      <strong>{day.dayNumber}</strong>
      <small>
        {day.eventCount > 0 ? `${day.eventCount} 場` : "無活動"}
        {day.hasRegistration ? " · 已報名" : ""}
      </small>
    </button>
  );
}

function primaryHref(
  event: EventSummary,
  ticket: Ticket | undefined,
  stateKind: string,
) {
  if ((stateKind === "entry-ready" || stateKind === "registered") && ticket) {
    return ticketDetailPath(ticket.ticket_id);
  }
  return `/user/events/detail?event_id=${encodeURIComponent(event.event_id)}`;
}

function ticketForEvent(tickets: Ticket[], eventID: string) {
  return tickets.find((ticket) => ticket.event_id === eventID);
}

function eventDescription(event: EventSummary) {
  return (event.description || "未提供活動介紹。").trim();
}

function sameMonth(leftDateKey: string, rightDateKey: string) {
  return leftDateKey.slice(0, 7) === rightDateKey.slice(0, 7);
}

export function agendaTitle(dateKey: string, now: Date) {
  if (dateKey === localDateKey(now)) return "今日活動";
  const date = new Date(`${dateKey}T00:00:00`);
  return `${date.toLocaleDateString("zh-TW", {
    month: "2-digit",
    day: "2-digit",
  })} 活動`;
}
