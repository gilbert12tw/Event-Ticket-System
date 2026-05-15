import { useEffect, useMemo, useState } from "react";
import { bookEvent, cancelMyRegistration, getEvent, listEvents } from "@/lib/api";
import type { AuthMeClaims, EventSummary } from "@/lib/api";
import { getEligibilityDecision } from "@/lib/api/contracts";
import { navigate } from "@/app/routes";
import { bookingActionLabel, errorMessage, eventStatusTone, formatDate, registrationTone } from "@/lib/formatting";
import { Alert, EmptyState, IdentityCard, Kpi, ProgressMeter, ProviderClaimsCard, SkeletonRows, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { TicketPanel } from "@/features/tickets/pages";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { EligibilityWarningList } from "./eligibility-warning";

const maxFamilyCount = 10;

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
      confirmed: events.filter((event) => event.current_user_status === "confirmed").length,
      waitlisted: events.filter((event) => event.current_user_status === "waitlisted").length,
      openSeats: events.reduce((sum, event) => sum + (event.remaining_capacity ?? 0), 0)
    }),
    [events]
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
      const familyCount = event.capacity_type === "unlimited" ? familyCounts[event.event_id] ?? 0 : 0;
      const result = await bookEvent(event.event_id, `book-${event.event_id}-${principalID}`, familyCount);
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
    const reason = (cancelReasons[event.event_id] || "").trim() || "employee cancellation";
    setMessage("");
    try {
      const result = await cancelMyRegistration(registrationID, reason, `cancel-${registrationID}-${principalID}`);
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
          <p>Book eligible events, track waitlist state, and cancel open registrations with a retry-safe request.</p>
        </div>
        <IdentityCard claims={claims} />
        <div className="context-kpis">
          <Kpi label="Eligible" value={eventStats.eligible} />
          <Kpi label="Confirmed" value={eventStats.confirmed} />
          <Kpi label="Waitlisted" value={eventStats.waitlisted} />
          <Kpi label="Open seats" value={eventStats.openSeats} />
        </div>
      </div>
      <div className="panel span-8">
        <div className="section-heading">
          <div>
            <h2>Available events</h2>
            <p>Limited events issue one employee-bound ticket. Unlimited events keep companions on the main registration.</p>
          </div>
          <button className="button secondary" type="button" onClick={refresh} disabled={loading}>
            <Icon name="refresh" />
            Refresh
          </button>
        </div>
        {message && <Alert tone={messageTone(message)}>{message}</Alert>}
        <div className="event-list" aria-busy={loading}>
          {loading && <SkeletonRows rows={3} />}
          {!loading && events.length === 0 && <EmptyState title="No events published" action="Ask an activity admin to publish an event." />}
          {!loading &&
            events.map((event) => (
              <EmployeeEventCard
                cancelReason={cancelReasons[event.event_id] || ""}
                event={event}
                familyCount={familyCounts[event.event_id] ?? 0}
                key={event.event_id}
                onBook={() => void requestBooking(event)}
                onCancel={() => void cancel(event)}
                onCancelReasonChange={(value) => setCancelReasons((current) => ({ ...current, [event.event_id]: value }))}
                onFamilyCountChange={(value) => setFamilyCounts((current) => ({ ...current, [event.event_id]: value }))}
              />
            ))}
        </div>
      </div>
      <ProviderClaimsCard claims={claims} />
      <Dialog open={pendingCrossCity !== null} onOpenChange={(open) => (!open ? setPendingCrossCity(null) : null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Cross-city confirmation</DialogTitle>
            <DialogDescription>
              {pendingCrossCity ? crossCityWarningMessage(pendingCrossCity) : ""}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <button className="button secondary" type="button" onClick={() => setPendingCrossCity(null)}>
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
  const [selectedID, setSelectedID] = useState(() => new URLSearchParams(window.location.search).get("event_id") || "");
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
    window.history.replaceState({}, "", `/user/events/detail${eventID ? `?event_id=${encodeURIComponent(eventID)}` : ""}`);
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
      const nextFamilyCount = detail.capacity_type === "unlimited" ? familyCount : 0;
      const result = await bookEvent(detail.event_id, `book-${detail.event_id}-${principalID}`, nextFamilyCount);
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
      const result = await cancelMyRegistration(registrationID, cancelReason.trim() || "employee cancellation", `cancel-${registrationID}-${principalID}`);
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
          <p>Review eligibility, family-count limits, cancellation state, and ticket handoff from the same event record.</p>
        </div>
        <label className="field compact">
          <span>Event</span>
          <select value={selectedID} onChange={(event) => void selectEvent(event.target.value)} disabled={busy}>
            <option value="">Choose event</option>
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
            <h2>Booking facts</h2>
            <p>Provider claims and event policy are rechecked before booking or cancellation.</p>
          </div>
          <button className="button secondary" type="button" onClick={() => void refresh()} disabled={busy || !selectedID}>
            <Icon name="refresh" />
            Refresh
          </button>
        </div>
        {message && <Alert tone={messageTone(message)}>{message}</Alert>}
        {!detail && <EmptyState title="No event selected" action="Choose an event to inspect booking state." />}
        {detail && <EventSummaryBlock event={detail} />}
      </div>
      <div className="panel span-4">
        <h2>Action</h2>
        {!detail && <EmptyState title="Choose an event" action="Booking and cancellation controls appear here." />}
        {detail && (
          <div className="summary-block">
            <FamilyCountControl event={detail} value={familyCount} onChange={setFamilyCount} />
            <button className="button full-width" type="button" onClick={() => void bookSelected()} disabled={!canBook(detail)}>
              <Icon name="ticket" />
              {bookingActionLabel(detail)}
            </button>
            <CancellationControl event={detail} reason={cancelReason} onCancel={() => void cancelSelected()} onReasonChange={setCancelReason} />
            {detail.current_user_ticket && <TicketPanel compact ticket={detail.current_user_ticket} />}
          </div>
        )}
      </div>
      <Dialog open={pendingCrossCity} onOpenChange={(open) => (!open ? setPendingCrossCity(false) : null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Cross-city confirmation</DialogTitle>
            <DialogDescription>{detail ? crossCityWarningMessage(detail) : ""}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <button className="button secondary" type="button" onClick={() => setPendingCrossCity(false)}>
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

function EmployeeEventCard({
  cancelReason,
  event,
  familyCount,
  onBook,
  onCancel,
  onCancelReasonChange,
  onFamilyCountChange
}: {
  cancelReason: string;
  event: EventSummary;
  familyCount: number;
  onBook: () => void;
  onCancel: () => void;
  onCancelReasonChange: (value: string) => void;
  onFamilyCountChange: (value: number) => void;
}) {
  return (
    <article className="event-card">
      <EventSummaryBlock event={event} compact />
      <div className="event-action">
        <Kpi label="Capacity" value={event.capacity_type === "limited" ? event.capacity ?? 0 : "Open"} />
        <Kpi label="Open" value={event.remaining_capacity ?? "No cap"} />
        <Kpi label="Waitlist" value={event.waitlist_count} />
        <FamilyCountControl event={event} value={familyCount} onChange={onFamilyCountChange} />
        <button className="button" type="button" disabled={!canBook(event)} onClick={onBook}>
          <Icon name="ticket" />
          {bookingActionLabel(event)}
        </button>
        <button className="button secondary" type="button" onClick={() => navigate(`/user/events/detail?event_id=${encodeURIComponent(event.event_id)}`)}>
          <Icon name="audit" />
          Detail
        </button>
        <CancellationControl event={event} reason={cancelReason} onCancel={onCancel} onReasonChange={onCancelReasonChange} />
      </div>
    </article>
  );
}

function EventSummaryBlock({ compact = false, event }: { compact?: boolean; event: EventSummary }) {
  const capacityMax = event.capacity ?? Math.max(event.confirmed_count + event.waitlist_count, 1);
  const eligibility = getEligibilityDecision(event);
  const cooldown = eligibility?.no_show_cooldown ?? event.no_show_cooldown;
  const isEligible = eligibility ? eligibility.eligible : false;
  const warnings = eligibility?.warnings ?? [];
  const ineligibleReasons = eligibility && !eligibility.eligible ? (eligibility.reasons ?? []) : [];

  return (
    <div className={compact ? "" : "summary-block"}>
      <div className="event-card-top">
        <StatusBadge tone={isEligible && !cooldown?.active ? "ok" : "fail"}>{eligibilityLabel(event)}</StatusBadge>
        <StatusBadge tone={event.capacity_type === "unlimited" ? "info" : "neutral"}>
          {event.capacity_type === "unlimited" ? "Unlimited activity" : "Limited capacity"}
        </StatusBadge>
        <StatusBadge tone={eventStatusTone(event.status)}>{event.status}</StatusBadge>
        {event.current_user_status && <StatusBadge tone={registrationTone(event.current_user_status)}>{event.current_user_status}</StatusBadge>}
      </div>
      <h3>{event.title}</h3>
      <p>{event.description || "No description provided."}</p>
      {cooldown?.active && <Alert tone="warn">Limited-event booking is blocked by no-show cooldown until {formatDate(cooldown.until || "")}.</Alert>}
      {warnings.length > 0 && <EligibilityWarningList warnings={warnings} />}
      {ineligibleReasons.length > 0 && (
        <Alert tone="fail">
          <strong>Not eligible:</strong>{" "}
          {ineligibleReasons.join("; ")}.
          {" "}Contact HR or the event organizer if this looks incorrect.
        </Alert>
      )}
      <dl className="meta-list">
        {!compact && (
          <div>
            <dt>Event ID</dt>
            <dd>{event.event_id}</dd>
          </div>
        )}
        <div>
          <dt>Location</dt>
          <dd>{event.location || event.event_site || "Not set"}</dd>
        </div>
        {event.event_city && (
          <div>
            <dt>Event city</dt>
            <dd>{event.event_city}</dd>
          </div>
        )}
        <div>
          <dt>Starts</dt>
          <dd>{formatDate(event.starts_at)}</dd>
        </div>
        <div>
          <dt>Registration closes</dt>
          <dd>{formatDate(event.registration_close)}</dd>
        </div>
        <div>
          <dt>Rule</dt>
          <dd>
            {event.rule.department} / {event.rule.site} / G{event.rule.min_grade}+
          </dd>
        </div>
        {event.allows_family && event.capacity_type === "unlimited" && (
          <div>
            <dt>Family participation</dt>
            <dd>Allowed. Family count is used for planning and entry support; tickets remain tied to the employee registration.</dd>
          </div>
        )}
      </dl>
      {event.capacity_type === "limited" ? (
        <ProgressMeter
          label="容量使用"
          value={event.confirmed_count}
          max={capacityMax}
          helper={`${event.confirmed_count}/${capacityMax} confirmed, ${event.waitlist_count} waitlisted`}
        />
      ) : (
        <p className="form-hint">Unlimited event: booking does not decrement inventory or create companion tickets.</p>
      )}
    </div>
  );
}

function FamilyCountControl({ event, onChange, value }: { event: EventSummary; onChange: (value: number) => void; value: number }) {
  if (event.capacity_type === "limited") {
    return <p className="form-hint">Limited event: companions are not available.</p>;
  }
  if (!event.allows_family) {
    return <p className="form-hint">Unlimited event: no companions allowed for this event.</p>;
  }
  const boundedValue = Math.min(Math.max(value, 0), maxFamilyCount);
  return (
    <label className="field compact">
      <span>Companions</span>
      <input
        aria-label={`Companions for ${event.title}`}
        max={maxFamilyCount}
        min={0}
        onChange={(input) => onChange(Math.min(Math.max(Number(input.target.value || 0), 0), maxFamilyCount))}
        type="number"
        value={boundedValue}
      />
      <small className="form-hint">0 to {maxFamilyCount}; companions are recorded on your main ticket.</small>
    </label>
  );
}

function CancellationControl({
  event,
  onCancel,
  onReasonChange,
  reason
}: {
  event: EventSummary;
  onCancel: () => void;
  onReasonChange: (value: string) => void;
  reason: string;
}) {
  const registrationID = registrationIDFor(event);
  const booked = event.current_user_status === "confirmed" || event.current_user_status === "waitlisted";
  if (!booked) return <p className="form-hint">No active registration to cancel.</p>;
  const open = cancellationOpen(event);
  return (
    <div className="cancel-box">
      <label className="field compact">
        <span>Cancellation reason</span>
        <input value={reason} onChange={(input) => onReasonChange(input.target.value)} placeholder="Optional reason" disabled={!open} />
      </label>
      <button aria-label="Cancel registration" className="button secondary" type="button" onClick={onCancel} disabled={!open || !registrationID}>
        <Icon name="x" />
        Cancel
      </button>
      <p className="form-hint">
        {open ? "Cancellation is retry-safe while registration is open." : "Self-cancel is closed. Contact an activity admin for an exception."}
      </p>
    </div>
  );
}

function canBook(event: EventSummary) {
  const eligibility = getEligibilityDecision(event);
  // Derive can_book from typed eligibility if available; fall back to legacy flat fields.
  const bookable = eligibility ? eligibility.can_book && !eligibility.no_show_cooldown.active : (event.eligible ?? false) && !event.no_show_cooldown?.active;
  return bookable && event.current_user_status !== "confirmed" && event.current_user_status !== "waitlisted";
}

function eligibilityLabel(event: EventSummary) {
  const eligibility = getEligibilityDecision(event);
  if (eligibility) {
    if (eligibility.eligible) return eligibility.warnings.length > 0 ? "符合資格 ⚠" : "符合資格";
    return eligibility.reasons[0] || "不符合資格";
  }
  // Legacy flat-field fallback
  const reason = (event.eligibility_reason ?? "").trim();
  if (!reason || reason === "eligible") return event.eligible ? "符合資格" : "不符合資格";
  return reason;
}

function needsCrossCityConfirm(event: EventSummary) {
  const eligibility = getEligibilityDecision(event);
  return eligibility?.warnings?.some((warning) => warning.code === "cross_city") ?? false;
}

function crossCityWarningMessage(event: EventSummary) {
  const eligibility = getEligibilityDecision(event);
  const warning = eligibility?.warnings?.find((item) => item.code === "cross_city");
  if (warning?.message) return warning.message;
  if (event.event_city) return `This event is in ${event.event_city}; please confirm before booking.`;
  return "This event is in a different city; please confirm before booking.";
}

function cancellationOpen(event: EventSummary) {
  const close = Date.parse(event.registration_close);
  return Number.isFinite(close) && close > Date.now();
}

function registrationIDFor(event: EventSummary) {
  return event.current_user_registration_id || event.current_user_ticket?.registration_id || "";
}

function messageTone(message: string): "ok" | "warn" | "fail" | "info" {
  const lower = message.toLowerCase();
  if (lower.includes("cancelled") || lower.includes("confirmed")) return "ok";
  if (lower.includes("cooldown") || lower.includes("closed") || lower.includes("waitlist")) return "warn";
  if (lower.includes("error") || lower.includes("failed") || lower.includes("not eligible")) return "fail";
  return "info";
}
