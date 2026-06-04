import type { ReactNode } from "react";
import { navigate } from "@/app/routes";
import { EmptyState, MetaList, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";
import type { Ticket } from "@/lib/api";
import { formatDate } from "@/lib/formatting";
import { runClientNavigation } from "@/lib/navigation";
import { siteLabel, ticketStatusView } from "@/lib/ui/options";
import { TicketQrCode } from "./qr";
import { ticketEntryReadinessView } from "./ticket-readiness";

export function TicketPanel({
  compact = false,
  onRefresh,
  ticket,
}: Readonly<{
  compact?: boolean;
  onRefresh?: () => void;
  ticket?: Ticket;
}>) {
  if (!ticket) {
    return (
      <EmptyState
        title="沒有可顯示的票券"
        action="完成報名確認後，二維碼票券會出現在這裡。"
      />
    );
  }
  const qrToken = ticket.qr_payload || ticket.signed_token || "";
  const readiness = ticketEntryReadinessView(ticket);
  const statusView = ticketStatusView(ticket.status);
  const canShowQr = Boolean(qrToken) && readiness.kind === "entry-ready";
  const eventDetailHref = `/user/events/detail?event_id=${encodeURIComponent(
    ticket.event_id,
  )}`;
  const panelClassName = [
    "ticket-panel",
    compact ? "compact" : "",
    canShowQr ? "" : "unavailable",
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <div className={panelClassName}>
      <div className="ticket-detail">
        <StatusBadge tone={statusView.tone}>{statusView.label}</StatusBadge>
        <h2>
          {ticket.event_title || (compact ? "活動票券" : ticket.event_id)}
        </h2>
        <div className="helper-strip">
          <StatusBadge tone={readiness.tone}>{readiness.label}</StatusBadge>
          <span>{readiness.copy}</span>
          {readiness.kind === "qr-pending" && onRefresh && (
            <Button variant="outline" type="button" onClick={onRefresh}>
              <Icon name="refresh" />
              重新整理票券
            </Button>
          )}
        </div>
        <div className="ticket-nontransferable" role="note">
          <StatusBadge tone="warn">不可轉讓</StatusBadge>
          <span>此票券綁定持票員工本人，入場時驗票員會核對持票人身份。</span>
        </div>
        {canShowQr && (
          <>
            <div className="ticket-entry-instruction" role="note">
              <StatusBadge tone="ok">入場提示</StatusBadge>
              <span>
                請在入口出示此 QR code，驗票員完成核銷後票券會更新為已核銷。
              </span>
            </div>
            <div className="qr-wrap">
              <TicketQrCode token={qrToken} />
            </div>
          </>
        )}
        <MetaList
          className="ticket-meta-list"
          rows={
            compact ? compactTicketMetaRows(ticket) : ticketMetaRows(ticket)
          }
        />
        <Button asChild variant="outline">
          <a
            href={eventDetailHref}
            onClick={(event) =>
              runClientNavigation(event, () => navigate(eventDetailHref))
            }
          >
            <Icon name="audit" />
            查看活動詳情
          </a>
        </Button>
      </div>
    </div>
  );
}

function ticketMetaRows(ticket: Ticket): Array<[string, ReactNode]> {
  const rows: Array<[string, ReactNode]> = [
    [
      "持票人",
      <>
        {ticket.employee_name || ticket.employee_id}
        <span className="table-muted">{ticket.employee_id}</span>
      </>,
    ],
    [
      "部門 / 城市",
      <>
        {ticket.department || "未提供"}
        <span className="table-muted">{ticket.city || "未提供"}</span>
      </>,
    ],
    [
      "同行人數",
      <>
        {ticket.family_count ?? 0} 人
        <span className="table-muted">僅供入場人數核對，非可轉讓票券。</span>
      </>,
    ],
    ["地點", siteLabel(ticket.event_location)],
    ["開始時間", formatDate(ticket.event_starts_at)],
    ["票券編號", ticket.ticket_id],
    ["發行時間", formatDate(ticket.issued_at)],
  ];
  if (ticket.expires_at) rows.push(["有效期限", formatDate(ticket.expires_at)]);
  if (ticket.revoked_reason) rows.push(["撤銷原因", ticket.revoked_reason]);
  return rows;
}

function compactTicketMetaRows(ticket: Ticket): Array<[string, ReactNode]> {
  return [
    ["地點", siteLabel(ticket.event_location)],
    ["開始時間", formatDate(ticket.event_starts_at)],
    [
      "同行人數",
      <>
        {ticket.family_count ?? 0} 人
        <span className="table-muted">入場時核對人數。</span>
      </>,
    ],
  ];
}
