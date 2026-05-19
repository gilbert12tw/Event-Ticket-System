import type { CheckinResponse, OfflineCheckinSyncResponse } from "@/lib/api";
import { formatDate } from "@/lib/formatting";
import { EmptyState, Kpi, ResponsiveTable, StatusBadge } from "@/components/shared";
import { checkinStatusView, localizedMessage } from "@/lib/ui/options";

export function OfflineResultStep({
  syncResult,
}: {
  syncResult: OfflineCheckinSyncResponse | null;
}) {
  return (
    <>
      {syncResult && (
        <div className="kpi-row">
          <Kpi label="成功" value={syncResult.accepted} />
          <Kpi label="重複" value={syncResult.duplicate} />
          <Kpi label="衝突" value={syncResult.conflict} />
        </div>
      )}
      {syncResult && syncResult.results.length > 0 ? (
        <ResponsiveTable>
          <thead>
            <tr>
              <th>票券</th>
              <th>持票人</th>
              <th>活動</th>
              <th>狀態</th>
              <th>原因</th>
              <th>掃描時間</th>
            </tr>
          </thead>
          <tbody>
            {syncResult.results.map((result) => {
              const status = checkinStatusView(result.status, result.duplicate);
              return (
                <tr
                  key={`${result.checkin_id || result.ticket_id || result.scanned_at}-${result.status}`}
                >
                  <td className="mono-cell">{result.ticket_id}</td>
                  <td>
                    {result.holder?.display_name || result.employee_id}
                    <span className="table-muted">
                      {checkinHolderSummary(result)}
                    </span>
                  </td>
                  <td>{result.event_title || result.event_id}</td>
                  <td>
                    <StatusBadge tone={status.tone}>{status.label}</StatusBadge>
                  </td>
                  <td>{checkinReasonLabel(result)}</td>
                  <td>{formatDate(result.scanned_at)}</td>
                </tr>
              );
            })}
          </tbody>
        </ResponsiveTable>
      ) : (
        <EmptyState
          title="尚無同步結果"
          action="完成第二步同步後，成功、重複與衝突結果會出現在這裡。"
        />
      )}
    </>
  );
}

function checkinHolderSummary(result: CheckinResponse) {
  const identity = [
    result.employee_id,
    result.holder?.department,
    result.holder?.city,
  ]
    .filter(Boolean)
    .join(" / ");
  return `${identity} · 同行 ${result.family_count ?? 0} 人`;
}

function checkinReasonLabel(result: CheckinResponse) {
  if (!result.reason_code && !result.conflict_reason) return "-";
  return localizedMessage(
    result.rejection_message || result.reason_code || result.conflict_reason || "",
  );
}
