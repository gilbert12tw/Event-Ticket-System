import { useEffect, useMemo, useState } from "react";
import { navigate } from "@/app/routes";
import { listEvents, listTickets } from "@/lib/api";
import type { AuthMeClaims, EventSummary, Ticket } from "@/lib/api";
import { errorMessage } from "@/lib/formatting";
import { Alert, SkeletonRows } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { messageTone } from "./employee-event-components";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { localDateKey } from "./employee-calendar";
import {
  calendarRangeForView,
  employeeCalendarPath,
  groupEmployeeCalendarEventsByDay,
  parseEmployeeCalendarQuery,
  selectEmployeeCalendarEvents,
  shiftCalendarDate,
  type EmployeeCalendarViewMode,
} from "./employee-calendar-planner";
import { EmployeeCalendarView } from "./employee-calendar-view";

export function EmployeeEventsPage({
  claims,
  now: injectedNow,
}: Readonly<{ claims: AuthMeClaims; now?: Date }>) {
  const initialNow = injectedNow ?? new Date();
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState("");
  const [calendarState, setCalendarState] = useState(() =>
    parseEmployeeCalendarQuery(globalThis.location.search, initialNow),
  );

  const principalID = claims.employee_id;
  const now = useMemo(
    () => new Date((injectedNow ?? new Date()).getTime()),
    [events, injectedNow, tickets],
  );
  const calendarRange = useMemo(
    () => calendarRangeForView(calendarState.view, calendarState.date, now),
    [calendarState.date, calendarState.view, now],
  );
  const calendarEvents = useMemo(
    () => selectEmployeeCalendarEvents(events, tickets, calendarRange, now),
    [calendarRange, events, now, tickets],
  );
  const calendarGroups = useMemo(
    () => groupEmployeeCalendarEventsByDay(calendarEvents, calendarRange),
    [calendarEvents, calendarRange],
  );
  const selectedEvents = useMemo(
    () =>
      calendarEvents.filter(
        ({ event }) =>
          localDateKey(new Date(event.starts_at)) === calendarState.dateKey,
      ),
    [calendarEvents, calendarState.dateKey],
  );
  async function refresh() {
    setLoading(true);
    setMessage("");
    try {
      const [nextEvents, nextTickets] = await Promise.all([
        listEvents(),
        listTickets(),
      ]);
      setEvents(nextEvents);
      setTickets(nextTickets);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, [principalID]);

  useEffect(() => {
    const syncFromLocation = () =>
      setCalendarState(parseEmployeeCalendarQuery(globalThis.location.search));
    globalThis.addEventListener("popstate", syncFromLocation);
    return () => globalThis.removeEventListener("popstate", syncFromLocation);
  }, []);

  function applyCalendarState(view: EmployeeCalendarViewMode, date: Date) {
    const dateKey = localDateKey(date);
    setCalendarState({ view, date, dateKey });
    navigate(employeeCalendarPath(view, dateKey));
  }

  function selectDate(dateKey: string) {
    applyCalendarState(calendarState.view, new Date(`${dateKey}T00:00:00`));
  }

  return (
    <section className="content-grid">
      <Card className="panel span-12 employee-events-home">
        <div className="section-heading">
          <h2 className="sr-only">活動日曆</h2>
          <Button
            aria-label="重新整理活動"
            className="employee-page-refresh"
            disabled={loading}
            size="icon"
            title="重新整理活動"
            type="button"
            variant="ghost"
            onClick={refresh}
          >
            <Icon name="refresh" />
          </Button>
        </div>
        {message && <Alert tone={messageTone(message)}>{message}</Alert>}
        {loading ? (
          <SkeletonRows rows={3} />
        ) : (
          <EmployeeCalendarView
            groups={calendarGroups}
            now={now}
            range={calendarRange}
            selectedDateKey={calendarState.dateKey}
            selectedEvents={selectedEvents}
            onMove={(direction) =>
              applyCalendarState(
                calendarState.view,
                shiftCalendarDate(
                  calendarState.view,
                  calendarState.date,
                  direction,
                ),
              )
            }
            onSelectDate={selectDate}
            onToday={() => applyCalendarState(calendarState.view, now)}
            onViewChange={(view) =>
              applyCalendarState(view, calendarState.date)
            }
          />
        )}
      </Card>
    </section>
  );
}
