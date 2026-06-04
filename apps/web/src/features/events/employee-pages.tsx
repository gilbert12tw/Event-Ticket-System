import { useEffect, useMemo, useState } from "react";
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
import {
  employeeEventDisplayState,
  groupEventsByCalendarDay,
  groupEventsByMonth,
  localDateKey,
  selectEmployeeAgenda,
} from "./employee-calendar";
import {
  EmployeeAgenda,
  EmployeeCalendarStrip,
  agendaTitle,
} from "./employee-calendar-components";

export function EmployeeEventsPage({
  claims,
}: Readonly<{ claims: AuthMeClaims }>) {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState("");
  const [calendarOpen, setCalendarOpen] = useState(false);
  const [selectedDateKey, setSelectedDateKey] = useState(() =>
    localDateKey(new Date()),
  );

  const principalID = claims.employee_id;
  const now = useMemo(() => new Date(), [events, tickets]);
  const selectedDate = useMemo(
    () => new Date(`${selectedDateKey}T00:00:00`),
    [selectedDateKey],
  );
  const weekDays = useMemo(
    () => groupEventsByCalendarDay(events, now),
    [events, now],
  );
  const monthDays = useMemo(
    () => groupEventsByMonth(events, selectedDate, now),
    [events, now, selectedDate],
  );
  const agendaEvents = useMemo(
    () => selectEmployeeAgenda(events, selectedDateKey, tickets, now),
    [events, selectedDateKey, tickets, now],
  );
  const registeredEvents = useMemo(
    () =>
      events.filter((event) =>
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

  return (
    <section className="content-grid">
      <Card className="panel span-12 employee-events-home">
        <div className="section-heading">
          <div>
            <h2>活動首頁</h2>
            <p>用行事曆查看最近活動，選一天就能看到適合你的安排。</p>
          </div>
          <Button
            variant="outline"
            type="button"
            onClick={refresh}
            disabled={loading}
          >
            <Icon name="refresh" />
            重新整理
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
            <EmployeeCalendarStrip
              days={weekDays}
              monthDays={monthDays}
              monthOpen={calendarOpen}
              selectedDateKey={selectedDateKey}
              onSelectDate={setSelectedDateKey}
              onToggleMonth={() => setCalendarOpen((open) => !open)}
            />
            <EmployeeAgenda
              events={agendaEvents}
              now={now}
              tickets={tickets}
              title={agendaTitle(selectedDateKey, now)}
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
