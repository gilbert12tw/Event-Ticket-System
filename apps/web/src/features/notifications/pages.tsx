import { useEffect, useState } from "react";
import { ticketDetailPath } from "@/app/routes";
import {
  listEvents,
  listNotificationDeliveries,
  listTickets,
  retryNotificationDelivery,
  type EventSummary,
  type NotificationDelivery,
  type Ticket,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Alert,
  CompactStatsBar,
  EmptyState,
  MetaList,
  ResponsiveTable,
  SegmentedFilter,
  StatusBadge,
} from "@/components/shared";
import {
  channelLabel,
  deliveryStatusOptions,
  deliveryStatusView,
  localizedMessage,
  ticketStatusView,
} from "@/lib/ui/options";
import {
  canRetryDelivery,
  retryButtonLabel,
  retryDisabledReason,
  sortDeliveriesByAttention,
} from "./delivery-helpers";
import { RetryDeliveryDialog } from "./delivery-retry-dialog";
import {
  UserNotificationsPanel,
  type UserNotificationRow,
} from "./user-notifications-panel";

export function UserNotificationsPage() {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");

  async function refresh() {
    setLoading(true);
    setMessage("");
    try {
      const [nextEvents, nextTickets] = await Promise.all([
        listEvents(),
        listTickets(),
      ]);
      setEvents(nextEvents);
      setTickets(nextTickets);
    } catch (error) {
      setEvents([]);
      setTickets([]);
      setMessage(error instanceof Error ? error.message : "通知載入失敗。");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => void refresh(), []);

  const notificationRows: UserNotificationRow[] = [
    ...events
      .filter((event) => event.current_user_status)
      .map((event) => ({
        id: `registration-${event.event_id}`,
        href: `/user/events/detail?event_id=${encodeURIComponent(
          event.event_id,
        )}`,
        kind: "報名",
        title: event.title,
        status: event.current_user_status,
        detail:
          event.current_user_status === "waitlisted"
            ? "候補名單會由主辦在有空位時提升。"
            : "報名狀態已更新。",
      })),
    ...tickets.map((ticket) => ({
      id: ticket.ticket_id,
      href: ticketDetailPath(ticket.ticket_id),
      kind: "票券",
      title: ticket.event_title || ticket.event_id,
      status: ticket.status,
      detail:
        ticket.status === "active"
          ? "票券已核發，可於現場驗票使用。"
          : `票券狀態：${ticketStatusView(ticket.status).label}`,
    })),
  ];

  return (
    <UserNotificationsPanel
      loading={loading}
      message={message}
      notificationRows={notificationRows}
      waitlistedCount={
        events.filter((event) => event.current_user_status === "waitlisted")
          .length
      }
      ticketCount={tickets.length}
      onRefresh={() => void refresh()}
    />
  );
}

export function NotificationDeliveryPage() {
  const [deliveries, setDeliveries] = useState<NotificationDelivery[]>([]);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");
  const [messageTone, setMessageTone] = useState<"fail" | "info">("info");
  const [statusFilter, setStatusFilter] = useState("all");
  const [retrying, setRetrying] = useState("");
  const [pendingRetry, setPendingRetry] = useState<NotificationDelivery | null>(
    null,
  );

  async function refresh() {
    setLoading(true);
    setMessage("");
    try {
      setDeliveries(await listNotificationDeliveries());
    } catch (error) {
      setMessageTone("fail");
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
      setDeliveries((current) =>
        current.map((candidate) =>
          candidate.delivery_id === row.delivery_id ? updated : candidate,
        ),
      );
      setPendingRetry(null);
      setMessageTone("info");
      setMessage(`已重試投遞 ${row.delivery_id}。`);
    } catch (error) {
      setMessageTone("fail");
      setMessage(error instanceof Error ? error.message : "重試投遞失敗。");
    } finally {
      setRetrying("");
    }
  }

  const filteredDeliveries = sortDeliveriesByAttention(
    deliveries.filter(
      (delivery) => statusFilter === "all" || delivery.status === statusFilter,
    ),
  );
  const deliveryCounts = deliveries.reduce<Record<string, number>>(
    (counts, delivery) => ({
      ...counts,
      all: counts.all + 1,
      [delivery.status]: (counts[delivery.status] ?? 0) + 1,
    }),
    { all: 0, pending: 0, failed: 0, dead_letter: 0, sent: 0, suppressed: 0 },
  );
  const deliveryStats = {
    total: deliveryCounts.all,
    pending: deliveryCounts.pending,
    failed: deliveryCounts.failed,
    deadLetter: deliveryCounts.dead_letter,
  };
  const retryActionProps = { loading, retrying, onRetry: setPendingRetry };

  return (
    <section className="content-grid">
      <Card className="panel span-12">
        <div className="section-heading">
          <div>
            <h2>投遞紀錄</h2>
            <p>
              從管理端介接讀取投遞紀錄，並支援對失敗與投遞終止事件重新嘗試。
            </p>
          </div>
          <div className="toolbar">
            <Button
              variant="outline"
              type="button"
              onClick={() => void refresh()}
              disabled={loading}
            >
              重新整理
            </Button>
          </div>
        </div>
        {message && <Alert tone={messageTone}>{message}</Alert>}
        <CompactStatsBar
          items={[
            { label: "總筆數", value: deliveryStats.total },
            { label: "待處理", value: deliveryStats.pending },
            { label: "重試失敗", value: deliveryStats.failed },
            { label: "投遞終止", value: deliveryStats.deadLetter },
          ]}
          label="通知投遞摘要"
        />
        <SegmentedFilter
          label="投遞狀態"
          value={statusFilter}
          onChange={setStatusFilter}
          options={deliveryStatusOptions}
          counts={deliveryCounts}
        />
        <ResponsiveTable
          label="通知投遞紀錄"
          mobileCards={filteredDeliveries.map((row) => (
            <article className="mobile-summary-card" key={row.delivery_id}>
              <div>
                <h3 className="mono-cell" title={row.delivery_id}>
                  投遞 {compactIdentifier(row.delivery_id)}
                </h3>
                <p className="table-muted" title={row.outbox_id}>
                  批次 {compactIdentifier(row.outbox_id)}
                </p>
              </div>
              <DeliveryStatusBadge status={row.status} />
              <MetaList
                rows={[
                  ["員工", row.employee_ref || "系統"],
                  ["通道", channelLabel(row.channel)],
                  ["嘗試次數", row.attempts],
                  ["最後錯誤", deliveryError(row)],
                ]}
              />
              <DeliveryRetryAction row={row} {...retryActionProps} />
            </article>
          ))}
        >
          <thead>
            <tr>
              <th scope="col">投遞編號</th>
              <th scope="col">投遞批次</th>
              <th scope="col">員工</th>
              <th scope="col">通道</th>
              <th scope="col">狀態</th>
              <th scope="col">嘗試次數</th>
              <th scope="col">最後錯誤</th>
              <th scope="col">操作</th>
            </tr>
          </thead>
          <tbody>
            {filteredDeliveries.map((row) => (
              <tr key={row.delivery_id}>
                <td className="mono-cell">{row.delivery_id}</td>
                <td className="mono-cell">{row.outbox_id}</td>
                <td>{row.employee_ref || "系統"}</td>
                <td>{channelLabel(row.channel)}</td>
                <td>
                  <DeliveryStatusBadge status={row.status} />
                </td>
                <td>{row.attempts}</td>
                <td>{deliveryError(row)}</td>
                <td>
                  <DeliveryRetryAction row={row} {...retryActionProps} />
                </td>
              </tr>
            ))}
          </tbody>
        </ResponsiveTable>
        {!loading && filteredDeliveries.length === 0 && (
          <EmptyState
            title="尚無投遞紀錄"
            action="完成報名或觸發通知事件後，會出現工作程序投遞結果。"
          />
        )}
      </Card>
      <RetryDeliveryDialog
        delivery={pendingRetry}
        busy={Boolean(retrying)}
        onClose={() => setPendingRetry(null)}
        onConfirm={(delivery) => void retry(delivery)}
      />
    </section>
  );
}

type RetryActionProps = {
  loading: boolean;
  retrying: string;
  row: NotificationDelivery;
  onRetry: (delivery: NotificationDelivery) => void;
};

function DeliveryStatusBadge({ status }: Readonly<{ status: string }>) {
  const view = deliveryStatusView(status);
  return <StatusBadge tone={view.tone}>{view.label}</StatusBadge>;
}

function DeliveryRetryAction({
  loading,
  retrying,
  row,
  onRetry,
}: Readonly<RetryActionProps>) {
  const isRetrying = retrying === row.delivery_id;
  const disabledReason = canRetryDelivery(row) ? "" : retryDisabledReason(row);

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        type="button"
        disabled={loading || Boolean(disabledReason) || isRetrying}
        onClick={() => onRetry(row)}
      >
        {retryButtonLabel(row, isRetrying)}
      </Button>
      {disabledReason && <span className="table-muted">{disabledReason}</span>}
    </>
  );
}

function deliveryError(row: NotificationDelivery) {
  return row.last_error ? localizedMessage(row.last_error) : "—";
}

function compactIdentifier(value: string) {
  if (value.length <= 22) return value;
  return `${value.slice(0, 8)}…${value.slice(-6)}`;
}
