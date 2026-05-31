import { useEffect, useRef, useState } from "react";
import type { MouseEvent, ReactNode } from "react";
import { getTicket, listTickets } from "@/lib/api";
import type { AuthMeClaims, Ticket } from "@/lib/api";
import { navigate, ticketDetailPath } from "@/app/routes";
import { errorMessage, formatDate } from "@/lib/formatting";
import { runClientNavigation } from "@/lib/navigation";
import {
  Alert,
  CompactStatsBar,
  EmptyState,
  MetaList,
  SkeletonRows,
  StatusBadge,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { TicketQrCode } from "./qr";
import { siteLabel, ticketStatusView } from "@/lib/ui/options";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  safeTicketID,
  ticketEntryReadinessView,
  ticketListMeta,
} from "./ticket-readiness";

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
              票券清單
            </h2>
            <p>點選票券後才會進入詳細頁並顯示入場二維碼。</p>
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
  const canShowQr =
    Boolean(qrToken) &&
    (readiness.kind === "entry-ready" || readiness.kind === "not-open");
  const showEntryInstruction = readiness.kind === "entry-ready";
  const eventDetailHref = `/user/events/detail?event_id=${encodeURIComponent(ticket.event_id)}`;
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
        <h2>{ticket.event_title || ticket.event_id}</h2>
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
            {showEntryInstruction && (
              <div className="ticket-entry-instruction" role="note">
                <StatusBadge tone="ok">入場提示</StatusBadge>
                <span>
                  請在入口出示此 QR code，驗票員完成核銷後票券會更新為已核銷。
                </span>
              </div>
            )}
            <div className="qr-wrap">
              <TicketQrCode token={qrToken} />
            </div>
          </>
        )}
        <MetaList className="ticket-meta-list" rows={ticketMetaRows(ticket)} />
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
      redeemed: 0,
      unavailable: 0,
      revoked: 0,
    },
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

function ticketIDFromLocation() {
  return new URLSearchParams(globalThis.location.search).get("ticket_id") || "";
}
