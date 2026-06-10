import { navigate } from "@/app/routes";
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
  onHide: (eventID: string) => void;
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
}: Readonly<
  Pick<CalendarViewProps, "onMove" | "onToday" | "onViewChange" | "range">
>) {
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
      <fieldset className="employee-calendar-mode">
        <legend className="sr-only">日曆視圖</legend>
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
      </fieldset>
    </div>
  );
}

export function EmployeeCalendarView({
  groups,
  now,
  onHide,
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
        <EmployeeDayAgenda events={selectedEvents} now={now} onHide={onHide} />
      )}
      {range.view === "week" && (
        <>
          <EmployeeWeekCalendar
            groups={groups}
            onHide={onHide}
            onSelectDate={onSelectDate}
            selectedDateKey={selectedDateKey}
          />
          <EmployeeSelectedDayAgenda
            events={selectedEvents}
            now={now}
            onHide={onHide}
          />
        </>
      )}
      {range.view === "month" && (
        <>
          <EmployeeMonthCalendar
            groups={groups}
            onHide={onHide}
            onSelectDate={onSelectDate}
            selectedDateKey={selectedDateKey}
          />
          <EmployeeSelectedDayAgenda
            events={selectedEvents}
            now={now}
            onHide={onHide}
          />
        </>
      )}
    </section>
  );
}

export function EmployeeDayAgenda({
  events,
  now,
  onHide,
}: Readonly<{
  events: EmployeeCalendarEvent[];
  now: Date;
  onHide?: (eventID: string) => void;
}>) {
  return (
    <section className="employee-day-agenda" aria-label="日行程">
      <div className="employee-section-title">
        <h2>日行程</h2>
      </div>
      <PosterCardList events={events} now={now} onHide={onHide} />
    </section>
  );
}

export function EmployeeWeekCalendar({
  groups,
  onHide,
  onSelectDate,
  selectedDateKey,
}: Readonly<{
  groups: EmployeeCalendarDayGroup[];
  onHide?: (eventID: string) => void;
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
                  onHide={onHide}
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
  onHide,
  onSelectDate,
  selectedDateKey,
}: Readonly<{
  groups: EmployeeCalendarDayGroup[];
  onHide?: (eventID: string) => void;
  onSelectDate: (dateKey: string) => void;
  selectedDateKey: string;
}>) {
  return (
    <div className="employee-month-calendar" aria-label="月行事曆">
      {groups.map((group) => (
        <EmployeeMonthCell
          group={group}
          key={group.dateKey}
          onHide={onHide}
          onSelectDate={onSelectDate}
          selected={group.dateKey === selectedDateKey}
        />
      ))}
    </div>
  );
}

function EmployeeMonthCell({
  group,
  onHide,
  onSelectDate,
  selected,
}: Readonly<{
  group: EmployeeCalendarDayGroup;
  onHide?: (eventID: string) => void;
  onSelectDate: (dateKey: string) => void;
  selected: boolean;
}>) {
  return (
    <div
      className={[
        "employee-month-cell",
        selected ? "selected" : "",
        group.isToday ? "today" : "",
        group.isOutsideMonth ? "muted" : "",
      ]
        .filter(Boolean)
        .join(" ")}
    >
      <button
        aria-label={calendarGroupLabel(group)}
        aria-pressed={selected}
        className="employee-month-select"
        type="button"
        onClick={() => onSelectDate(group.dateKey)}
      >
        <span className="employee-month-date">{group.dayNumber}</span>
        <span className="employee-month-count">
          {group.eventCount > 0 ? `${group.eventCount} 場` : ""}
          {group.hasRegistration ? " · 已報名" : ""}
        </span>
      </button>
      {group.events.length > 0 && (
        <ul className="employee-month-events">
          {group.events.map(({ event }) => (
            <li className="employee-month-event-row" key={event.event_id}>
              <button
                className="employee-month-event"
                title={event.title}
                type="button"
                onClick={() => onSelectDate(group.dateKey)}
              >
                {event.title}
              </button>
              {onHide && (
                <button
                  aria-label={`隱藏活動：${event.title}`}
                  className="employee-month-event-hide"
                  title="隱藏活動"
                  type="button"
                  onClick={() => onHide(event.event_id)}
                >
                  <Icon name="x" />
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function EmployeeSelectedDayAgenda({
  events,
  now,
  onHide,
}: Readonly<{
  events: EmployeeCalendarEvent[];
  now: Date;
  onHide?: (eventID: string) => void;
}>) {
  return (
    <section className="employee-selected-day-agenda" aria-label="選取日期活動">
      <div className="employee-section-title">
        <h2>選取日期活動</h2>
      </div>
      <PosterCardList events={events} now={now} onHide={onHide} />
    </section>
  );
}

function PosterCardList({
  events,
  now,
  onHide,
}: Readonly<{
  events: EmployeeCalendarEvent[];
  now: Date;
  onHide?: (eventID: string) => void;
}>) {
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
          onHide={onHide}
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
      aria-label={calendarGroupLabel(group)}
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
  onHide,
}: Readonly<{
  calendarEvent: EmployeeCalendarEvent;
  onHide?: (eventID: string) => void;
}>) {
  const { deadline, event, state, ticket } = calendarEvent;
  const href = primaryHref(calendarEvent);
  return (
    // The hide button is a sibling of the anchor (not nested inside it) to keep
    // interactive content out of the <a>.
    <div className="employee-calendar-event-block-wrap">
      <a
        aria-label={`開啟活動：${event.title}，${eventTimeRange(
          event.starts_at,
          event.ends_at,
        )}，${siteLabel(event.location || event.event_site)}，${state.label}`}
        className="employee-calendar-event-block"
        href={href}
        onClick={(clickEvent) =>
          runClientNavigation(clickEvent, () => navigate(href))
        }
      >
        <time>{eventTimeRange(event.starts_at, event.ends_at)}</time>
        <strong>{event.title}</strong>
        <span>{siteLabel(event.location || event.event_site)}</span>
        <span className="employee-calendar-event-badges">
          <StatusBadge tone={state.tone}>{state.label}</StatusBadge>
          <EmployeeRegistrationDeadlineChip deadline={deadline} />
        </span>
        {ticket ? <span className="sr-only">已產生票券</span> : null}
      </a>
      {onHide && (
        <button
          aria-label={`隱藏活動：${event.title}`}
          className="employee-calendar-event-hide"
          title="隱藏活動"
          type="button"
          onClick={() => onHide(event.event_id)}
        >
          <Icon name="x" />
        </button>
      )}
    </div>
  );
}

function primaryHref({ event }: EmployeeCalendarEvent) {
  return `/user/events/detail?event_id=${encodeURIComponent(event.event_id)}`;
}

function eventTimeRange(startsAt: string, endsAt: string) {
  const start = eventTime(startsAt);
  const end = eventTime(endsAt);
  if (!start || !end) return start || end || "時間待公布";
  return `${start} 至 ${end}`;
}

function eventTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleTimeString("zh-TW", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function calendarGroupLabel(group: EmployeeCalendarDayGroup) {
  const labels = [
    calendarDateLabel(group.dateKey),
    group.eventCount > 0 ? `${group.eventCount} 場活動` : "無活動",
    group.hasRegistration ? "已報名" : "",
    group.isToday ? "今天" : "",
    group.isOutsideMonth ? "非本月日期" : "",
  ].filter(Boolean);
  return labels.join("，");
}

function calendarDateLabel(dateKey: string) {
  return new Date(`${dateKey}T00:00:00`).toLocaleDateString("zh-TW", {
    year: "numeric",
    month: "long",
    day: "numeric",
    weekday: "long",
  });
}
