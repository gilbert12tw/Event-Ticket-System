import { useEffect, useRef, useState } from "react";
import type { MouseEvent } from "react";
import { getTicket, listTickets } from "@/lib/api";
import type { AuthMeClaims, Ticket } from "@/lib/api";
import { navigate, ticketDetailPath } from "@/app/routes";
import { errorMessage } from "@/lib/formatting";
import { runClientNavigation } from "@/lib/navigation";
import { Alert, EmptyState, SkeletonRows } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  EmployeeTicketEntryHint,
  EmployeeTicketPass,
  EmployeeTicketTimeline,
} from "./employee-ticket-components";
import { groupEmployeeTicketsByDate } from "./employee-ticket-surface";
import { selectCurrentTicket } from "./ticket-readiness";

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
  const currentTicket = selectCurrentTicket(tickets);
  const ticketGroups = groupEmployeeTicketsByDate(tickets);

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
            <h2 className="sr-only" ref={detailHeadingRef} tabIndex={-1}>
              票券詳細
            </h2>
            <Button
              asChild
              aria-label="返回我的票券"
              className="ticket-detail-back-button"
              size="icon"
              title="返回我的票券"
              variant="ghost"
            >
              <a
                href="/user/tickets"
                onClick={(event) => runClientNavigation(event, returnToList)}
              >
                <Icon name="chevronLeft" />
                <span className="sr-only">返回我的票券</span>
              </a>
            </Button>
          </div>
          {detailMessage && <Alert tone="warn">{detailMessage}</Alert>}
          {detailLoading && <SkeletonRows rows={3} />}
          {!detailLoading && detailTicket && (
            <EmployeeTicketPass
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
          <h2 className="sr-only" ref={listHeadingRef} tabIndex={-1}>
            我的票券清單
          </h2>
          <Button
            aria-label="重新整理票券"
            disabled={loading}
            size="icon"
            title="重新整理票券"
            type="button"
            variant="ghost"
            onClick={() => void refreshList()}
          >
            <Icon name="refresh" />
          </Button>
        </div>
        {message && <Alert tone="warn">{message}</Alert>}
        {listLoaded && currentTicket && (
          <EmployeeTicketPass
            ticket={currentTicket}
            onRefresh={() => void refreshList()}
          />
        )}
        {listLoaded && !currentTicket && (
          <EmployeeTicketEntryHint tickets={tickets} />
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
            <div className="ticket-empty-state">
              <EmptyState
                title="尚無票券"
                action="完成報名確認後，票券會出現在這裡。"
              />
              <BrowseEventsAction />
            </div>
          )}
          {!loading && tickets.length > 0 && (
            <EmployeeTicketTimeline groups={ticketGroups} onOpen={openTicket} />
          )}
        </div>
      </Card>
    </section>
  );
}

function ticketIDFromLocation() {
  return new URLSearchParams(globalThis.location.search).get("ticket_id") || "";
}

function BrowseEventsAction() {
  return (
    <Button asChild variant="outline">
      <a
        href="/user/events"
        onClick={(event) =>
          runClientNavigation(event, () => navigate("/user/events"))
        }
      >
        <Icon name="calendar" />
        瀏覽活動
      </a>
    </Button>
  );
}
