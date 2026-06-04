import type { MouseEvent } from "react";
import { ticketDetailPath } from "@/app/routes";
import { EmptyState, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";
import { EventPoster } from "@/features/events/employee-event-poster";
import type { Ticket } from "@/lib/api";
import {
  type EmployeeTicketDateGroup,
  employeeTicketSurfaceState,
  shouldShowCompanionCount,
  ticketCalendarExport,
  ticketTimeLocation,
} from "./employee-ticket-surface";
import { TicketQrCode } from "./qr";

export function EmployeeTicketPass({
  onRefresh,
  ticket,
}: Readonly<{
  onRefresh?: () => void;
  ticket: Ticket;
}>) {
  const state = employeeTicketSurfaceState(ticket);
  const qrToken = ticket.qr_payload || ticket.signed_token || "";
  const canShowQr = Boolean(qrToken) && state.canShowQr;
  return (
    <section className="employee-ticket-pass" aria-label="目前可入場票券">
      <div className="employee-ticket-pass-qr">
        {canShowQr ? (
          <TicketQrCode token={qrToken} />
        ) : (
          <div className="employee-ticket-qr-placeholder">
            <Icon name="ticket" />
            <span>{state.label}</span>
          </div>
        )}
      </div>
      <div className="employee-ticket-pass-info">
        <div className="employee-ticket-pass-poster">
          <EventPoster
            eventID={ticket.event_id}
            title={ticket.event_title || "活動票券"}
          />
        </div>
        <div className="employee-ticket-pass-main">
          <div className="employee-ticket-pass-badges">
            <StatusBadge tone={state.tone}>{state.label}</StatusBadge>
            {shouldShowCompanionCount(ticket) && (
              <StatusBadge tone="info">
                同行 {ticket.family_count} 人
              </StatusBadge>
            )}
          </div>
          <h3>{ticket.event_title || "活動票券"}</h3>
          <p>{ticketTimeLocation(ticket)}</p>
          <strong>{state.copy}</strong>
          <div className="employee-ticket-pass-actions">
            {state.canAddToCalendar && (
              <EmployeeTicketAddToCalendarButton ticket={ticket} />
            )}
            {state.kind === "qr-pending" && onRefresh && (
              <Button variant="outline" type="button" onClick={onRefresh}>
                <Icon name="refresh" />
                更新票券
              </Button>
            )}
          </div>
          <TicketUsageDisclosure />
        </div>
      </div>
    </section>
  );
}

export function EmployeeTicketEntryHint({
  tickets,
}: Readonly<{ tickets: Ticket[] }>) {
  if (tickets.length === 0) return null;
  return (
    <div className="employee-ticket-entry-hint">
      <Icon name="ticket" />
      <div>
        <strong>目前沒有可入場票券</strong>
        <span>活動開始後，可入場 QR code 會出現在這裡。</span>
      </div>
    </div>
  );
}

export function EmployeeTicketTimeline({
  groups,
  onOpen,
}: Readonly<{
  groups: EmployeeTicketDateGroup[];
  onOpen: (event: MouseEvent<HTMLAnchorElement>, ticketID: string) => void;
}>) {
  if (groups.length === 0) {
    return (
      <EmptyState
        title="尚無票券"
        action="完成報名確認後，票券會出現在這裡。"
      />
    );
  }
  return (
    <div className="employee-ticket-timeline" aria-label="票券時間軸">
      {groups.map((group) => (
        <section className="employee-ticket-date-group" key={group.dateKey}>
          <h3>{group.label}</h3>
          <div className="employee-ticket-card-list">
            {group.tickets.map((ticket) => (
              <EmployeeTicketCard
                key={ticket.ticket_id}
                ticket={ticket}
                onOpen={onOpen}
              />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function EmployeeTicketCard({
  onOpen,
  ticket,
}: Readonly<{
  onOpen: (event: MouseEvent<HTMLAnchorElement>, ticketID: string) => void;
  ticket: Ticket;
}>) {
  const state = employeeTicketSurfaceState(ticket);
  return (
    <article className="employee-ticket-card">
      <a
        className="employee-ticket-card-link"
        href={ticketDetailPath(ticket.ticket_id)}
        onClick={(event) => onOpen(event, ticket.ticket_id)}
      >
        <EventPoster
          eventID={ticket.event_id}
          title={ticket.event_title || "活動票券"}
        />
        <span className="employee-ticket-card-copy">
          <span className="employee-ticket-card-badges">
            <StatusBadge tone={state.tone}>{state.label}</StatusBadge>
            {shouldShowCompanionCount(ticket) && (
              <StatusBadge tone="info">
                同行 {ticket.family_count} 人
              </StatusBadge>
            )}
          </span>
          <strong>{ticket.event_title || "活動票券"}</strong>
          <small>{ticketTimeLocation(ticket)}</small>
          <span>{state.actionLabel}</span>
        </span>
      </a>
      {state.canAddToCalendar && (
        <EmployeeTicketAddToCalendarButton ticket={ticket} />
      )}
    </article>
  );
}

function EmployeeTicketAddToCalendarButton({
  ticket,
}: Readonly<{ ticket: Ticket }>) {
  return (
    <Button
      aria-label={`加入行事曆：${ticket.event_title || "活動票券"}`}
      className="employee-ticket-calendar-button"
      size="icon-sm"
      title={`加入行事曆：${ticket.event_title || "活動票券"}`}
      type="button"
      variant="outline"
      onClick={() => downloadTicketCalendar(ticket)}
    >
      <Icon name="calendarPlus" />
    </Button>
  );
}

function TicketUsageDisclosure() {
  return (
    <details className="ticket-usage-disclosure">
      <summary>票券使用說明</summary>
      <p>票券只限持票員工本人使用，請勿截圖轉傳。</p>
      <p>驗票完成後，票券會更新為已使用。</p>
    </details>
  );
}

function downloadTicketCalendar(ticket: Ticket) {
  const artifact = ticketCalendarExport(ticket);
  const blob = new Blob([artifact.content], { type: artifact.mimeType });
  const url = globalThis.URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = artifact.filename;
  document.body.append(link);
  link.click();
  link.remove();
  globalThis.URL.revokeObjectURL(url);
}
