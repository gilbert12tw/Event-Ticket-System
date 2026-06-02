import { useEffect, useMemo, useState } from "react";
import { getOpsDashboard } from "@/lib/api";
import type {
  CapacityPressureRow,
  NotificationDeliveryOpsRow,
  OpsDashboard,
  OutboxReplayRecentRow,
  QueueStatusRow,
  ReportFreshnessProjection,
} from "@/lib/api";
import { errorMessage, formatDate } from "@/lib/formatting";
import {
  Alert,
  CompactStatsBar,
  EmptyState,
  ResponsiveTable,
  SkeletonRows,
  StatusBadge,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";

const emptyDashboard: OpsDashboard = {
  capacity_pressure: { events: [] },
  queues: { queues: [] },
  reports_freshness: { projections: [] },
  dead_letter_recent: [],
  replay_recent: [],
};

export function OpsControlPlanePage() {
  const [dashboard, setDashboard] = useState<OpsDashboard>(emptyDashboard);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");

  async function refresh() {
    setLoading(true);
    setMessage("");
    try {
      setDashboard(await getOpsDashboard());
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    let ignore = false;
    async function load() {
      setLoading(true);
      setMessage("");
      try {
        const next = await getOpsDashboard();
        if (!ignore) setDashboard(next);
      } catch (error) {
        if (!ignore) setMessage(errorMessage(error));
      } finally {
        if (!ignore) setLoading(false);
      }
    }
    void load();
    return () => {
      ignore = true;
    };
  }, []);

  const capacityRows = dashboard.capacity_pressure.events;
  const queueRows = dashboard.queues.queues;
  const freshnessRows = dashboard.reports_freshness.projections;
  const deadLetters = dashboard.dead_letter_recent ?? [];
  const replays = dashboard.replay_recent ?? [];
  const stats = useMemo(
    () => opsStats(capacityRows, queueRows, freshnessRows),
    [capacityRows, queueRows, freshnessRows],
  );

  return (
    <section className="content-grid ops-page" aria-busy={loading}>
      <Card className="panel span-12 ops-overview">
        <div className="section-heading">
          <div>
            <h2>營運監控</h2>
            <p>容量、佇列、報表與 dead letter 以唯讀快照呈現。</p>
          </div>
          <Button
            type="button"
            variant="outline"
            onClick={() => void refresh()}
          >
            <Icon name="refresh" />
            重新整理
          </Button>
        </div>
        <CompactStatsBar
          items={[
            { label: "活動", value: stats.events },
            { label: "預約", value: stats.reservations },
            { label: "待處理", value: stats.pending },
            { label: "Dead letter", value: stats.deadLetters },
            { label: "報表延遲", value: secondsLabel(stats.maxLagSeconds) },
            { label: "劣化投影", value: stats.degraded },
          ]}
          label="營運監控摘要"
        />
      </Card>
      {message && <Alert tone="warn">{message}</Alert>}
      {loading && <SkeletonRows rows={4} />}

      <CapacityPressurePanel rows={capacityRows} />
      <QueuePanel rows={queueRows} />
      <ReportFreshnessPanel rows={freshnessRows} />
      <DeadLetterPanel rows={deadLetters} />
      <ReplayRecentPanel rows={replays} />
    </section>
  );
}

function CapacityPressurePanel({
  rows,
}: Readonly<{ rows: CapacityPressureRow[] }>) {
  return (
    <Card className="panel span-6 ops-panel">
      <PanelHeading title="容量壓力" count={rows.length} />
      {rows.length === 0 ? (
        <EmptyState
          title="沒有容量壓力資料"
          action="目前沒有公開或已關閉活動。"
        />
      ) : (
        <ResponsiveTable
          label="容量壓力"
          mobileCards={rows.map((row) => (
            <article className="mobile-summary-card" key={row.event_id}>
              <h3>{row.event_id}</h3>
              <CompactStatsBar
                items={[
                  {
                    label: "剩餘",
                    value: nullableNumber(row.remaining_capacity),
                  },
                  { label: "預約", value: row.reservation_count },
                  {
                    label: "限流",
                    value: nullableNumber(row.rate_limit_drop_per_min),
                  },
                  {
                    label: "重播",
                    value: nullableNumber(row.idempotency_replay_per_min),
                  },
                ]}
              />
            </article>
          ))}
        >
          <thead>
            <tr>
              <th>活動</th>
              <th>類型</th>
              <th className="number-cell">剩餘</th>
              <th className="number-cell">預約</th>
              <th className="number-cell">限流/min</th>
              <th className="number-cell">重播/min</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.event_id}>
                <td className="mono-cell">{row.event_id}</td>
                <td>{capacityLabel(row.capacity_type)}</td>
                <td className="number-cell">
                  {nullableNumber(row.remaining_capacity)}
                </td>
                <td className="number-cell">{row.reservation_count}</td>
                <td className="number-cell">
                  {nullableNumber(row.rate_limit_drop_per_min)}
                </td>
                <td className="number-cell">
                  {nullableNumber(row.idempotency_replay_per_min)}
                </td>
              </tr>
            ))}
          </tbody>
        </ResponsiveTable>
      )}
    </Card>
  );
}

