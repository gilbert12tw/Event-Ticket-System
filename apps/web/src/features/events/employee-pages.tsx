import { useEffect, useMemo, useState } from "react";
import { bookEvent, cancelMyRegistration, getEvent, listEvents } from "@/lib/api";
import type { AuthMeClaims, EventSummary } from "@/lib/api";
import { navigate } from "@/app/routes";
import {
  bookingActionLabel,
  capacityTypeLabel,
  errorMessage,
  eventStatusLabel,
  eventStatusTone,
  formatDate,
  registrationStatusLabel,
  registrationTone
} from "@/lib/formatting";
import { Alert, EmptyState, IdentityCard, Kpi, ProgressMeter, ProviderClaimsCard, SkeletonRows, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { TicketPanel } from "@/features/tickets/pages";

const maxFamilyCount = 10;

type NumberByEvent = Record<string, number>;
type TextByEvent = Record<string, string>;

export function EmployeeEventsPage({ claims }: { claims: AuthMeClaims }) {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [familyCounts, setFamilyCounts] = useState<NumberByEvent>({});
  const [cancelReasons, setCancelReasons] = useState<TextByEvent>({});
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");

  const principalID = claims.employee_id;
  const eventStats = useMemo(
    () => ({
      eligible: events.filter((event) => event.eligible && !event.no_show_cooldown?.active).length,
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

  async function cancel(event: EventSummary) {
    const registrationID = registrationIDFor(event);
    if (!registrationID) return;
    const reason = (cancelReasons[event.event_id] || "").trim() || "員工取消報名";
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
          <p>查看可報名活動、追蹤候補狀態，並在報名開放期間安全取消既有報名。</p>
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
            <p>限量活動會核發本人票券；不限量活動可將同行人數記錄在同一筆報名。</p>
          </div>
          <button className="button secondary" type="button" onClick={refresh} disabled={loading}>
            <Icon name="refresh" />
            重新整理
          </button>
        </div>
        {message && <Alert tone={messageTone(message)}>{message}</Alert>}
        <div className="event-list" aria-busy={loading}>
          {loading && <SkeletonRows rows={3} />}
          {!loading && events.length === 0 && <EmptyState title="目前沒有已發布活動" action="請活動管理員先發布活動。" />}
          {!loading &&
            events.map((event) => (
              <EmployeeEventCard
                cancelReason={cancelReasons[event.event_id] || ""}
                event={event}
                familyCount={familyCounts[event.event_id] ?? 0}
                key={event.event_id}
                onBook={() => void book(event)}
                onCancel={() => void cancel(event)}
                onCancelReasonChange={(value) => setCancelReasons((current) => ({ ...current, [event.event_id]: value }))}
                onFamilyCountChange={(value) => setFamilyCounts((current) => ({ ...current, [event.event_id]: value }))}
              />
            ))}
        </div>
      </div>
      <ProviderClaimsCard claims={claims} />
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

  async function bookSelected() {
    if (!detail) return;
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
      const result = await cancelMyRegistration(registrationID, cancelReason.trim() || "員工取消報名", `cancel-${registrationID}-${principalID}`);
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
          <select value={selectedID} onChange={(event) => void selectEvent(event.target.value)} disabled={busy}>
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
          <button className="button secondary" type="button" onClick={() => void refresh()} disabled={busy || !selectedID}>
            <Icon name="refresh" />
            重新整理
          </button>
        </div>
        {message && <Alert tone={messageTone(message)}>{message}</Alert>}
        {!detail && <EmptyState title="尚未選擇活動" action="選擇活動後會顯示報名狀態。" />}
        {detail && <EventSummaryBlock event={detail} />}
      </div>
      <div className="panel span-4">
        <h2>報名決策</h2>
        {!detail && <EmptyState title="等待活動" action="選擇活動後會顯示可執行動作。" />}
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
        <Kpi label="容量" value={event.capacity_type === "limited" ? event.capacity ?? 0 : "不限"} />
        <Kpi label="剩餘" value={event.remaining_capacity ?? "不限"} />
        <Kpi label="候補" value={event.waitlist_count} />
        <FamilyCountControl event={event} value={familyCount} onChange={onFamilyCountChange} />
        <button className="button" type="button" disabled={!canBook(event)} onClick={onBook}>
          <Icon name="ticket" />
          {bookingActionLabel(event)}
        </button>
        <button className="button secondary" type="button" onClick={() => navigate(`/user/events/detail?event_id=${encodeURIComponent(event.event_id)}`)}>
          <Icon name="audit" />
          詳情
        </button>
        <CancellationControl event={event} reason={cancelReason} onCancel={onCancel} onReasonChange={onCancelReasonChange} />
      </div>
    </article>
  );
}

function EventSummaryBlock({ compact = false, event }: { compact?: boolean; event: EventSummary }) {
  const capacityMax = event.capacity ?? Math.max(event.confirmed_count + event.waitlist_count, 1);
  const cooldown = event.no_show_cooldown;
  return (
    <div className={compact ? "" : "summary-block"}>
      <div className="event-card-top">
        <StatusBadge tone={event.eligible && !cooldown?.active ? "ok" : "fail"}>{eligibilityLabel(event)}</StatusBadge>
        <StatusBadge tone={event.capacity_type === "unlimited" ? "info" : "neutral"}>{capacityTypeLabel(event.capacity_type)}</StatusBadge>
        <StatusBadge tone={eventStatusTone(event.status)}>{eventStatusLabel(event.status)}</StatusBadge>
        {event.current_user_status && (
          <StatusBadge tone={registrationTone(event.current_user_status)}>{registrationStatusLabel(event.current_user_status)}</StatusBadge>
        )}
      </div>
      <h3>{event.title}</h3>
      <p>{event.description || "此活動尚未填寫描述。"}</p>
      {cooldown?.active && <Alert tone="warn">限量活動報名因未報到冷卻而暫停，直到 {formatDisplayDate(cooldown.until || "")}。</Alert>}
      <dl className="meta-list">
        {!compact && (
          <div>
            <dt>活動 ID</dt>
            <dd>{event.event_id}</dd>
          </div>
        )}
        <div>
          <dt>地點</dt>
          <dd>{event.location || event.event_site || "未設定"}</dd>
        </div>
        <div>
          <dt>開始時間</dt>
          <dd>{formatDisplayDate(event.starts_at)}</dd>
        </div>
        <div>
          <dt>報名截止</dt>
          <dd>{formatDisplayDate(event.registration_close)}</dd>
        </div>
        <div>
          <dt>資格規則</dt>
          <dd>
            {event.rule.department} / {event.rule.site} / G{event.rule.min_grade}+
          </dd>
        </div>
      </dl>
      {event.capacity_type === "limited" ? (
        <ProgressMeter
          label="容量使用"
          value={event.confirmed_count}
          max={capacityMax}
          helper={`${event.confirmed_count}/${capacityMax} 已確認，候補 ${event.waitlist_count}`}
        />
      ) : (
        <p className="form-hint">不限量活動：報名不會扣除庫存，也不會為同行人另發票券。</p>
      )}
    </div>
  );
}

function FamilyCountControl({ event, onChange, value }: { event: EventSummary; onChange: (value: number) => void; value: number }) {
  if (event.capacity_type === "limited") {
    return <p className="form-hint">限量活動不開放同行人數。</p>;
  }
  if (!event.allows_family) {
    return <p className="form-hint">此不限量活動不開放同行人數。</p>;
  }
  const boundedValue = Math.min(Math.max(value, 0), maxFamilyCount);
  return (
    <label className="field compact">
      <span>同行人數</span>
      <input
        aria-label={`同行人數：${event.title}`}
        max={maxFamilyCount}
        min={0}
        onChange={(input) => onChange(Math.min(Math.max(Number(input.target.value || 0), 0), maxFamilyCount))}
        type="number"
        value={boundedValue}
      />
      <small className="form-hint">可填 0 到 {maxFamilyCount} 人；同行人數會記錄在你的主要票券。</small>
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
  if (!booked) return <p className="form-hint">目前沒有可取消的報名。</p>;
  const open = cancellationOpen(event);
  return (
    <div className="cancel-box">
      <label className="field compact">
        <span>取消原因</span>
        <input value={reason} onChange={(input) => onReasonChange(input.target.value)} placeholder="可選填原因" disabled={!open} />
      </label>
      <button aria-label="取消報名" className="button secondary" type="button" onClick={onCancel} disabled={!open || !registrationID}>
        <Icon name="x" />
        取消報名
      </button>
      <p className="form-hint">
        {open ? "報名開放期間可安全重試取消報名。" : "自助取消已關閉，請聯絡活動管理員處理例外。"}
      </p>
    </div>
  );
}

function canBook(event: EventSummary) {
  return event.eligible && !event.no_show_cooldown?.active && event.current_user_status !== "confirmed" && event.current_user_status !== "waitlisted";
}

function eligibilityLabel(event: EventSummary) {
  const reason = event.eligibility_reason.trim();
  if (!reason) return event.eligible ? "符合資格" : "不可報名";
  if (reason === "eligible") return "符合資格";
  return reason;
}

function cancellationOpen(event: EventSummary) {
  const close = Date.parse(event.registration_close);
  return Number.isFinite(close) && close > Date.now();
}

function registrationIDFor(event: EventSummary) {
  return event.current_user_registration_id || event.current_user_ticket?.registration_id || "";
}

function formatDisplayDate(value?: string | null) {
  return value ? formatDate(value) : "未設定";
}

function messageTone(message: string): "ok" | "warn" | "fail" | "info" {
  const lower = message.toLowerCase();
  if (lower.includes("cancelled") || lower.includes("confirmed")) return "ok";
  if (lower.includes("cooldown") || lower.includes("closed") || lower.includes("waitlist")) return "warn";
  if (lower.includes("error") || lower.includes("failed") || lower.includes("not eligible")) return "fail";
  return "info";
}
