import { EmptyState, StatusBadge } from "@/components/shared";
import type { EventSummary } from "@/lib/api";

export function AdminCreateResult({ event }: { event: EventSummary | null }) {
  return (
    <div className="panel span-4">
      <h2>建立結果</h2>
      {!event && (
        <EmptyState
          title="尚未建立活動"
          action="提交表單後會顯示活動 ID、容量與規則。"
        />
      )}
      {event && (
        <div className="summary-block">
          <StatusBadge tone="ok">{event.status}</StatusBadge>
          <h3>{event.title}</h3>
          <dl className="meta-list vertical">
            <div>
              <dt>Event ID</dt>
              <dd>{event.event_id}</dd>
            </div>
            <div>
              <dt>票數類型</dt>
              <dd>{event.capacity_type}</dd>
            </div>
            <div>
              <dt>容量</dt>
              <dd>{event.capacity_type === "unlimited" ? "不限" : event.capacity}</dd>
            </div>
            <div>
              <dt>規則</dt>
              <dd>
                {event.rule.department} / {event.rule.site} / G{event.rule.min_grade}+
              </dd>
            </div>
          </dl>
        </div>
      )}
    </div>
  );
}
