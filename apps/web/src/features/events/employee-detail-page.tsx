import type { MouseEvent } from "react";
import { useEffect, useRef, useState } from "react";
import { navigate, ticketDetailPath } from "@/app/routes";
import {
  bookEvent,
  cancelMyRegistration,
  getEvent,
  listEvents,
} from "@/lib/api";
import type {
  AuthMeClaims,
  BookingResponse,
  EventSummary,
  Ticket,
} from "@/lib/api";
import { errorMessage } from "@/lib/formatting";
import {
  Alert,
  EmptyState,
  SelectField,
  StatusBadge,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { localizedMessage } from "@/lib/ui/options";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  ActionSummary,
  BookingResultBlock,
  bookingResultFromExistingEvent,
  bookingResultFromResponse,
  focusBookingResult,
  type BookingResultState,
} from "./employee-booking-result";
import {
  EventSummaryBlock,
  FamilyCountControl,
  WaitlistPolicy,
  messageTone,
  registrationIDFor,
} from "./employee-event-components";
import { CancellationControl } from "./employee-cancellation-control";
import {
  attendeeActionState,
  canSubmitAttendeeAction,
} from "./employee-event-state";

type PendingAction = "book" | "cancel" | "";

export function EmployeeEventDetailPage({ claims }: { claims: AuthMeClaims }) {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [selectedID, setSelectedID] = useState(
    () => new URLSearchParams(window.location.search).get("event_id") || "",
  );
  const [detail, setDetail] = useState<EventSummary | null>(null);
  const [familyCount, setFamilyCount] = useState(0);
  const [cancelReason, setCancelReason] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [bookingResult, setBookingResult] = useState<BookingResultState | null>(
    null,
  );
  const [pendingAction, setPendingAction] = useState<PendingAction>("");
  const principalID = claims.employee_id;
  const refreshRequestRef = useRef(0);

  async function refresh(nextID = selectedID) {
    const requestID = refreshRequestRef.current + 1;
    refreshRequestRef.current = requestID;
    setBusy(true);
    setMessage("");
    try {
      const rows = await listEvents();
      const eventID = nextID || rows[0]?.event_id || "";
      const nextDetail = eventID ? await getEvent(eventID) : null;
      if (requestID !== refreshRequestRef.current) return;
      setEvents(rows);
      setSelectedID(eventID);
      setDetail(nextDetail);
    } catch (error) {
      if (requestID === refreshRequestRef.current)
        setMessage(errorMessage(error));
    } finally {
      if (requestID === refreshRequestRef.current) setBusy(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, [principalID]);

  async function selectEvent(eventID: string) {
    setSelectedID(eventID);
    setBookingResult(null);
    setMessage("");
    window.history.replaceState(
      {},
      "",
      `/user/events/detail${
        eventID ? `?event_id=${encodeURIComponent(eventID)}` : ""
      }`,
    );
    await refresh(eventID);
  }

  async function bookSelected() {
    if (!detail) return;
    const existingResult = bookingResultFromExistingEvent(detail);
    if (existingResult) {
      setMessage("");
      setBookingResult(existingResult);
      focusBookingResult(detail.event_id);
      return;
    }
    setMessage("");
    setPendingAction("book");
    try {
      const nextFamilyCount =
        detail.capacity_type === "unlimited" ? familyCount : 0;
      const result = await bookEvent(
        detail.event_id,
        `book-${detail.event_id}-${principalID}`,
        nextFamilyCount,
      );
      setMessage(localizedMessage(result.message));
      setBookingResult(bookingResultFromResponse(result));
      applyBookingResponse(result);
      await refresh(detail.event_id);
      applyBookingResponse(result);
      focusBookingResult(detail.event_id);
    } catch (error) {
      const copy = errorMessage(error);
      setMessage(copy);
      setBookingResult({
        title: "報名未完成",
        copy,
        tone: "fail",
      });
      focusBookingResult(detail.event_id);
    } finally {
      setPendingAction("");
    }
  }

  function applyBookingResponse(response: BookingResponse) {
    setDetail((current) =>
      current?.event_id === response.registration.event_id
        ? eventWithBookingResponse(current, response)
        : current,
    );
    setEvents((current) =>
      current.map((event) =>
        event.event_id === response.registration.event_id
          ? eventWithBookingResponse(event, response)
          : event,
      ),
    );
  }

  async function cancelSelected() {
    if (!detail) return;
    const registrationID = registrationIDFor(detail);
    if (!registrationID) return;
    setMessage("");
    setPendingAction("cancel");
    try {
      const result = await cancelMyRegistration(
        registrationID,
        cancelReason.trim() || "employee cancellation",
        `cancel-${registrationID}-${principalID}`,
      );
      setMessage(localizedMessage(result.message));
      setBookingResult({
        title: "報名已取消",
        copy: cancellationResultCopy(detail),
        tone: "ok",
      });
      await refresh(detail.event_id);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setPendingAction("");
    }
  }

  return (
    <section className="content-grid">
      <Card className="panel span-8 event-detail-check-panel">
        <div className="section-heading">
          <div>
            <h2>報名前檢查</h2>
            <p>系統送出時仍會重新檢查資格、活動狀態與名額。</p>
          </div>
          <div className="toolbar">
            <SelectField
              className="compact-field"
              label="活動"
              value={selectedID}
              options={[
                { value: "", label: "選擇活動" },
                ...events.map((event) => ({
                  value: event.event_id,
                  label: event.title,
                })),
              ]}
              onChange={(value) => void selectEvent(value)}
            />
            <Button
              variant="outline"
              type="button"
              onClick={() => void refresh()}
              disabled={busy || !selectedID}
            >
              <Icon name="refresh" />
              重新整理
            </Button>
          </div>
        </div>
        {message && <Alert tone={messageTone(message)}>{message}</Alert>}
        {!detail && (
          <EmptyState
            title="尚未選擇活動"
            action="請選擇活動以檢查報名狀態。"
          />
        )}
        {detail && <EventSummaryBlock event={detail} />}
      </Card>
      <Card className="panel span-4 event-primary-action-panel">
        <h2>主要操作</h2>
        {detail && (
          <p className="form-hint event-action-context">{detail.title}</p>
        )}
        {!detail && (
          <EmptyState
            title="尚未選擇活動"
            action="選擇活動後會顯示報名與取消控制。"
          />
        )}
        {detail && (
          <div className="summary-block event-action-rail">
            <DetailActionControls
              claims={claims}
              detail={detail}
              familyCount={familyCount}
              pendingAction={pendingAction}
              onBook={() => void bookSelected()}
              onFamilyCountChange={setFamilyCount}
            />
            <CancellationControl
              busy={pendingAction === "cancel"}
              event={detail}
              reason={cancelReason}
              onCancel={() => void cancelSelected()}
              onReasonChange={setCancelReason}
            />
            <BookingResultBlock
              eventID={detail.event_id}
              result={bookingResult || undefined}
            />
            {detail.current_user_ticket?.status === "active" &&
              !bookingResult?.ticketID && (
                <TicketHandoff ticket={detail.current_user_ticket} />
              )}
          </div>
        )}
      </Card>
    </section>
  );
}

function eventWithBookingResponse(
  event: EventSummary,
  response: BookingResponse,
): EventSummary {
  const status = response.registration.status;
  return {
    ...event,
    current_user_registration_id: response.registration.registration_id,
    current_user_status: status,
    current_user_ticket: response.ticket ?? event.current_user_ticket,
    remaining_capacity:
      response.remaining_capacity ?? event.remaining_capacity ?? null,
  };
}

function DetailActionControls({
  claims,
  detail,
  familyCount,
  onBook,
  onFamilyCountChange,
  pendingAction,
}: {
  claims: AuthMeClaims;
  detail: EventSummary;
  familyCount: number;
  onBook: () => void;
  onFamilyCountChange: (value: number) => void;
  pendingAction: PendingAction;
}) {
  const action = attendeeActionState(detail);
  const canSubmit = canSubmitAttendeeAction(detail);
  return (
    <>
      <div className="action-readiness">
        <StatusBadge tone={action.tone}>{action.label}</StatusBadge>
        <span>{action.recoveryCopy}</span>
      </div>
      {action.kind !== "ticket" && (
        <ActionSummary
          claims={claims}
          event={detail}
          familyCount={familyCount}
        />
      )}
      <WaitlistPolicy event={detail} />
      <FamilyCountControl
        event={detail}
        value={familyCount}
        onChange={onFamilyCountChange}
      />
      {action.kind !== "ticket" && (
        <Button
          className="w-full"
          type="button"
          variant={canSubmit ? "default" : "outline"}
          title={!canSubmit ? action.recoveryCopy : undefined}
          onClick={onBook}
          disabled={pendingAction === "book" || !canSubmit}
        >
          <Icon name={canSubmit ? "ticket" : "ban"} />
          {pendingAction === "book" ? "送出中" : action.label}
        </Button>
      )}
    </>
  );
}

function TicketHandoff({ ticket }: { ticket: Ticket }) {
  return (
    <div className="ticket-handoff">
      <span>二維碼已移到我的票券詳細頁，入場時再開啟即可。</span>
      <Button asChild variant="outline">
        <a
          href={ticketDetailPath(ticket.ticket_id)}
          onClick={(event) => {
            if (shouldUseNativeNavigation(event)) return;
            event.preventDefault();
            navigate(ticketDetailPath(ticket.ticket_id));
          }}
        >
          <Icon name="ticket" />
          查看這張票券
        </a>
      </Button>
    </div>
  );
}

function cancellationResultCopy(event: EventSummary) {
  const ticketCopy = event.current_user_ticket
    ? "已核發票券會同步失效。"
    : "目前沒有已核發票券。";
  return `報名已取消。${ticketCopy}名額與候補可能更新；若需恢復或重新報名，請聯絡活動主辦。`;
}

function shouldUseNativeNavigation(event: MouseEvent<HTMLAnchorElement>) {
  return (
    event.button !== 0 ||
    event.metaKey ||
    event.ctrlKey ||
    event.altKey ||
    event.shiftKey
  );
}
