import { useEffect, useRef, useState } from "react";
import type { MouseEvent } from "react";
import { getTicket, listTickets } from "@/lib/api";
import type { AuthMeClaims, Ticket } from "@/lib/api";
import { navigate, ticketDetailPath } from "@/app/routes";
import { errorMessage } from "@/lib/formatting";
import { runClientNavigation } from "@/lib/navigation";
import {
  Alert,
  CompactStatsBar,
  EmptyState,
  SkeletonRows,
  StatusBadge,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { ticketStatusView } from "@/lib/ui/options";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  safeTicketID,
  selectCurrentTicket,
  ticketEntryReadinessView,
  ticketListMeta,
} from "./ticket-readiness";
import { TicketPanel } from "./ticket-panel";

export { TicketPanel } from "./ticket-panel";

export function EmployeeTicketsPage({
  claims,
}: Readonly<{ claims: AuthMeClaims }>) {
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [detailID, setDetailID] = useState(ticketIDFromLocation);
  const [detailTicket, setDetailTicket] = useState<Ticket | null>(null);
  const [message, setMessage] = useState("");
  const [detailMessage, setDetailMessage] = useState("");
  const [loading, setLoading] = useState(false);
  const [listLoaded, setListLoaded] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const detailHeadingRef = useRef<HTMLHeadingElement>(null);
  const listHeadingRef = useRef<HTMLHeadingElement>(null);
  const pendingListFocusRef = useRef(false);

  const principalID = claims.employee_id;
  const readinessCounts = countTicketReadiness(tickets);
  const currentTicket = selectCurrentTicket(tickets);

  async function refreshList() {
    setMessage("");
    setLoading(true);
    try {
      const rows = await listTickets();
      setTickets(rows);
      setListLoaded(true);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  async function refreshDetail(ticketID = detailID) {
    if (!ticketID) return;
    setDetailLoading(true);
    setDetailMessage("");
    setDetailTicket(null);
    try {
      const ticket = await getTicket(ticketID);
      if (ticket.ticket_id !== ticketID) {
        setDetailMessage("票券資料不一致，請返回清單後重新開啟。");
        return;
      }
      setDetailTicket(ticket);
    } catch (error) {
      setDetailMessage(errorMessage(error));
    } finally {
      setDetailLoading(false);
    }
  }

  useEffect(() => {
    const syncTicketID = () => setDetailID(ticketIDFromLocation());
    globalThis.addEventListener("popstate", syncTicketID);
    return () => globalThis.removeEventListener("popstate", syncTicketID);
  }, []);

  useEffect(() => {
    if (detailID) return;
    void refreshList();
  }, [detailID, principalID]);

  useEffect(() => {
    if (!detailID) {
      setDetailTicket(null);
      setDetailMessage("");
      if (pendingListFocusRef.current) {
        pendingListFocusRef.current = false;
        globalThis.setTimeout(() => listHeadingRef.current?.focus(), 0);
      }
      return;
    }
    let active = true;
    setDetailLoading(true);
    setDetailMessage("");
    setDetailTicket(null);
    getTicket(detailID)
      .then((ticket) => {
        if (!active) return;
        if (ticket.ticket_id !== detailID) {
          setDetailMessage("票券資料不一致，請返回清單後重新開啟。");
          return;
        }
        setDetailTicket(ticket);
      })
      .catch((error) => {
        if (!active) return;
        setDetailMessage(errorMessage(error));
      })
      .finally(() => {
        if (active) setDetailLoading(false);
      });
    return () => {
      active = false;
    };
  }, [detailID, principalID]);

  useEffect(() => {
    if (!detailID) return;
    globalThis.setTimeout(() => detailHeadingRef.current?.focus(), 0);
  }, [detailID]);

  function openTicket(event: MouseEvent<HTMLAnchorElement>, ticketID: string) {
    runClientNavigation(event, () => {
      setDetailID(ticketID);
      navigate(ticketDetailPath(ticketID));
    });
  }

  function returnToList() {
    pendingListFocusRef.current = true;
    setDetailID("");
    navigate("/user/tickets");
  }

  if (detailID) {
    return (
      <section className="content-grid ticket-detail-workspace">
        <Card
          className="panel span-12 ticket-detail-panel"
          aria-busy={detailLoading}
        >
          <div className="section-heading ticket-detail-heading">
            <div>
              <h2 ref={detailHeadingRef} tabIndex={-1}>
                票券詳細
              </h2>
              <p>入場二維碼只在票券詳細頁顯示；票券簽章碼不直接顯示。</p>
            </div>
            <Button asChild variant="outline">
              <a
                href="/user/tickets"
                onClick={(event) => runClientNavigation(event, returnToList)}
              >
                返回我的票券
              </a>
            </Button>
          </div>
          {detailMessage && <Alert tone="warn">{detailMessage}</Alert>}
          {detailLoading && <SkeletonRows rows={3} />}
          {!detailLoading && detailTicket && (
            <TicketPanel
              ticket={detailTicket}
              onRefresh={() => void refreshDetail()}
            />
          )}
          {!detailLoading && !detailTicket && !detailMessage && (
            <EmptyState
              title="找不到票券"
              action="請返回我的票券清單，或稍後重新整理。"
            />
          )}
        </Card>
      </section>
    );
  }

  return (
    <section className="content-grid ticket-workspace">
      <Card className="panel span-12 ticket-list-panel">
        <div className="section-heading">
          <div>
            <h2 ref={listHeadingRef} tabIndex={-1}>
              我的票券
            </h2>
            <p>目前可入場票券會直接顯示 QR code，其他票券保留在清單。</p>
          </div>
          <Button
            variant="outline"
            type="button"
            onClick={() => void refreshList()}
            disabled={loading}
          >
            <Icon name="refresh" />
            重新整理
          </Button>
        </div>
        <CompactStatsBar
          items={[
            { label: "票券", value: tickets.length },
            { label: "可入場", value: readinessCounts["entry-ready"] },
            { label: "尚未開放", value: readinessCounts["not-open"] },
            { label: "待產生 QR", value: readinessCounts["qr-pending"] },
            { label: "已核銷", value: readinessCounts.redeemed },
            { label: "已撤銷", value: readinessCounts.revoked },
          ]}
          label="票券摘要"
        />
        {message && <Alert tone="warn">{message}</Alert>}
        {listLoaded && (
          <CurrentTicketPanel
            ticket={currentTicket}
            tickets={tickets}
            onRefresh={() => void refreshList()}
          />
        )}
        <div className="list-stack ticket-list" aria-busy={loading}>
          {loading && tickets.length === 0 && <SkeletonRows rows={3} />}
          {!loading && message && tickets.length === 0 && (
            <EmptyState
              title="無法載入票券"
              action="請重新整理票券清單，或稍後再試。"
            />
          )}
          {!loading && listLoaded && tickets.length === 0 && !message && (
            <EmptyState
              title="尚無票券"
              action="完成報名確認後，票券會出現在這裡。"
            />
          )}
          {tickets.map((ticket) => (
            <TicketRow
              key={ticket.ticket_id}
              ticket={ticket}
              onOpen={openTicket}
            />
          ))}
        </div>
      </Card>
    </section>
  );
}

function CurrentTicketPanel({
  onRefresh,
  ticket,
  tickets,
}: Readonly<{
  onRefresh: () => void;
  ticket?: Ticket;
  tickets: Ticket[];
}>) {
  if (ticket) {
    return (
      <div className="current-ticket-panel" aria-label="目前可入場票券">
        <div className="section-heading">
          <div>
            <h3>目前可入場票券</h3>
            <p>入口出示此 QR code 即可，不需要先進入活動詳情。</p>
          </div>
        </div>
        <TicketPanel compact ticket={ticket} onRefresh={onRefresh} />
      </div>
    );
  }

  if (tickets.length === 0) return null;
  const hasFutureTicket = tickets.some(
    (row) => ticketEntryReadinessView(row).kind === "not-open",
  );
  const action = hasFutureTicket
    ? "尚未到入場時間；活動開始後 QR code 會出現在這裡。"
    : "目前沒有可入場票券；可在下方查看過期、已核銷或待產生 QR 的票券。";
  return <EmptyState title="目前沒有可入場票券" action={action} />;
}

function TicketRow({
  onOpen,
  ticket,
}: Readonly<{
  onOpen: (event: MouseEvent<HTMLAnchorElement>, ticketID: string) => void;
  ticket: Ticket;
}>) {
  const readiness = ticketEntryReadinessView(ticket);
  const statusView = ticketStatusView(ticket.status);
  return (
    <a
      className="ticket-row"
      href={ticketDetailPath(ticket.ticket_id)}
      onClick={(event) => onOpen(event, ticket.ticket_id)}
    >
      <span className="ticket-row-main">
        <strong>{ticket.event_title || ticket.event_id}</strong>
        <small>{ticketListMeta(ticket)}</small>
        <small>{safeTicketID(ticket.ticket_id)}</small>
      </span>
      <span className="ticket-row-badges">
        <StatusBadge tone={statusView.tone}>{statusView.label}</StatusBadge>
        <StatusBadge tone={readiness.tone}>{readiness.label}</StatusBadge>
      </span>
      <span className="ticket-row-action" aria-hidden="true">
        <Icon name="arrowRight" />
      </span>
    </a>
  );
}

function countTicketReadiness(tickets: Ticket[]) {
  return tickets.reduce(
    (counts, ticket) => {
      counts[ticketEntryReadinessView(ticket).kind] += 1;
      return counts;
    },
    {
      "entry-ready": 0,
      "not-open": 0,
      "qr-pending": 0,
      expired: 0,
      redeemed: 0,
      unavailable: 0,
      revoked: 0,
    },
  );
}

function ticketIDFromLocation() {
  return new URLSearchParams(globalThis.location.search).get("ticket_id") || "";
}
