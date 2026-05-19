import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { cancelMyRegistration, listEvents } from "@/lib/api";
import type { AuthMeClaims, EventSummary } from "@/lib/api";
import { errorMessage } from "@/lib/formatting";
import {
  Alert,
  CompactStatsBar,
  EmptyState,
  SkeletonRows,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useUrlTab } from "@/hooks/use-url-tab";
import {
  EmployeeEventCard,
  messageTone,
  registrationIDFor,
} from "./employee-event-components";
import { localizedMessage } from "@/lib/ui/options";
import { type BookingResultState } from "./employee-booking-result";
import { canSubmitAttendeeAction } from "./employee-event-state";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";

type TextByEvent = Record<string, string>;
type EmployeeEventTab = "available" | "registered" | "unavailable";
const employeeEventTabs = ["available", "registered", "unavailable"] as const;

export function EmployeeEventsPage({ claims }: { claims: AuthMeClaims }) {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [cancelReasons, setCancelReasons] = useState<TextByEvent>({});
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");
  const [pendingActionID, setPendingActionID] = useState("");
  const [bookingResults, setBookingResults] = useState<
    Record<string, BookingResultState>
  >({});
  const [activeTab, setActiveTab] = useUrlTab<EmployeeEventTab>(
    "tab",
    employeeEventTabs,
    "available",
  );

  const principalID = claims.employee_id;
  const eventStats = useMemo(
    () => ({
      eligible: events.filter((event) => canSubmitAttendeeAction(event)).length,
      confirmed: events.filter(
        (event) => event.current_user_status === "confirmed",
      ).length,
      waitlisted: events.filter(
        (event) => event.current_user_status === "waitlisted",
      ).length,
      openSeats: events.reduce(
        (sum, event) => sum + (event.remaining_capacity ?? 0),
        0,
      ),
    }),
    [events],
  );
  const availableEvents = events.filter((event) =>
    canSubmitAttendeeAction(event),
  );
  const registeredEvents = events.filter(
    (event) =>
      event.current_user_status === "confirmed" ||
      event.current_user_status === "waitlisted",
  );
  const unavailableEvents = events.filter(
    (event) =>
      !availableEvents.includes(event) && !registeredEvents.includes(event),
  );
  async function refresh() {
    setLoading(true);
    setMessage("");
    try {
      setEvents(await listEvents());
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, [principalID]);

  async function cancel(event: EventSummary) {
    const registrationID = registrationIDFor(event);
    if (!registrationID) return;
    const reason =
      (cancelReasons[event.event_id] || "").trim() || "employee cancellation";
    setMessage("");
    setPendingActionID(`cancel-${event.event_id}`);
    try {
      const result = await cancelMyRegistration(
        registrationID,
        reason,
        `cancel-${registrationID}-${principalID}`,
      );
      setMessage(localizedMessage(result.message));
      setBookingResults((current) => ({
        ...current,
        [event.event_id]: {
          title: "報名已取消",
          copy: cancellationResultCopy(event),
          tone: "ok",
        },
      }));
      await refresh();
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setPendingActionID("");
    }
  }

  return (
    <section className="content-grid">
      <Card className="panel span-12 focused-tabs event-list-panel">
        <div className="section-heading">
          <div>
            <h2>活動列表</h2>
            <p>先看可報名活動；完整資格與容量細節保留在活動詳情。</p>
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
        <CompactStatsBar
          items={[
            { label: "符合資格", value: eventStats.eligible },
            { label: "已報名", value: eventStats.confirmed },
            { label: "候補中", value: eventStats.waitlisted },
            { label: "剩餘名額", value: eventStats.openSeats },
          ]}
        />
        {message && <Alert tone={messageTone(message)}>{message}</Alert>}
        <Tabs
          value={activeTab}
          onValueChange={(value) => setActiveTab(value as EmployeeEventTab)}
          className="event-tabs"
        >
          <TabsList>
            <TabsTrigger value="available">
              可報名 {availableEvents.length}
            </TabsTrigger>
            <TabsTrigger value="registered">
              我的報名 {registeredEvents.length}
            </TabsTrigger>
            <TabsTrigger value="unavailable">
              不可報名 {unavailableEvents.length}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="available">
            <EventListState
              loading={loading}
              rows={availableEvents}
              emptyTitle={
                events.length === 0 ? "尚無活動" : "目前沒有可報名活動"
              }
              emptyAction={
                events.length === 0
                  ? "請活動主辦先發布活動。"
                  : "已報名、候補或不符合資格的活動已移到其他分頁。"
              }
              render={(event) => (
                <EmployeeEventCard
                  event={event}
                  key={event.event_id}
                  mode="available"
                />
              )}
            />
          </TabsContent>
          <TabsContent value="registered">
            <EventListState
              loading={loading}
              rows={registeredEvents}
              emptyTitle="尚無報名紀錄"
              emptyAction="完成報名或加入候補後，會出現在這裡。"
              render={(event) => (
                <EmployeeEventCard
                  cancelReason={cancelReasons[event.event_id] || ""}
                  event={event}
                  key={event.event_id}
                  mode="registered"
                  onCancel={() => void cancel(event)}
                  cancelBusy={pendingActionID === `cancel-${event.event_id}`}
                  onCancelReasonChange={(value) =>
                    setCancelReasons((current) => ({
                      ...current,
                      [event.event_id]: value,
                    }))
                  }
                  result={
                    bookingResults[event.event_id] && (
                      <div
                        className={`booking-result ${bookingResults[event.event_id].tone}`}
                        role="status"
                      >
                        <p>{bookingResults[event.event_id].copy}</p>
                      </div>
                    )
                  }
                />
              )}
            />
          </TabsContent>
          <TabsContent value="unavailable">
            <EventListState
              loading={loading}
              rows={unavailableEvents}
              emptyTitle="沒有不可報名活動"
              emptyAction="目前所有活動都可報名，或已在我的報名中。"
              render={(event) => (
                <EmployeeEventCard
                  event={event}
                  key={event.event_id}
                  mode="unavailable"
                />
              )}
            />
          </TabsContent>
        </Tabs>
      </Card>
    </section>
  );
}

function EventListState({
  emptyAction,
  emptyTitle,
  loading,
  render,
  rows,
}: {
  emptyAction: string;
  emptyTitle: string;
  loading: boolean;
  render: (event: EventSummary) => ReactNode;
  rows: EventSummary[];
}) {
  if (loading) return <SkeletonRows rows={3} />;
  if (rows.length === 0) {
    return <EmptyState title={emptyTitle} action={emptyAction} />;
  }
  return <div className="event-list">{rows.map(render)}</div>;
}

function cancellationResultCopy(event: EventSummary) {
  const ticketCopy = event.current_user_ticket
    ? "已核發票券會同步失效。"
    : "目前沒有已核發票券。";
  return `報名已取消。${ticketCopy}名額與候補可能更新；若需恢復或重新報名，請聯絡活動主辦。`;
}
