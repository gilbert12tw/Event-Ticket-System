import { useEffect, useMemo, useState } from "react";
import { bookEvent, getEvent, listEvents } from "@/lib/api";
import type { AuthMeClaims, EventSummary } from "@/lib/api";
import { navigate } from "@/app/routes";
import { bookingActionLabel, errorMessage, eventStatusTone, formatDate, registrationTone } from "@/lib/formatting";
import { Alert, EmptyState, IdentityCard, Kpi, ProgressMeter, ProviderClaimsCard, SkeletonRows, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { TicketPanel } from "@/features/tickets/pages";

export function EmployeeEventsPage({ claims }: { claims: AuthMeClaims }) {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");

  const principalID = claims.employee_id;
  const eventStats = useMemo(
    () => ({
      eligible: events.filter((event) => event.eligible).length,
      confirmed: events.filter((event) => event.current_user_status === "confirmed").length,
      waitlisted: events.filter((event) => event.current_user_status === "waitlisted").length,
      openSeats: events.reduce((sum, event) => sum + Math.max(event.remaining_capacity, 0), 0)
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

  async function book(eventID: string) {
    setMessage("");
    try {
      const result = await bookEvent(eventID, `book-${eventID}-${principalID}`);
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
          <p>以目前員工 HR 屬性判斷活動資格，報名結果會立即反映 confirmed、waitlisted 或不可報名原因。</p>
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
            <p>依員工屬性顯示資格、剩餘名額與目前報名狀態。</p>
          </div>
          <button className="button secondary" type="button" onClick={refresh} disabled={loading}>
            <Icon name="refresh" />
            重新整理
          </button>
        </div>
        {message && <Alert tone={message.includes("eligible") || message.includes("不") ? "warn" : "info"}>{message}</Alert>}
        <div className="event-list" aria-busy={loading}>
          {loading && <SkeletonRows rows={3} />}
          {!loading && events.length === 0 && (
            <EmptyState title="目前沒有已發布活動" action="請到活動主辦頁建立一筆活動，或執行 Demo Runbook。" />
          )}
          {!loading &&
            events.map((event) => (
              <article className="event-card" key={event.event_id}>
                <div>
                  <div className="event-card-top">
                    <StatusBadge tone={event.eligible ? "ok" : "fail"}>
                      {event.eligible ? "符合資格" : "不可報名"}
                    </StatusBadge>
                    <StatusBadge tone={eventStatusTone(event.status)}>{event.status}</StatusBadge>
                    {event.current_user_status && (
                      <StatusBadge tone={registrationTone(event.current_user_status)}>{event.current_user_status}</StatusBadge>
                    )}
                  </div>
                  <h3>{event.title}</h3>
                  <p>{event.description || "此活動尚未填寫描述。"}</p>
                  <dl className="meta-list">
                    <div>
                      <dt>地點</dt>
                      <dd>{event.location || "未設定"}</dd>
                    </div>
                    <div>
                      <dt>開始時間</dt>
                      <dd>{formatDate(event.starts_at)}</dd>
                    </div>
                    <div>
                      <dt>報名截止</dt>
                      <dd>{formatDate(event.registration_close)}</dd>
                    </div>
                    <div>
                      <dt>資格規則</dt>
                      <dd>
                        {event.rule.department} / {event.rule.site} / G{event.rule.min_grade}+
                      </dd>
                    </div>
                  </dl>
                  <ProgressMeter
                    label="容量使用"
                    value={event.confirmed_count}
                    max={event.capacity}
                    helper={`${event.confirmed_count}/${event.capacity} confirmed，候補 ${event.waitlist_count}`}
                  />
                </div>
                <div className="event-action">
                  <Kpi label="總名額" value={event.capacity} />
                  <Kpi label="剩餘" value={event.remaining_capacity} />
                  <Kpi label="候補" value={event.waitlist_count} />
                  <button
                    className="button"
                    type="button"
                    disabled={!event.eligible || event.current_user_status === "confirmed" || event.current_user_status === "waitlisted"}
                    onClick={() => void book(event.event_id)}
                  >
                    <Icon name="ticket" />
                    {bookingActionLabel(event)}
                  </button>
                  <button className="button secondary" type="button" onClick={() => navigate(`/user/events/detail?event_id=${encodeURIComponent(event.event_id)}`)}>
                    <Icon name="audit" />
                    詳情
                  </button>
                  <p className="form-hint">{event.eligible ? `報名模式：${event.allocation_mode}` : event.eligibility_reason}</p>
                </div>
              </article>
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
      const result = await bookEvent(detail.event_id, `book-${detail.event_id}-${principalID}`);
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
          <p>使用新單筆 API 重新檢查資格與目前報名狀態，避免只依賴列表快取。</p>
        </div>
        <label className="field compact">
          <span>選擇活動</span>
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
            <h2>活動與資格狀態</h2>
            <p>單筆查詢以 provider claims 身分重新計算資格與目前報名狀態。</p>
          </div>
          <button className="button secondary" type="button" onClick={() => void refresh()} disabled={busy || !selectedID}>
            <Icon name="refresh" />
            重新整理
          </button>
        </div>
        {message && <Alert tone={message.includes("eligible") || message.includes("not") ? "warn" : "info"}>{message}</Alert>}
        {!detail && <EmptyState title="尚未選擇活動" action="從下拉選單選擇活動後會顯示完整狀態。" />}
        {detail && (
          <div className="summary-block">
            <div className="event-card-top">
              <StatusBadge tone={detail.eligible ? "ok" : "fail"}>{detail.eligible ? "符合資格" : "不可報名"}</StatusBadge>
              <StatusBadge tone={eventStatusTone(detail.status)}>{detail.status}</StatusBadge>
              {detail.current_user_status && <StatusBadge tone={registrationTone(detail.current_user_status)}>{detail.current_user_status}</StatusBadge>}
            </div>
            <h2>{detail.title}</h2>
            <p>{detail.description || "此活動尚未填寫描述。"}</p>
            <dl className="meta-list">
              <div>
                <dt>地點</dt>
                <dd>{detail.location || "未設定"}</dd>
              </div>
              <div>
                <dt>開始時間</dt>
                <dd>{formatDate(detail.starts_at)}</dd>
              </div>
              <div>
                <dt>報名期間</dt>
                <dd>
                  {formatDate(detail.registration_start)} 到 {formatDate(detail.registration_close)}
                </dd>
              </div>
              <div>
                <dt>資格原因</dt>
                <dd>{detail.eligibility_reason || "符合資格"}</dd>
              </div>
              <div>
                <dt>規則</dt>
                <dd>
                  {detail.rule.department} / {detail.rule.site} / G{detail.rule.min_grade}+
                </dd>
              </div>
              <div>
                <dt>Event ID</dt>
                <dd>{detail.event_id}</dd>
              </div>
            </dl>
            <ProgressMeter
              label="容量使用"
              max={detail.capacity}
              value={detail.confirmed_count}
              helper={`${detail.confirmed_count}/${detail.capacity} confirmed，候補 ${detail.waitlist_count}`}
            />
          </div>
        )}
      </div>
      <div className="panel span-4">
        <h2>報名決策</h2>
        {!detail && <EmptyState title="等待活動" action="選擇活動後會顯示可執行動作。" />}
        {detail && (
          <div className="summary-block">
            <Kpi label="剩餘名額" value={detail.remaining_capacity} />
            <Kpi label="目前狀態" value={detail.current_user_status || "none"} />
            <button
              className="button full-width"
              type="button"
              onClick={() => void bookSelected()}
              disabled={!detail.eligible || detail.current_user_status === "confirmed" || detail.current_user_status === "waitlisted"}
            >
              <Icon name="ticket" />
              {bookingActionLabel(detail)}
            </button>
            {detail.current_user_ticket && <TicketPanel compact ticket={detail.current_user_ticket} />}
          </div>
        )}
      </div>
    </section>
  );
}
