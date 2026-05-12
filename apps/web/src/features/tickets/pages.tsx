import { useEffect, useState } from "react";
import { listTickets } from "@/lib/api";
import type { Ticket } from "@/lib/api";
import { navigate } from "@/app/routes";
import { errorMessage, formatDate } from "@/lib/formatting";
import { Alert, EmptyState, IdentityCard, Kpi, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { checkinHistoryState } from "@/features/checkin/checkin-token";
import { TicketQrCode } from "./qr";

export function EmployeeTicketsPage({ principalID }: { principalID: string }) {
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [message, setMessage] = useState("");

  const selectedTicket = tickets.find((ticket) => ticket.ticket_id === selectedID) || tickets[0];

  async function refresh() {
    setMessage("");
    try {
      const rows = await listTickets();
      setTickets(rows);
      setSelectedID(rows[0]?.ticket_id || "");
    } catch (error) {
      setMessage(errorMessage(error));
    }
  }

  useEffect(() => {
    void refresh();
  }, [principalID]);

  return (
    <section className="content-grid">
      <div className="panel span-12 workspace-context user-context">
        <div>
          <div className="eyebrow">User Workspace</div>
          <h2>票券入口</h2>
          <p>員工只看自己的票券狀態與 QR 入場畫面；signed token 不在畫面或 API activity 中裸露。</p>
        </div>
        <IdentityCard principalID={principalID} />
        <div className="context-kpis">
          <Kpi label="票券數" value={tickets.length} />
          <Kpi label="可使用" value={tickets.filter((ticket) => ticket.status === "active").length} />
          <Kpi label="已核銷" value={tickets.filter((ticket) => ticket.status !== "active").length} />
          <Kpi label="QR 可用" value={tickets.filter((ticket) => ticket.qr_payload || ticket.signed_token).length} />
        </div>
      </div>
      <div className="panel span-5">
        <div className="section-heading">
          <div>
            <h2>票券清單</h2>
            <p>票券 token 不直接顯示，QR 區塊保留給現場掃描。</p>
          </div>
        </div>
        {message && <Alert tone="warn">{message}</Alert>}
        <div className="list-stack">
          {tickets.length === 0 && <EmptyState title="尚未取得票券" action="先到員工活動頁完成報名，或執行 Demo Runbook。" />}
          {tickets.map((ticket) => (
            <button
              className={ticket.ticket_id === selectedTicket?.ticket_id ? "ticket-row active" : "ticket-row"}
              key={ticket.ticket_id}
              type="button"
              onClick={() => setSelectedID(ticket.ticket_id)}
            >
              <span>
                <strong>{ticket.event_title || ticket.event_id}</strong>
                <small>
                  {ticket.ticket_id} · {formatDate(ticket.issued_at)}
                </small>
              </span>
              <StatusBadge tone={ticket.status === "active" ? "ok" : "neutral"}>{ticket.status}</StatusBadge>
            </button>
          ))}
        </div>
      </div>
      <div className="panel span-7">
        <TicketPanel ticket={selectedTicket} />
      </div>
    </section>
  );
}

export function TicketPanel({ compact = false, ticket }: { compact?: boolean; ticket?: Ticket }) {
  if (!ticket) {
    return <EmptyState title="沒有可顯示的票券" action="完成 confirmed booking 後，QR 票券會出現在這裡。" />;
  }
  return (
    <div className={compact ? "ticket-panel compact" : "ticket-panel"}>
      <div className="qr-wrap">
        <TicketQrCode token={ticket.qr_payload || ticket.signed_token || ""} />
      </div>
      <div className="ticket-detail">
        <StatusBadge tone={ticket.status === "active" ? "ok" : "neutral"}>{ticket.status}</StatusBadge>
        <h2>{ticket.event_title || ticket.event_id}</h2>
        <div className="token-safety">
          <StatusBadge tone={ticket.qr_payload || ticket.signed_token ? "ok" : "warn"}>QR</StatusBadge>
          <span>Signed token 已保留給驗票流程，API activity 只顯示 redacted payload。</span>
        </div>
        <dl className="meta-list vertical">
          <div>
            <dt>持票人</dt>
            <dd>{ticket.employee_name || ticket.employee_id}</dd>
          </div>
          <div>
            <dt>地點</dt>
            <dd>{ticket.event_location || "未設定"}</dd>
          </div>
          <div>
            <dt>開始時間</dt>
            <dd>{formatDate(ticket.event_starts_at)}</dd>
          </div>
          <div>
            <dt>Ticket ID</dt>
            <dd>{ticket.ticket_id}</dd>
          </div>
          <div>
            <dt>Issued at</dt>
            <dd>{formatDate(ticket.issued_at)}</dd>
          </div>
        </dl>
        <button
          className="button secondary"
          type="button"
          onClick={() => {
            navigate("/admin/checkin", ticket.signed_token ? checkinHistoryState(ticket.signed_token) : undefined);
          }}
        >
          <Icon name="scan" />
          帶到驗票頁
        </button>
      </div>
    </div>
  );
}
