import { navigate, ticketDetailPath } from "@/app/routes";
import { EmptyState, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";
import { runClientNavigation } from "@/lib/navigation";
import { siteLabel } from "@/lib/ui/options";
import {
  type EmployeeCalendarDayGroup,
  type EmployeeCalendarEvent,
  type EmployeeCalendarRange,
  type EmployeeCalendarViewMode,
} from "./employee-calendar-planner";
import { EmployeeEventPosterCard } from "./employee-calendar-components";
import { EmployeeRegistrationDeadlineChip } from "./employee-registration-deadline-chip";

type CalendarViewProps = {
  groups: EmployeeCalendarDayGroup[];
  now: Date;
  onSelectDate: (dateKey: string) => void;
  onViewChange: (view: EmployeeCalendarViewMode) => void;
  onMove: (direction: -1 | 1) => void;
  onToday: () => void;
  range: EmployeeCalendarRange;
  selectedDateKey: string;
  selectedEvents: EmployeeCalendarEvent[];
};

const viewLabels = {
  day: "日",
  week: "週",
  month: "月",
} satisfies Record<EmployeeCalendarViewMode, string>;

export function EmployeeCalendarToolbar({
  onMove,
  onToday,
  onViewChange,
  range,
}: Readonly<Pick<CalendarViewProps, "onMove" | "onToday" | "onViewChange" | "range">>) {
  return (
    <div className="employee-calendar-toolbar" aria-label="活動日曆工具列">
      <div className="employee-calendar-nav">
        <Button size="sm" type="button" variant="outline" onClick={onToday}>
          今天
        </Button>
        <Button
          aria-label={`上一${viewLabels[range.view]}`}
          size="icon-sm"
          type="button"
          variant="outline"
          onClick={() => onMove(-1)}
        >
          <Icon name="chevronLeft" />
        </Button>
        <Button
          aria-label={`下一${viewLabels[range.view]}`}
          size="icon-sm"
          type="button"
          variant="outline"
          onClick={() => onMove(1)}
        >
          <Icon name="chevronRight" />
        </Button>
      </div>
      <div className="employee-calendar-title">
        <h2>{range.label}</h2>
      </div>
      <div
        className="employee-calendar-mode"
        role="group"
        aria-label="日曆視圖"
      >
        {(Object.keys(viewLabels) as EmployeeCalendarViewMode[]).map((view) => (
          <Button
            aria-pressed={range.view === view}
            key={view}
            size="sm"
            type="button"
            variant={range.view === view ? "default" : "outline"}
            onClick={() => onViewChange(view)}
          >
            {viewLabels[view]}
          </Button>
        ))}
      </div>
    </div>
  );
}

export function EmployeeCalendarView({
  groups,
  now,
  onMove,
  onSelectDate,
  onToday,
  onViewChange,
  range,
  selectedDateKey,
  selectedEvents,
}: Readonly<CalendarViewProps>) {
  return (
    <section className="employee-calendar" aria-label="活動行事曆">
      <EmployeeCalendarToolbar
        range={range}
        onMove={onMove}
        onToday={onToday}
        onViewChange={onViewChange}
      />
      {range.view === "day" && (
        <EmployeeDayAgenda events={selectedEvents} now={now} />
      )}
      {range.view === "week" && (
        <>
          <EmployeeWeekCalendar
            groups={groups}
            onSelectDate={onSelectDate}
            selectedDateKey={selectedDateKey}
          />
          <EmployeeSelectedDayAgenda events={selectedEvents} now={now} />
        </>
      )}
      {range.view === "month" && (
        <>
          <EmployeeMonthCalendar
            groups={groups}
            onSelectDate={onSelectDate}
            selectedDateKey={selectedDateKey}
          />
          <EmployeeSelectedDayAgenda events={selectedEvents} now={now} />
        </>
      )}
    </section>
  );
}

export function EmployeeDayAgenda({
  events,
  now,
}: Readonly<{ events: EmployeeCalendarEvent[]; now: Date }>) {
  return (
    <section className="employee-day-agenda" aria-label="日行程">
      <div className="employee-section-title">
        <h2>日行程</h2>
      </div>
      <PosterCardList events={events} now={now} />
    </section>
  );
}

export function EmployeeWeekCalendar({
  groups,
  onSelectDate,
  selectedDateKey,
}: Readonly<{
  groups: EmployeeCalendarDayGroup[];
  onSelectDate: (dateKey: string) => void;
  selectedDateKey: string;
}>) {
  return (
    <div className="employee-week-calendar" aria-label="週行事曆">
      {groups.map((group) => (
        <div
          className={[
            "employee-week-column",
            group.dateKey === selectedDateKey ? "selected" : "",
            group.isToday ? "today" : "",
          ]
            .filter(Boolean)
            .join(" ")}
          key={group.dateKey}
        >
          <CalendarDayHeader
            group={group}
            selected={group.dateKey === selectedDateKey}
            onSelectDate={onSelectDate}
          />
          <div className="employee-week-events">
            {group.events.length === 0 ? (
              <span className="employee-calendar-empty-inline">無活動</span>
            ) : (
              group.events.map((event) => (
                <EmployeeCalendarEventBlock
                  calendarEvent={event}
                  key={event.event.event_id}
                />
              ))
            )}
          </div>
        </div>
      ))}
    </div>
  );
}

export function EmployeeMonthCalendar({
  groups,
  onSelectDate,
  selectedDateKey,
}: Readonly<{
  groups: EmployeeCalendarDayGroup[];
  onSelectDate: (dateKey: string) => void;
  selectedDateKey: string;
}>) {
  return (
    <div className="employee-month-calendar" aria-label="月行事曆">
      {groups.map((group) => (
        <button
          aria-pressed={group.dateKey === selectedDateKey}
          className={[
            "employee-month-cell",
            group.dateKey === selectedDateKey ? "selected" : "",
            group.isToday ? "today" : "",
            group.isOutsideMonth ? "muted" : "",
          ]
            .filter(Boolean)
            .join(" ")}
          key={group.dateKey}
          type="button"
          onClick={() => onSelectDate(group.dateKey)}
        >
          <span className="employee-month-date">{group.dayNumber}</span>
          <span className="employee-month-count">
            {group.eventCount > 0 ? `${group.eventCount} 場` : ""}
            {group.hasRegistration ? " · 已報名" : ""}
          </span>
          <span className="employee-month-previews">
            {group.events.slice(0, 2).map(({ event }) => (
              <span key={event.event_id}>{event.title}</span>
            ))}
          </span>
        </button>
      ))}
    </div>
  );
}

export function EmployeeSelectedDayAgenda({
  events,
  now,
}: Readonly<{ events: EmployeeCalendarEvent[]; now: Date }>) {
  return (
    <section className="employee-selected-day-agenda" aria-label="選取日期活動">
      <div className="employee-section-title">
        <h2>選取日期活動</h2>
      </div>
      <PosterCardList events={events} now={now} />
    </section>
  );
}

function PosterCardList({
  events,
  now,
}: Readonly<{ events: EmployeeCalendarEvent[]; now: Date }>) {
  if (events.length === 0) {
    return (
      <EmptyState
        title="這天沒有活動"
        action="換一天看看，或等活動主辦發布新活動。"
      />
    );
  }
  return (
    <div className="employee-event-card-list">
      {events.map(({ event, ticket }) => (
        <EmployeeEventPosterCard
          event={event}
          key={event.event_id}
          now={now}
          ticket={ticket}
        />
      ))}
    </div>
  );
}

function CalendarDayHeader({
  group,
  onSelectDate,
  selected,
}: Readonly<{
  group: EmployeeCalendarDayGroup;
  onSelectDate: (dateKey: string) => void;
  selected: boolean;
}>) {
  return (
    <button
      aria-pressed={selected}
      className="employee-week-day-header"
      type="button"
      onClick={() => onSelectDate(group.dateKey)}
    >
      <span>{group.weekdayLabel}</span>
      <strong>{group.dayNumber}</strong>
      <small>
        {group.eventCount > 0 ? `${group.eventCount} 場` : "無活動"}
        {group.hasRegistration ? " · 已報名" : ""}
      </small>
    </button>
  );
}

function EmployeeCalendarEventBlock({
  calendarEvent,
}: Readonly<{ calendarEvent: EmployeeCalendarEvent }>) {
  const { deadline, event, state, ticket } = calendarEvent;
  const href = primaryHref(calendarEvent);
  return (
    <a
      className="employee-calendar-event-block"
      href={href}
      onClick={(clickEvent) =>
        runClientNavigation(clickEvent, () => navigate(href))
      }
    >
      <time>{eventTime(event.starts_at)}</time>
      <strong>{event.title}</strong>
      <span>{siteLabel(event.location || event.event_site)}</span>
      <span className="employee-calendar-event-badges">
        <StatusBadge tone={state.tone}>{state.label}</StatusBadge>
        <EmployeeRegistrationDeadlineChip deadline={deadline} />
      </span>
      {ticket ? <span className="sr-only">已產生票券</span> : null}
    </a>
  );
}

function primaryHref({ event, state, ticket }: EmployeeCalendarEvent) {
  if ((state.kind === "entry-ready" || state.kind === "registered") && ticket) {
    return ticketDetailPath(ticket.ticket_id);
  }
  return `/user/events/detail?event_id=${encodeURIComponent(event.event_id)}`;
}

function eventTime(value: string) {
  return new Date(value).toLocaleTimeString("zh-TW", {
    hour: "2-digit",
    minute: "2-digit",
  });
}
