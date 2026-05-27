import { EmptyState, MetaList, StatusBadge } from "@/components/shared";
import type { EventSummary } from "@/lib/api";
import { Card } from "@/components/ui/card";
import { departmentLabel, eventStatusView, siteLabel } from "@/lib/ui/options";

export function AdminCreateResult({ event }: { event: EventSummary | null }) {
  return (
    <Card className="panel span-4">
      <h2>建立結果</h2>
      {!event && (
        <EmptyState
          title="尚未建立活動"
          action="提交表單後會顯示活動編號、容量與規則。"
        />
      )}
      {event && (
        <div className="summary-block">
          <StatusBadge tone={eventStatusView(event.status).tone}>
            {eventStatusView(event.status).label}
          </StatusBadge>
          <h3>{event.title}</h3>
          <MetaList
            rows={[
              ["活動編號", event.event_id],
              [
                "票數類型",
                event.capacity_type === "unlimited" ? "不限量" : "限量",
              ],
              [
                "容量",
                event.capacity_type === "unlimited" ? "不限" : event.capacity,
              ],
              [
                "規則",
                <>
                  {departmentLabel(event.rule.department)} /{" "}
                  {siteLabel(event.rule.site)} / G{event.rule.min_grade}+
                </>,
              ],
            ]}
          />
        </div>
      )}
    </Card>
  );
}
