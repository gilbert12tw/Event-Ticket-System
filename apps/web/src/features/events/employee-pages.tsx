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
import { selectCurrentTicket } from "@/features/tickets/ticket-readiness";
import { TicketPanel } from "@/features/tickets/ticket-panel";
import { employeeEventDisplayState, localDateKey } from "./employee-calendar";
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
import { EmployeeAgenda } from "./employee-calendar-components";

export function EmployeeEventsPage({
  claims,
}: Readonly<{ claims: AuthMeClaims }>) {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState("");
  const [calendarState, setCalendarState] = useState(() =>
    parseEmployeeCalendarQuery(globalThis.location.search),
  );

  const principalID = claims.employee_id;
  const now = useMemo(() => new Date(), [events, tickets]);
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
  const registeredEvents = useMemo(
    () =>
      events.filter(
        (event) =>
          employeeEventDisplayState(
            event,
            ticketForEvent(tickets, event.event_id),
            now,
          ).isRegistered,
      ),
    [events, now, tickets],
  );
  const currentTicket = selectCurrentTicket(tickets);

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
          <div>
            <h2>活動首頁</h2>
            <p>用日曆安排活動時間，卡片會提示報名期限與參加狀態。</p>
          </div>
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
        {!loading && (
          <CurrentEventTicketPanel ticket={currentTicket} onRefresh={refresh} />
        )}
        {loading ? (
          <SkeletonRows rows={3} />
        ) : (
          <>
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
            {registeredEvents.length > 0 && (
              <EmployeeAgenda
                emptyAction=""
                emptyTitle=""
                events={registeredEvents}
                now={now}
                tickets={tickets}
                title="我的報名"
              />
            )}
          </>
        )}
      </Card>
    </section>
  );
}

function CurrentEventTicketPanel({
  onRefresh,
  ticket,
}: Readonly<{
  onRefresh: () => void;
  ticket?: Ticket;
}>) {
  if (ticket) {
    return (
      <div className="current-ticket-panel" aria-label="目前活動票券 QR code">
        <div className="section-heading">
          <div>
            <h3>目前活動票券</h3>
            <p>有可入場票券時，QR code 會直接顯示在活動首頁。</p>
          </div>
        </div>
        <TicketPanel compact ticket={ticket} onRefresh={onRefresh} />
      </div>
    );
  }
  return null;
}

function ticketForEvent(tickets: Ticket[], eventID: string) {
  return tickets.find((ticket) => ticket.event_id === eventID);
}
