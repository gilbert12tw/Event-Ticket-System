import { useEffect, useState } from "react";
import { cancelRegistration, listAdminEvents, listRegistrations, promoteWaitlist, revokeTicket } from "@/lib/api";
import type { EventSummary, RegistrationDetail, Ticket } from "@/lib/api";
import { errorMessage, formatDate, registrationTone } from "@/lib/formatting";
import { Alert, EmptyState, Field, Kpi, ResponsiveTable, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";

export function AdminRegistrationsPage() {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [eventID, setEventID] = useState("");
  const [rows, setRows] = useState<RegistrationDetail[]>([]);
  const [reason, setReason] = useState("admin action");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const selectedEvent = events.find((event) => event.event_id === eventID);

  async function refresh(nextEventID = eventID) {
    setBusy(true);
    setMessage("");
    try {
      const nextEvents = await listAdminEvents();
      setEvents(nextEvents);
      const nextID = nextEventID || nextEvents[0]?.event_id || "";
      setEventID(nextID);
      setRows(nextID ? await listRegistrations(nextID) : []);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function promote() {
    if (!eventID) return;
    setBusy(true);
    setMessage("");
    try {
      const result = await promoteWaitlist(eventID);
      setMessage(result.message);
      await refresh(eventID);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  async function cancel(row: RegistrationDetail) {
    setBusy(true);
    setMessage("");
    try {
      const result = await cancelRegistration(row.event_id, row.registration_id, reason, `cancel-${row.registration_id}`);
      setMessage(result.message);
      await refresh(row.event_id);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  async function revoke(ticket: Ticket) {
    setBusy(true);
    setMessage("");
    try {
      await revokeTicket(ticket.ticket_id, reason);
      setMessage("票券已撤銷。");
      await refresh(ticket.event_id);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  const stats = {
    confirmed: rows.filter((row) => row.status === "confirmed").length,
    waitlisted: rows.filter((row) => row.status === "waitlisted").length,
    cancelled: rows.filter((row) => row.status === "cancelled").length,
    tickets: rows.filter((row) => row.ticket).length
  };

  return (
    <section className="content-grid">
      <div className="panel span-12 workspace-context admin-context">
        <div>
          <div className="eyebrow">Admin Console</div>
          <h2>報名治理入口</h2>
          <p>針對單一活動檢視 registration detail，執行候補提升、取消報名與撤銷票券。</p>
        </div>
        <label className="field compact">
          <span>活動</span>
          <select value={eventID} onChange={(event) => void refresh(event.target.value)} disabled={busy}>
            <option value="">選擇活動</option>
            {events.map((event) => (
              <option value={event.event_id} key={event.event_id}>
                {event.title}
              </option>
            ))}
          </select>
        </label>
      </div>
      <div className="panel span-12">
        <div className="section-heading">
          <div>
            <h2>{selectedEvent?.title || "Registration detail"}</h2>
            <p>後端仍以交易、唯一約束與 audit log 保證取消、撤銷與候補提升的一致性。</p>
          </div>
          <div className="toolbar">
            <Field label="原因" value={reason} onChange={setReason} required />
            <button className="button secondary" type="button" onClick={() => void refresh()} disabled={busy}>
              <Icon name="refresh" />
              重新整理
            </button>
            <button className="button" type="button" onClick={() => void promote()} disabled={busy || !eventID}>
              <Icon name="users" />
              提升候補
            </button>
          </div>
        </div>
        {message && <Alert tone={message.includes("cancel") || message.includes("撤銷") ? "warn" : "info"}>{message}</Alert>}
        <div className="kpi-row four">
          <Kpi label="Confirmed" value={stats.confirmed} />
          <Kpi label="Waitlist" value={stats.waitlisted} />
          <Kpi label="Cancelled" value={stats.cancelled} />
          <Kpi label="Tickets" value={stats.tickets} />
        </div>
        <ResponsiveTable>
          <thead>
            <tr>
              <th>員工</th>
              <th>Registration</th>
              <th>狀態</th>
              <th>Ticket</th>
              <th>建立時間</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.registration_id}>
                <td>
                  {row.employee_name || row.employee_id}
                  <span className="table-muted">{row.employee_id}</span>
                </td>
                <td className="mono-cell">{row.registration_id}</td>
                <td>
                  <StatusBadge tone={registrationTone(row.status)}>{row.status}</StatusBadge>
                  {row.cancel_reason && <span className="table-muted">{row.cancel_reason}</span>}
                </td>
                <td>
                  {row.ticket ? (
                    <>
                      <StatusBadge tone={row.ticket.status === "active" ? "ok" : "neutral"}>{row.ticket.status}</StatusBadge>
                      <span className="table-muted">{row.ticket.ticket_id}</span>
                    </>
                  ) : (
                    <span className="table-muted">no ticket</span>
                  )}
                </td>
                <td>{formatDate(row.created_at)}</td>
                <td>
                  <div className="row-actions">
                    <button className="button secondary compact-button" type="button" onClick={() => void cancel(row)} disabled={busy || row.status === "cancelled"}>
                      取消
                    </button>
                    <button
                      className="button secondary compact-button"
                      type="button"
                      onClick={() => row.ticket && void revoke(row.ticket)}
                      disabled={busy || !row.ticket || row.ticket.status !== "active"}
                    >
                      撤銷票券
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </ResponsiveTable>
        {rows.length === 0 && <EmptyState title="尚無報名資料" action="選擇已有報名的活動，或先執行 Demo Runbook。" />}
      </div>
    </section>
  );
}
