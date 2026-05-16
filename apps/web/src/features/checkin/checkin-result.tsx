import type { CheckinResponse } from "@/lib/api";
import { formatDate } from "@/lib/formatting";
import { StatusBadge } from "@/components/shared";
import { checkinStatusView, localizedMessage } from "@/lib/ui/options";

export function CheckinResult({ result }: { result: CheckinResponse }) {
  const status = checkinStatusView(result.status, result.duplicate);
  return (
    <div
      className={`checkin-result ${status.tone}`}
      role="status"
      aria-live="polite"
      tabIndex={-1}
    >
      <StatusBadge tone={status.tone}>{status.label}</StatusBadge>
      <h3>{checkinHeading(result.status, result.duplicate)}</h3>
      <dl className="meta-list vertical">
        <div>
          <dt>票券</dt>
          <dd>{result.ticket_id}</dd>
        </div>
        <div>
          <dt>員工</dt>
          <dd>{result.employee_id}</dd>
        </div>
        <div>
          <dt>掃描時間</dt>
          <dd>{formatDate(result.scanned_at || result.first_scanned_at)}</dd>
        </div>
        {result.first_scanned_by && (
          <div>
            <dt>首次掃描裝置</dt>
            <dd>{result.first_scanned_by}</dd>
          </div>
        )}
        {result.conflict_reason && (
          <div>
            <dt>拒絕原因</dt>
            <dd>{localizedMessage(result.conflict_reason)}</dd>
          </div>
        )}
      </dl>
    </div>
  );
}

function checkinHeading(status: string, duplicate: boolean) {
  if (duplicate || status === "duplicate") return "重複掃描被拒絕";
  if (status === "accepted") return "驗票成功";
  if (status === "rejected") return "驗票失敗，票券不可入場";
  return "驗票結果已更新";
}
