import type { CheckinResponse } from "@/lib/api";
import { formatDate } from "@/lib/formatting";
import { StatusBadge } from "@/components/shared";
import { checkinStatusView, localizedMessage } from "@/lib/ui/options";

export function CheckinResult({ result }: { result: CheckinResponse }) {
  const status = checkinStatusView(result.status, result.duplicate);
  const holder = result.holder;
  const reasonCode = result.reason_code || result.conflict_reason || "";
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
          <dt>持票人</dt>
          <dd>
            {holder?.display_name || result.employee_id || "未知"}
            <span className="table-muted">
              {[holder?.department, holder?.city].filter(Boolean).join(" / ") ||
                "未提供部門與城市"}
            </span>
          </dd>
        </div>
        <div>
          <dt>同行家屬</dt>
          <dd>{result.family_count ?? 0} 人</dd>
        </div>
        {result.duplicate && (
          <div>
            <dt>首次入場</dt>
            <dd>
              {formatDate(result.first_scanned_at || result.scanned_at)}
              <span className="table-muted">
                裝置 {result.first_scanned_by || "未記錄"}
              </span>
            </dd>
          </div>
        )}
        {result.status === "rejected" && (
          <div>
            <dt>處置建議</dt>
            <dd>{recoveryCopy(reasonCode, result.rejection_message)}</dd>
          </div>
        )}
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
        {reasonCode && (
          <div>
            <dt>原因代碼</dt>
            <dd>
              {reasonCode}
              <span className="table-muted">
                {localizedMessage(result.rejection_message || reasonCode)}
              </span>
            </dd>
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

function recoveryCopy(reasonCode: string, rejectionMessage?: string) {
  if (rejectionMessage && reasonCode === "holder_mismatch") {
    return `請核對證件或轉交主辦人工處理：${localizedMessage(rejectionMessage)}`;
  }
  if (reasonCode === "duplicate_scan") {
    return "此票券已完成入場，請依首次入場時間與裝置核對現場紀錄。";
  }
  if (reasonCode === "revoked_ticket") {
    return "此票券已撤銷，請請持票人聯絡活動主辦重新確認資格。";
  }
  if (reasonCode === "expired_ticket") {
    return "此票券已逾期，請確認活動時間或交由主辦人工處理。";
  }
  if (reasonCode === "event_mismatch") {
    return "此票券屬於其他活動，請切換正確活動或請持票人出示正確票券。";
  }
  if (reasonCode === "ticket_token_claims_mismatch") {
    return "票券簽章與票券資料不一致，請拒絕入場並回報主辦。";
  }
  return localizedMessage(rejectionMessage || reasonCode || "invalid_ticket");
}
