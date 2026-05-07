import { useEffect, useState } from "react";
import { listNotificationDeliveries, listEvents, listTickets, retryNotificationDelivery, type NotificationDelivery } from "@/lib/api";
import type { EventSummary, Ticket } from "@/lib/api";
import { registrationTone } from "@/lib/formatting";
import { BoundaryContext, EmptyState, Kpi, ResponsiveTable, StatusBadge } from "@/components/shared";

export function UserNotificationsPage({ employeeID }: { employeeID: string }) {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [tickets, setTickets] = useState<Ticket[]>([]);

  useEffect(() => {
    listEvents(employeeID).then(setEvents).catch(() => setEvents([]));
    listTickets(employeeID).then(setTickets).catch(() => setTickets([]));
  }, [employeeID]);

  const notificationRows = [
    ...events
      .filter((event) => event.current_user_status)
      .map((event) => ({
        id: `registration-${event.event_id}`,
        kind: "Registration",
        title: event.title,
        status: event.current_user_status,
        detail: event.current_user_status === "waitlisted" ? "候補名單會由主辦在有空位時提升。" : "報名狀態已更新。"
      })),
    ...tickets.map((ticket) => ({
      id: ticket.ticket_id,
      kind: "Ticket",
      title: ticket.event_title || ticket.event_id,
      status: ticket.status,
      detail: ticket.status === "active" ? "票券已核發，可於現場驗票使用。" : `票券狀態：${ticket.status}`
    }))
  ];

  return (
    <section className="content-grid">
      <div className="panel span-12 workspace-context user-context">
        <div>
          <div className="eyebrow">User Workspace</div>
          <h2>通知中心</h2>
          <p>Phase 1 先以目前報名與票券狀態呈現通知，占位保留 Email 與站內通知投遞紀錄。</p>
        </div>
        <div className="context-kpis">
          <Kpi label="通知候選" value={notificationRows.length} />
          <Kpi label="候補提醒" value={events.filter((event) => event.current_user_status === "waitlisted").length} />
          <Kpi label="票券提醒" value={tickets.length} />
        </div>
      </div>
      <div className="panel span-12">
        <ResponsiveTable>
          <thead>
            <tr>
              <th>類型</th>
              <th>主旨</th>
              <th>狀態</th>
              <th>說明</th>
            </tr>
          </thead>
          <tbody>
            {notificationRows.map((row) => (
              <tr key={row.id}>
                <td>{row.kind}</td>
                <td>{row.title}</td>
                <td>
                  <StatusBadge tone={registrationTone(row.status)}>{row.status}</StatusBadge>
                </td>
                <td>{row.detail}</td>
              </tr>
            ))}
          </tbody>
        </ResponsiveTable>
        {notificationRows.length === 0 && <EmptyState title="尚無通知" action="完成報名或取得票券後，這裡會顯示可投遞的使用者通知。" />}
      </div>
    </section>
  );
}

export function NotificationDeliveryPage() {
  const [deliveries, setDeliveries] = useState<NotificationDelivery[]>([]);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [retrying, setRetrying] = useState("");

  async function refresh() {
    setLoading(true);
    setMessage("");
    try {
      setDeliveries(await listNotificationDeliveries());
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "載入投遞紀錄失敗。");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function retry(row: NotificationDelivery) {
    setRetrying(row.delivery_id);
    setMessage("");
    try {
      const updated = await retryNotificationDelivery(row.delivery_id);
      setDeliveries((current) => current.map((candidate) => (candidate.delivery_id === row.delivery_id ? updated : candidate)));
      setMessage(`已重試投遞 ${row.delivery_id}。`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "重試投遞失敗。");
    } finally {
      setRetrying("");
    }
  }

  const filteredDeliveries = deliveries.filter((delivery) => statusFilter === "all" || delivery.status === statusFilter);
  const deliveryStats = {
    total: deliveries.length,
    failed: deliveries.filter((delivery) => delivery.status === "failed").length,
    deadLetter: deliveries.filter((delivery) => delivery.status === "dead_letter").length,
    pending: deliveries.filter((delivery) => delivery.status === "pending").length
  };

  const visibleStatuses = ["all", "pending", "failed", "dead_letter", "sent", "suppressed"];

  return (
    <section className="content-grid">
      <BoundaryContext
        title="通知投遞"
        description="以後端實際 outbox/worker 投遞紀錄作為真實通知狀態來源，提供重試邊界與失敗紀錄稽核點。"
        icon="send"
      />
      <div className="panel span-12">
        <div className="section-heading">
          <div>
            <h2>Delivery log</h2>
            <p>從 Admin endpoint 讀取投遞紀錄，並支援對失敗與 dead-letter 事件重新嘗試。</p>
          </div>
          <div className="toolbar">
            <label className="field compact">
              <span>狀態</span>
              <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
                {visibleStatuses.map((status) => (
                  <option key={status} value={status}>
                    {status}
                  </option>
                ))}
              </select>
            </label>
            <button className="button secondary" type="button" onClick={() => void refresh()} disabled={loading}>
              重新整理
            </button>
          </div>
        </div>
        {message && <div className="form-hint">{message}</div>}
        <div className="kpi-row four">
          <Kpi label="總筆數" value={deliveryStats.total} />
          <Kpi label="待處理" value={deliveryStats.pending} />
          <Kpi label="重試失敗" value={deliveryStats.failed} />
          <Kpi label="Dead letter" value={deliveryStats.deadLetter} />
        </div>
        <div className="status-selectors">
          {visibleStatuses.map((status) => (
            <label key={status} className="field compact">
              <input
                type="radio"
                name="delivery-status"
                checked={statusFilter === status}
                onChange={() => setStatusFilter(status)}
              />
              <span>{status}</span>
            </label>
          ))}
        </div>
        <ResponsiveTable>
          <thead>
            <tr>
              <th>Delivery</th>
              <th>員工</th>
              <th>Channel</th>
              <th>狀態</th>
              <th>嘗試次數</th>
              <th>最後錯誤</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {filteredDeliveries.map((row) => (
              <tr key={row.delivery_id}>
                <td className="mono-cell">{row.delivery_id}</td>
                <td>{row.employee_id || "system"}</td>
                <td>{row.channel}</td>
                <td>
                  <StatusBadge tone={deliveryTone(row.status)}>{row.status}</StatusBadge>
                </td>
                <td>{row.attempts}</td>
                <td>{row.last_error || "—"}</td>
                <td>
                  <button
                    className="button secondary compact-button"
                    type="button"
                    disabled={loading || !canRetryDelivery(row.status) || retrying === row.delivery_id}
                    onClick={() => void retry(row)}
                  >
                    {retrying === row.delivery_id ? "重試中" : "Retry"}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </ResponsiveTable>
        {!loading && filteredDeliveries.length === 0 && <EmptyState title="尚無投遞紀錄" action="完成報名或觸發通知事件後，會出現 worker 投遞結果。" />}
      </div>
    </section>
  );
}

function deliveryTone(status: string): "ok" | "warn" | "fail" | "info" | "neutral" {
  if (status === "sent") return "ok";
  if (status === "pending") return "warn";
  if (status === "failed" || status === "dead_letter") return "fail";
  if (status === "suppressed") return "info";
  return "neutral";
}

function canRetryDelivery(status: string): boolean {
  return status === "failed" || status === "dead_letter" || status === "pending";
}
