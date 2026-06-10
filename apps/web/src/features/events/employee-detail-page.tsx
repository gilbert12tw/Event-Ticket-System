import { useEffect, useRef, useState } from "react";
import { navigate, ticketDetailPath } from "@/app/routes";
import {
  bookEvent,
  cancelMyRegistration,
  getEvent,
  listEvents,
} from "@/lib/api";
import type { AuthMeClaims, BookingResponse, EventSummary } from "@/lib/api";
import { errorMessage } from "@/lib/formatting";
import { runClientNavigation } from "@/lib/navigation";
import { Alert, EmptyState, StatusBadge } from "@/components/shared";
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
import { EmployeeEventDetailHero } from "./employee-calendar-components";
import { registrationDeadlineView } from "./employee-calendar-planner";
import { EmployeeRegistrationDeadlineChip } from "./employee-registration-deadline-chip";
import { canAddToCalendar } from "./employee-calendar-export";
import { EmployeeAddToCalendarButton } from "./employee-add-to-calendar-button";

type PendingAction = "book" | "cancel" | "";

export function EmployeeEventDetailPage({
  claims,
}: Readonly<{ claims: AuthMeClaims }>) {
  const [selectedID, setSelectedID] = useState(
    () => new URLSearchParams(globalThis.location.search).get("event_id") || "",
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
  const [calendarMessage, setCalendarMessage] = useState("");
  const principalID = claims.employee_id;
  const refreshRequestRef = useRef(0);
  const bookingResultRef = useRef<BookingResultState | null>(null);

  async function refresh(nextID = selectedID) {
    const requestID = refreshRequestRef.current + 1;
    refreshRequestRef.current = requestID;
    setBusy(true);
    setMessage("");
    try {
      const rows = await listEvents();
      const eventID = nextID === "" ? (rows[0]?.event_id ?? "") : nextID;
      const nextDetail = eventID ? await getEvent(eventID) : null;
      if (requestID !== refreshRequestRef.current) return;
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

  async function bookSelected() {
    if (!detail) return;
    const existingResult = bookingResultFromExistingEvent(detail);
    if (existingResult) {
      setMessage("");
      keepBookingResult(existingResult);
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
      const nextBookingResult = bookingResultFromResponse(result);
      setMessage(localizedMessage(result.message));
      applyBookingResponse(result);
      await refresh(detail.event_id);
      applyBookingResponse(result);
      keepBookingResult(nextBookingResult);
      focusBookingResult(detail.event_id);
    } catch (error) {
      const copy = errorMessage(error);
      setMessage(copy);
      keepBookingResult({
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
  }

  function keepBookingResult(result: BookingResultState) {
    bookingResultRef.current = result;
    setBookingResult(result);
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
      const nextBookingResult: BookingResultState = {
        title: "報名已取消",
        copy: cancellationResultCopy(detail),
        tone: "ok",
      };
      setMessage(localizedMessage(result.message));
      await refresh(detail.event_id);
      keepBookingResult(nextBookingResult);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setPendingAction("");
    }
  }

  const visibleBookingResult = bookingResult || bookingResultRef.current;
  const now = new Date();

  if (!detail) {
    return (
      <section className="content-grid">
        <Card className="panel span-12 employee-event-missing">
          <EmptyState
            title={busy ? "載入活動中" : "找不到活動"}
            action={
              busy
                ? "正在載入活動資料。"
                : "回到活動首頁，從行事曆選擇你想看的活動。"
            }
          />
          {message && <Alert tone={messageTone(message)}>{message}</Alert>}
          {!busy && (
            <Button asChild>
              <a
                href="/user/events"
                onClick={(event) =>
                  runClientNavigation(event, () => navigate("/user/events"))
                }
              >
                <Icon name="calendar" />
                回活動
              </a>
            </Button>
          )}
        </Card>
      </section>
    );
  }

  return (
    <section className="content-grid event-detail-app">
      <Card className="panel span-8 employee-event-hero-panel">
        <EmployeeEventDetailHero event={detail} now={now} />
      </Card>
      <Card className="panel span-4 event-primary-action-panel employee-event-detail-action-panel">
        <h2>主要操作</h2>
        <p className="form-hint event-action-context">{detail.title}</p>
        <EmployeeRegistrationDeadlineChip
          deadline={registrationDeadlineView(detail, now)}
        />
        <div className="summary-block event-action-rail event-detail-action-bar">
          <DetailActionControls
            claims={claims}
            detail={detail}
            familyCount={familyCount}
            pendingAction={pendingAction}
            onBook={() => void bookSelected()}
            onFamilyCountChange={setFamilyCount}
            suppressActiveTicketLink={Boolean(visibleBookingResult?.ticketID)}
          />
          {canAddToCalendar(detail, detail.current_user_ticket) && (
            <div className="employee-detail-calendar-action">
              <span>
                <strong>行事曆提醒</strong>
                <small aria-live="polite">
                  {calendarMessage ||
                    "下載 .ics 後可匯入手機、Google 或 Outlook 行事曆。"}
                </small>
              </span>
              <EmployeeAddToCalendarButton
                className="employee-detail-calendar-download"
                event={detail}
                label="下載行事曆 (.ics)"
                onDownloaded={(filename) =>
                  setCalendarMessage(`已下載行事曆檔案：${filename}`)
                }
              />
            </div>
          )}
          <CancellationControl
            busy={pendingAction === "cancel"}
            event={detail}
            reason={cancelReason}
            onCancel={() => void cancelSelected()}
            onReasonChange={setCancelReason}
          />
          <BookingResultBlock
            eventID={detail.event_id}
            result={visibleBookingResult || undefined}
          />
        </div>
      </Card>
      <Card className="panel span-12 event-detail-check-panel">
        {message && <Alert tone={messageTone(message)}>{message}</Alert>}
        <details className="event-readiness-details">
          <summary>
            <span>
              <strong>報名前系統檢查</strong>
              <small>{detailReadinessCopy(detail)}</small>
            </span>
          </summary>
          <div className="event-readiness-body">
            <p className="form-hint">
              送出時系統仍會重新檢查資格、活動狀態與名額。
            </p>
            <EventSummaryBlock compact event={detail} />
            <div className="toolbar">
              <Button asChild variant="outline">
                <a
                  href="/user/events"
                  onClick={(event) =>
                    runClientNavigation(event, () => navigate("/user/events"))
                  }
                >
                  <Icon name="calendar" />
                  回活動
                </a>
              </Button>
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
        </details>
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
  suppressActiveTicketLink = false,
}: Readonly<{
  claims: AuthMeClaims;
  detail: EventSummary;
  familyCount: number;
  onBook: () => void;
  onFamilyCountChange: (value: number) => void;
  pendingAction: PendingAction;
  suppressActiveTicketLink?: boolean;
}>) {
  const action = attendeeActionState(detail);
  const canSubmit = canSubmitAttendeeAction(detail);
  const activeTicketID =
    !suppressActiveTicketLink &&
    action.kind === "ticket" &&
    detail.current_user_ticket?.status === "active"
      ? detail.current_user_ticket.ticket_id
      : "";
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
      {activeTicketID ? (
        <Button asChild className="w-full" variant="default">
          <a
            href={ticketDetailPath(activeTicketID)}
            onClick={(event) =>
              runClientNavigation(event, () =>
                navigate(ticketDetailPath(activeTicketID)),
              )
            }
          >
            <Icon name="ticket" />
            查看票券
          </a>
        </Button>
      ) : (
        <Button
          className="w-full"
          type="button"
          variant={canSubmit ? "default" : "outline"}
          title={canSubmit ? undefined : action.recoveryCopy}
          onClick={onBook}
          disabled={pendingAction === "book" || !canSubmit}
        >
          <Icon name={canSubmit ? "ticket" : "ban"} />
          {pendingAction === "book" ? "送出中…" : action.label}
        </Button>
      )}
    </>
  );
}

function detailReadinessCopy(event: EventSummary) {
  if (event.current_user_ticket?.status === "active") {
    return "你已完成報名，可以查看票券。";
  }
  if (event.current_user_status === "confirmed") {
    return "你已完成報名，票券核發後會出現在我的票券。";
  }
  if (event.current_user_status === "waitlisted") {
    return "你已在候補名單中。";
  }
  if (canSubmitAttendeeAction(event)) {
    return "你符合資格，可以報名。";
  }
  return "目前不能報名，展開查看原因。";
}

function cancellationResultCopy(event: EventSummary) {
  const ticketCopy = event.current_user_ticket
    ? "已核發票券會同步失效。"
    : "目前沒有已核發票券。";
  return `報名已取消。${ticketCopy}名額與候補可能更新；若需恢復或重新報名，請聯絡活動主辦。`;
}