function QueuePanel({ rows }: Readonly<{ rows: QueueStatusRow[] }>) {
  return (
    <Card className="panel span-6 ops-panel">
      <PanelHeading title="Worker 佇列" count={rows.length} />
      {rows.length === 0 ? (
        <EmptyState
          title="沒有佇列資料"
          action="此角色目前只有容量壓力快照。"
        />
      ) : (
        <ResponsiveTable label="Worker 佇列">
          <thead>
            <tr>
              <th>Kind</th>
              <th className="number-cell">Pending</th>
              <th className="number-cell">In flight</th>
              <th className="number-cell">Dead</th>
              <th className="number-cell">p95</th>
              <th>Last processed</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.name}>
                <td>{workerLabel(row.name)}</td>
                <td className="number-cell">{row.pending}</td>
                <td className="number-cell">{row.in_flight}</td>
                <td className="number-cell">
                  <StatusBadge tone={row.dead_letter > 0 ? "fail" : "ok"}>
                    {row.dead_letter}
                  </StatusBadge>
                </td>
                <td className="number-cell">
                  {secondsLabel(row.p95_age_seconds)}
                </td>
                <td>{formatDate(row.last_processed_at ?? undefined)}</td>
              </tr>
            ))}
          </tbody>
        </ResponsiveTable>
      )}
    </Card>
  );
}

function ReportFreshnessPanel({
  rows,
}: Readonly<{ rows: ReportFreshnessProjection[] }>) {
  return (
    <Card className="panel span-6 ops-panel">
      <PanelHeading title="報表新鮮度" count={rows.length} />
      {rows.length === 0 ? (
        <EmptyState
          title="沒有報表投影資料"
          action="此角色目前只有容量壓力快照。"
        />
      ) : (
        <ResponsiveTable label="報表新鮮度">
          <thead>
            <tr>
              <th>Projection</th>
              <th>Last applied</th>
              <th className="number-cell">Lag</th>
              <th>State</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.name}>
                <td className="mono-cell">{row.name}</td>
                <td>{formatDate(row.last_applied ?? undefined)}</td>
                <td className="number-cell">{secondsLabel(row.lag_seconds)}</td>
                <td>
                  <StatusBadge tone={row.degraded ? "warn" : "ok"}>
                    {row.degraded ? "Stale" : "Fresh"}
                  </StatusBadge>
                </td>
              </tr>
            ))}
          </tbody>
        </ResponsiveTable>
      )}
    </Card>
  );
}

function DeadLetterPanel({
  rows,
}: Readonly<{ rows: NotificationDeliveryOpsRow[] }>) {
  return (
    <Card className="panel span-6 ops-panel">
      <PanelHeading title="Dead letter" count={rows.length} />
      {rows.length === 0 ? (
        <EmptyState
          title="沒有 dead letter"
          action="最近沒有需要處理的投遞列。"
        />
      ) : (
        <ResponsiveTable label="Dead letter">
          <thead>
            <tr>
              <th>Delivery</th>
              <th>Kind</th>
              <th>Event</th>
              <th className="number-cell">Retry</th>
              <th>Last error</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.delivery_id}>
                <td className="mono-cell">{row.delivery_id}</td>
                <td>{workerLabel(row.worker_kind)}</td>
                <td>{row.event_type}</td>
                <td className="number-cell">{row.retry_count}</td>
                <td>{row.last_error || "已遮蔽或未記錄"}</td>
              </tr>
            ))}
          </tbody>
        </ResponsiveTable>
      )}
    </Card>
  );
}

function ReplayRecentPanel({
  rows,
}: Readonly<{ rows: OutboxReplayRecentRow[] }>) {
  return (
    <Card className="panel span-6 ops-panel">
      <PanelHeading title="Replay recent" count={rows.length} />
      {rows.length === 0 ? (
        <EmptyState
          title="沒有 replay 紀錄"
          action="最近沒有 outbox replay 動作。"
        />
      ) : (
        <ResponsiveTable label="Replay recent">
          <thead>
            <tr>
              <th>Audit</th>
              <th>Kind</th>
              <th>Mode</th>
              <th className="number-cell">Affected</th>
              <th className="number-cell">Enqueued</th>
              <th>Created</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.audit_id}>
                <td className="mono-cell">{row.audit_id}</td>
                <td>{workerLabel(row.kind)}</td>
                <td>{row.dry_run ? "Dry run" : "Applied"}</td>
                <td className="number-cell">{row.affected_count}</td>
                <td className="number-cell">
                  {nullableNumber(row.enqueued_count ?? null)}
                </td>
                <td>{formatDate(row.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </ResponsiveTable>
      )}
    </Card>
  );
}

function PanelHeading({
  count,
  title,
}: Readonly<{ count: number; title: string }>) {
  return (
    <div className="section-heading compact">
      <h3>{title}</h3>
      <StatusBadge tone="neutral">{count}</StatusBadge>
    </div>
  );
}

function opsStats(
  capacityRows: CapacityPressureRow[],
  queueRows: QueueStatusRow[],
  freshnessRows: ReportFreshnessProjection[],
) {
  return {
    events: capacityRows.length,
    reservations: capacityRows.reduce(
      (total, row) => total + row.reservation_count,
      0,
    ),
    pending: queueRows.reduce((total, row) => total + row.pending, 0),
    deadLetters: queueRows.reduce((total, row) => total + row.dead_letter, 0),
    maxLagSeconds: freshnessRows.reduce(
      (max, row) => Math.max(max, row.lag_seconds),
      0,
    ),
    degraded: freshnessRows.filter((row) => row.degraded).length,
  };
}

function capacityLabel(value: string) {
  return value === "unlimited" ? "不限量" : "限量";
}

function workerLabel(value: string) {
  if (value === "in_flight") return "In flight";
  return value.replaceAll("_", " ");
}

function nullableNumber(value: number | null) {
  return value ?? "-";
}

function secondsLabel(seconds: number) {
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const rest = seconds % 60;
  return rest === 0 ? `${minutes}m` : `${minutes}m ${rest}s`;
}
