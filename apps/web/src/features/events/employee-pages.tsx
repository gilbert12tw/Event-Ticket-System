import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { navigate } from "@/app/routes";
import { listEvents, listTickets } from "@/lib/api";
import type { AuthMeClaims, EventSummary, Ticket } from "@/lib/api";
import { errorMessage } from "@/lib/formatting";
import { Alert, EmptyState, SkeletonRows } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { messageTone } from "./employee-event-components";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { EmployeeEventPosterCard } from "./employee-calendar-components";
import { localDateKey } from "./employee-calendar";
import {
  calendarRangeForView,
  groupEmployeeCalendarEventsByDay,
  parseEmployeeCalendarQuery,
  selectEmployeeCalendarEvents,
  shiftCalendarDate,
  type EmployeeCalendarViewMode,
} from "./employee-calendar-planner";
import { EmployeeCalendarView } from "./employee-calendar-view";
import { EmployeeDiscoveryControls } from "./employee-discovery-controls";
import {
  defaultEmployeeDiscoveryState,
  discoveryIsActive,
  employeeDiscoveryCityOptions,
  employeeDiscoveryListSummary,
  employeeEventsPath,
  parseEmployeeExploreMode,
  parseEmployeeDiscoveryQuery,
  selectEmployeeDiscoveryEventRows,
  type EmployeeDiscoveryState,
  type EmployeeExploreMode,
} from "./employee-discovery";

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
  const [exploreMode, setExploreMode] = useState(() =>
    parseEmployeeExploreMode(globalThis.location.search),
  );
  const [discoveryState, setDiscoveryState] = useState(() =>
    parseEmployeeDiscoveryQuery(globalThis.location.search),
  );

  const principalID = claims.employee_id;
  const now = useMemo(
    () => new Date(injectedNow ?? Date.now()),
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
  const discoveryEvents = useMemo(
    () =>
      selectEmployeeDiscoveryEventRows(events, tickets, discoveryState, now),
    [discoveryState, events, now, tickets],
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
  const cityOptions = useMemo(
    () => employeeDiscoveryCityOptions(events, discoveryState.city),
    [discoveryState.city, events],
  );
  const discoverySummary = useMemo(
    () => employeeDiscoveryListSummary(discoveryEvents, discoveryState),
    [discoveryEvents, discoveryState],
  );
  const discoveryActive = discoveryIsActive(discoveryState);
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
    const syncFromLocation = () => {
      setExploreMode(parseEmployeeExploreMode(globalThis.location.search));
      setCalendarState(parseEmployeeCalendarQuery(globalThis.location.search));
      setDiscoveryState(
        parseEmployeeDiscoveryQuery(globalThis.location.search),
      );
    };
    globalThis.addEventListener("popstate", syncFromLocation);
    return () => globalThis.removeEventListener("popstate", syncFromLocation);
  }, []);

  function applyCalendarState(view: EmployeeCalendarViewMode, date: Date) {
    const dateKey = localDateKey(date);
    setCalendarState({ view, date, dateKey });
    navigate(employeeEventsPath(view, dateKey, discoveryState, exploreMode));
  }

  function applyDiscoveryState(next: EmployeeDiscoveryState) {
    setDiscoveryState(next);
    setExploreMode("list");
    navigate(
      employeeEventsPath(
        calendarState.view,
        calendarState.dateKey,
        next,
        "list",
      ),
    );
  }

  function clearDiscoveryState() {
    applyDiscoveryState(defaultEmployeeDiscoveryState);
  }

  function applyExploreMode(next: EmployeeExploreMode) {
    setExploreMode(next);
    navigate(
      employeeEventsPath(
        calendarState.view,
        calendarState.dateKey,
        discoveryState,
        next,
      ),
    );
  }

  function selectDate(dateKey: string) {
    applyCalendarState(calendarState.view, new Date(`${dateKey}T00:00:00`));
  }

  return (
    <section className="content-grid">
      <Card className="panel span-12 employee-events-home">
        <div className="employee-events-workflow-bar">
          <h2 className="sr-only">活動日曆</h2>
          <div
            aria-label="活動探索方式"
            className="employee-events-tabs"
            role="tablist"
          >
            <Button
              aria-controls="employee-events-calendar-panel"
              aria-selected={exploreMode === "calendar"}
              id="employee-events-tab-calendar"
              role="tab"
              size="sm"
              type="button"
              variant={exploreMode === "calendar" ? "secondary" : "ghost"}
              onClick={() => applyExploreMode("calendar")}
            >
              <Icon name="calendar" />
              日曆
            </Button>
            <Button
              aria-controls="employee-events-list-panel"
              aria-selected={exploreMode === "list"}
              id="employee-events-tab-list"
              role="tab"
              size="sm"
              type="button"
              variant={exploreMode === "list" ? "secondary" : "ghost"}
              onClick={() => applyExploreMode("list")}
            >
              <Icon name="clipboard" />
              活動列表
            </Button>
          </div>
          <div className="employee-events-actions">
            {exploreMode === "list" && discoveryActive && (
              <Button
                className="employee-events-clear"
                size="sm"
                type="button"
                variant="ghost"
                onClick={clearDiscoveryState}
              >
                <Icon name="x" />
                清除
              </Button>
            )}
            <Button
              className="employee-events-refresh"
              disabled={loading}
              size="sm"
              title="重新整理活動"
              type="button"
              variant="ghost"
              onClick={refresh}
            >
              <Icon name="refresh" />
              更新
            </Button>
          </div>
        </div>
        {message && <Alert tone={messageTone(message)}>{message}</Alert>}
        {loading ? (
          <SkeletonRows rows={3} />
        ) : (
          <>
            {exploreMode === "calendar" ? (
              <div
                aria-labelledby="employee-events-tab-calendar"
                id="employee-events-calendar-panel"
                role="tabpanel"
              >
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
              </div>
            ) : (
              <EmployeeEventListPanel
                controls={
                  <EmployeeDiscoveryControls
                    cityOptions={cityOptions}
                    discovery={discoveryState}
                    summary={discoverySummary}
                    onChange={applyDiscoveryState}
                  />
                }
                events={discoveryEvents}
                now={now}
              />
            )}
          </>
        )}
      </Card>
    </section>
  );
}

function EmployeeEventListPanel({
  controls,
  events,
  now,
}: Readonly<{
  controls: ReactNode;
  events: ReturnType<typeof selectEmployeeDiscoveryEventRows>;
  now: Date;
}>) {
  return (
    <section
      aria-labelledby="employee-events-tab-list"
      className="employee-event-list-panel"
      id="employee-events-list-panel"
      role="tabpanel"
    >
      {controls}
      <div className="employee-event-list-heading">
        <h2>活動列表</h2>
        <p>用關鍵字、地點或報名狀態快速縮小全部活動。</p>
      </div>
      {events.length === 0 ? (
        <EmptyState
          title="找不到符合條件的活動"
          action="清除搜尋，或回到日曆依日期查看活動。"
        />
      ) : (
        <div className="employee-event-card-list employee-event-list-results">
          {events.map(({ event, ticket }) => (
            <EmployeeEventPosterCard
              event={event}
              key={event.event_id}
              now={now}
              ticket={ticket}
            />
          ))}
        </div>
      )}
    </section>
  );
}
