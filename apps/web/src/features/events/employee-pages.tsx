import { useEffect, useMemo, useState } from "react";
import {
  bookEvent,
  cancelMyRegistration,
  getEvent,
  listEvents,
} from "@/lib/api";
import type { AuthMeClaims, EventSummary } from "@/lib/api";
import { getEligibilityDecision } from "@/lib/api/contracts";
import { bookingActionLabel, errorMessage } from "@/lib/formatting";
import {
  Alert,
  EmptyState,
  IdentityCard,
  Kpi,
  ProviderClaimsCard,
  SkeletonRows,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { TicketPanel } from "@/features/tickets/pages";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  canBook,
  CancellationControl,
  EmployeeEventCard,
  EventSummaryBlock,
  FamilyCountControl,
  messageTone,
  registrationIDFor,
} from "./employee-event-components";

type NumberByEvent = Record<string, number>;
type TextByEvent = Record<string, string>;

export function EmployeeEventsPage({ claims }: { claims: AuthMeClaims }) {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [familyCounts, setFamilyCounts] = useState<NumberByEvent>({});
  const [cancelReasons, setCancelReasons] = useState<TextByEvent>({});
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");
  const [pendingCrossCity, setPendingCrossCity] = useState<EventSummary | null>(null);

  const principalID = claims.employee_id;
  const eventStats = useMemo(
    () => ({
      eligible: events.filter((event) => {
        const d = getEligibilityDecision(event);
        return d ? d.can_book && !d.no_show_cooldown.active : false;
      }).length,
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

  async function book(event: EventSummary) {
    setMessage("");
    try {
      const familyCount =
        event.capacity_type === "unlimited"
          ? (familyCounts[event.event_id] ?? 0)
          : 0;
      const result = await bookEvent(
        event.event_id,
        `book-${event.event_id}-${principalID}`,
        familyCount,
      );
      setMessage(result.message);
      await refresh();
    } catch (error) {
      setMessage(errorMessage(error));
    }
  }

  async function requestBooking(event: EventSummary) {
    if (needsCrossCityConfirm(event)) {
      setPendingCrossCity(event);
      return;
    }
    await book(event);
  }

  async function cancel(event: EventSummary) {
    const registrationID = registrationIDFor(event);
    if (!registrationID) return;
    const reason =
      (cancelReasons[event.event_id] || "").trim() || "員工取消報名";
    setMessage("");
    try {
      const result = await cancelMyRegistration(
        registrationID,
        reason,
        `cancel-${registrationID}-${principalID}`,
      );
      setMessage(result.message);
      await refresh();
    } catch (error) {
      setMessage(errorMessage(error));
    }
  }

  return (
    <section className="content-grid">
      <div className="panel span-12 workspace-context user-context">
        <div>
          <div className="eyebrow">User Workspace</div>
          <h2>員工入口</h2>
          <p>
            查看可報名活動、追蹤候補狀態，並在報名開放期間安全取消既有報名。
          </p>
        </div>
        <IdentityCard claims={claims} />
        <div className="context-kpis">
          <Kpi label="可報名" value={eventStats.eligible} />
          <Kpi label="已確認" value={eventStats.confirmed} />
          <Kpi label="候補中" value={eventStats.waitlisted} />
          <Kpi label="剩餘名額" value={eventStats.openSeats} />
        </div>
      </div>
      <div className="panel span-8">
        <div className="section-heading">
          <div>
            <h2>可報名活動</h2>
            <p>
              限量活動會核發本人票券；不限量活動可將同行人數記錄在同一筆報名。
            </p>
          </div>
          <button
            className="button secondary"
            type="button"
            onClick={refresh}
            disabled={loading}
          >
            <Icon name="refresh" />
            重新整理
          </button>
        </div>
        {message && <Alert tone={messageTone(message)}>{message}</Alert>}
        <div className="event-list" aria-busy={loading}>
          {loading && <SkeletonRows rows={3} />}
          {!loading && events.length === 0 && (
            <EmptyState
              title="目前沒有已發布活動"
              action="請活動管理員先發布活動。"
            />
          )}
          {!loading &&
            events.map((event) => (
              <EmployeeEventCard
                cancelReason={cancelReasons[event.event_id] || ""}
                event={event}
                familyCount={familyCounts[event.event_id] ?? 0}
                key={event.event_id}
                onBook={() => void requestBooking(event)}
                onCancel={() => void cancel(event)}
                onCancelReasonChange={(value) =>
                  setCancelReasons((current) => ({
                    ...current,
                    [event.event_id]: value,
                  }))
                }
                onFamilyCountChange={(value) =>
                  setFamilyCounts((current) => ({
                    ...current,
                    [event.event_id]: value,
                  }))
                }
              />
            ))}
        </div>
      </div>
      <ProviderClaimsCard claims={claims} />
      <Dialog
        open={pendingCrossCity !== null}
        onOpenChange={(open) => (!open ? setPendingCrossCity(null) : null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Cross-city confirmation</DialogTitle>
            <DialogDescription>
              {pendingCrossCity ? crossCityWarningMessage(pendingCrossCity) : ""}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <button
              className="button secondary"
              type="button"
              onClick={() => setPendingCrossCity(null)}
            >
              Cancel
            </button>
            <button
              className="button"
              type="button"
              onClick={() => {
                if (pendingCrossCity) {
                  void book(pendingCrossCity);
                }
                setPendingCrossCity(null);
              }}
            >
              Confirm booking
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}

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
  const [pendingCrossCity, setPendingCrossCity] = useState(false);
  const principalID = claims.employee_id;

  async function refresh(nextID = selectedID) {
    setBusy(true);
    setMessage("");
    try {
      const rows = await listEvents();
      setEvents(rows);
      const eventID = nextID || rows[0]?.event_id || "";
      setSelectedID(eventID);
      setDetail(eventID ? await getEvent(eventID) : null);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, [principalID]);

  async function selectEvent(eventID: string) {
    setSelectedID(eventID);
    window.history.replaceState(
      {},
      "",
      `/user/events/detail${eventID ? `?event_id=${encodeURIComponent(eventID)}` : ""}`,
    );
    await refresh(eventID);
  }

  async function bookSelected(confirmCrossCity = false) {
    if (!detail) return;
    if (!confirmCrossCity && needsCrossCityConfirm(detail)) {
      setPendingCrossCity(true);
      return;
    }
    setMessage("");
    try {
      const nextFamilyCount =
        detail.capacity_type === "unlimited" ? familyCount : 0;
      const result = await bookEvent(
        detail.event_id,
        `book-${detail.event_id}-${principalID}`,
        nextFamilyCount,
      );
      setMessage(result.message);
      await refresh(detail.event_id);
    } catch (error) {
      setMessage(errorMessage(error));
    }
  }

  async function cancelSelected() {
    if (!detail) return;
    const registrationID = registrationIDFor(detail);
    if (!registrationID) return;
    setMessage("");
    try {
      const result = await cancelMyRegistration(
        registrationID,
        cancelReason.trim() || "員工取消報名",
        `cancel-${registrationID}-${principalID}`,
      );
      setMessage(result.message);
      await refresh(detail.event_id);
    } catch (error) {
      setMessage(errorMessage(error));
    }
  }

  return (
    <section className="content-grid">
      <div className="panel span-12 workspace-context user-context">
        <div>
          <div className="eyebrow">User Workspace</div>
          <h2>單一活動詳情</h2>
          <p>在同一筆活動資料中確認資格、同行人數上限、取消狀態與票券交付。</p>
        </div>
        <label className="field compact">
          <span>活動</span>
          <select
            value={selectedID}
            onChange={(event) => void selectEvent(event.target.value)}
            disabled={busy}
          >
            <option value="">選擇活動</option>
            {events.map((event) => (
              <option value={event.event_id} key={event.event_id}>
                {event.title}
              </option>
            ))}
          </select>
        </label>
      </div>
      <div className="panel span-8">
        <div className="section-heading">
          <div>
            <h2>活動與報名狀態</h2>
            <p>報名或取消前會重新檢查身分資料與活動規則。</p>
          </div>
          <button
            className="button secondary"
            type="button"
            onClick={() => void refresh()}
            disabled={busy || !selectedID}
          >
            <Icon name="refresh" />
            重新整理
          </button>
        </div>
        {message && <Alert tone={messageTone(message)}>{message}</Alert>}
        {!detail && (
          <EmptyState
            title="尚未選擇活動"
            action="選擇活動後會顯示報名狀態。"
          />
        )}
        {detail && <EventSummaryBlock event={detail} />}
      </div>
      <div className="panel span-4">
        <h2>報名決策</h2>
        {!detail && (
          <EmptyState title="等待活動" action="選擇活動後會顯示可執行動作。" />
        )}
        {detail && (
          <div className="summary-block">
            <FamilyCountControl
              event={detail}
              value={familyCount}
              onChange={setFamilyCount}
            />
            <button
              className="button full-width"
              type="button"
              onClick={() => void bookSelected()}
              disabled={!canBook(detail)}
            >
              <Icon name="ticket" />
              {bookingActionLabel(detail)}
            </button>
            <CancellationControl
              event={detail}
              reason={cancelReason}
              onCancel={() => void cancelSelected()}
              onReasonChange={setCancelReason}
            />
            {detail.current_user_ticket && (
              <TicketPanel compact ticket={detail.current_user_ticket} />
            )}
          </div>
        )}
      </div>
      <Dialog
        open={pendingCrossCity}
        onOpenChange={(open) => (!open ? setPendingCrossCity(false) : null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Cross-city confirmation</DialogTitle>
            <DialogDescription>
              {detail ? crossCityWarningMessage(detail) : ""}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <button
              className="button secondary"
              type="button"
              onClick={() => setPendingCrossCity(false)}
            >
              Cancel
            </button>
            <button
              className="button"
              type="button"
              onClick={() => {
                setPendingCrossCity(false);
                void bookSelected(true);
              }}
            >
              Confirm booking
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}

function needsCrossCityConfirm(event: EventSummary) {
  const eligibility = getEligibilityDecision(event);
  return (
    eligibility?.warnings?.some((warning) => warning.code === "cross_city") ??
    false
  );
}

function crossCityWarningMessage(event: EventSummary) {
  const eligibility = getEligibilityDecision(event);
  const warning = eligibility?.warnings?.find(
    (item) => item.code === "cross_city",
  );
  if (warning?.message) return warning.message;
  if (event.event_city)
    return `This event is in ${event.event_city}; please confirm before booking.`;
  return "This event is in a different city; please confirm before booking.";
}
