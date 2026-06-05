import { useEffect, useState } from "react";
import { navigate, ticketDetailPath } from "@/app/routes";
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
  SkeletonRows,
  StatusBadge,
} from "@/components/shared";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  channelLabel,
  deliveryStatusOptions,
  deliveryStatusView,
  localizedMessage,
  registrationStatusView,
  ticketStatusView,
} from "@/lib/ui/options";
import {
  canRetryDelivery,
  retryButtonLabel,
  retryDisabledReason,
  sortDeliveriesByAttention,
} from "./delivery-helpers";
import { runClientNavigation } from "@/lib/navigation";

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

  useEffect(() => {
    void refresh();
  }, []);

  const notificationRows = [
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
    <section className="content-grid">
      <Card className="panel span-12" aria-busy={loading}>
        <div className="section-heading">
          <div>
            <h2>通知清單</h2>
            <p>報名、候補、票券與入場相關狀態集中在同一張表。</p>
          </div>
        </div>
        <CompactStatsBar
          items={[
            { label: "通知", value: notificationRows.length },
            {
              label: "候補更新",
              value: events.filter(
                (event) => event.current_user_status === "waitlisted",
              ).length,
            },
            { label: "票券更新", value: tickets.length },
          ]}
          label="通知摘要"
        />
        {message && (
          <Alert tone="fail">
            {message}
            <Button
              variant="outline"
              size="sm"
              type="button"
              onClick={() => void refresh()}
              disabled={loading}
            >
              重新整理
            </Button>
          </Alert>
        )}
        {loading ? (
          <SkeletonRows rows={4} />
        ) : (
          <ResponsiveTable
            label="通知清單"
            mobileCards={notificationRows.map((row) => (
              <article className="mobile-summary-card" key={row.id}>
                <div>
                  <h3>
                    <NotificationSubject href={row.href} title={row.title} />
                  </h3>
                  <p className="table-muted">{row.kind}</p>
                </div>
                <NotificationStatusBadge kind={row.kind} status={row.status} />
                <p>{row.detail}</p>
              </article>
            ))}
          >
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
                  <td>
                    <NotificationSubject href={row.href} title={row.title} />
                  </td>
                  <td>
                    <NotificationStatusBadge
                      kind={row.kind}
                      status={row.status}
                    />
                  </td>
                  <td>{row.detail}</td>
                </tr>
              ))}
            </tbody>
          </ResponsiveTable>
        )}
        {!loading && !message && notificationRows.length === 0 && (
          <EmptyState
            title="尚無通知"
            action="報名、候補或票券狀態有變更時，通知會出現在這裡。"
          />
        )}
      </Card>
    </section>
  );
}

type NotificationStatusProps = { kind: string; status?: string };

function NotificationSubject({
  href,
  title,
}: Readonly<{ href: string; title: string }>) {
  return (
    <a
      href={href}
      onClick={(event) => runClientNavigation(event, () => navigate(href))}
    >
      {title}
    </a>
  );
}

function NotificationStatusBadge({
  kind,
  status = "",
}: Readonly<NotificationStatusProps>) {
  const view =
    kind === "票券" ? ticketStatusView(status) : registrationStatusView(status);
  return <StatusBadge tone={view.tone}>{view.label}</StatusBadge>;
}

export function NotificationDeliveryPage() {
  const [deliveries, setDeliveries] = useState<NotificationDelivery[]>([]);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");
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
      setMessage(`已重試投遞 ${row.delivery_id}。`);
    } catch (error) {
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
    (counts, delivery) => {
      counts.all += 1;
      counts[delivery.status] = (counts[delivery.status] ?? 0) + 1;
      return counts;
    },
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
        {message && (
          <Alert tone={message.includes("失敗") ? "fail" : "info"}>
            {message}
          </Alert>
        )}
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
              <th>投遞編號</th>
              <th>投遞批次</th>
              <th>員工</th>
              <th>通道</th>
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

function RetryDeliveryDialog({
  busy,
  delivery,
  onClose,
  onConfirm,
}: Readonly<{
  busy: boolean;
  delivery: NotificationDelivery | null;
  onClose: () => void;
  onConfirm: (delivery: NotificationDelivery) => void;
}>) {
  return (
    <Dialog
      open={Boolean(delivery)}
      onOpenChange={(open) => !open && onClose()}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>確認重試通知</DialogTitle>
          <DialogDescription>
            系統會建立下一次投遞嘗試，並保留原本的投遞紀錄與稽核軌跡。
          </DialogDescription>
        </DialogHeader>
        {delivery && (
          <MetaList
            rows={[
              ["投遞編號", delivery.delivery_id],
              ["投遞批次", delivery.outbox_id],
              ["下一次嘗試", `第 ${delivery.attempts + 1} 次`],
            ]}
          />
        )}
        <DialogFooter>
          <Button
            variant="outline"
            type="button"
            onClick={onClose}
            disabled={busy}
          >
            返回
          </Button>
          <Button
            type="button"
            onClick={() => delivery && onConfirm(delivery)}
            disabled={busy || !delivery}
          >
            {busy ? "重試中" : "確認重試"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
