import { useEffect, useState } from "react";
import { listEligibilityImpactReviews, resolveEligibilityImpactReview } from "@/lib/api";
import type { EligibilityImpactReview } from "@/lib/api";
import { errorMessage, formatDate } from "@/lib/formatting";
import { EmptyState, Kpi, ResponsiveTable, StatusBadge } from "@/components/shared";

export function HrSyncSettingsPage() {
  const [reviews, setReviews] = useState<EligibilityImpactReview[]>([]);
  const [statusFilter, setStatusFilter] = useState("open");
  const [reasonByReview, setReasonByReview] = useState<Record<string, string>>({});
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(false);
  const [resolving, setResolving] = useState("");

  async function refresh() {
    setLoading(true);
    setMessage("");
    try {
      setReviews(await listEligibilityImpactReviews());
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function resolve(review: EligibilityImpactReview) {
    setResolving(review.review_id);
    setMessage("");
    try {
      const reason = reasonByReview[review.review_id]?.trim() || "HR 已人工處置影響項目。";
      const updated = await resolveEligibilityImpactReview(review.review_id, { reason });
      setReasonByReview((current) => {
        const next = { ...current };
        delete next[review.review_id];
        return next;
      });
      setReviews((current) => current.map((candidate) => (candidate.review_id === review.review_id ? updated : candidate)));
      setMessage(`已解析 review ${review.review_id}。`);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setResolving("");
    }
  }

  const filteredReviews = reviews.filter((review) => {
    if (statusFilter === "all") return true;
    if (statusFilter === "open") return review.status !== "resolved";
    return review.status === statusFilter;
  });
  const unresolvedCount = reviews.filter((review) => review.status !== "resolved").length;
  const resolvedCount = reviews.length - unresolvedCount;

  return (
    <section className="content-grid">
      <div className="panel span-12 workspace-context admin-context">
        <div>
          <div className="eyebrow">Admin Console</div>
          <h2>HR 同步設定</h2>
          <p>審核 HR 異動造成的 eligibility 影響；必要時以手動 resolve 導回一致性。</p>
        </div>
        <div className="kpi-row">
          <Kpi label="待處理" value={unresolvedCount} />
          <Kpi label="已處理" value={resolvedCount} />
          <Kpi label="總筆數" value={reviews.length} />
        </div>
      </div>
      <div className="panel span-12">
        <div className="section-heading">
          <div>
            <h2>影響項目清單</h2>
            <p>每列對應一筆 eligibility 影響，保留人工處置紀錄以進行稽核交接。</p>
          </div>
          <div className="toolbar">
            <label className="field compact">
              <span>狀態</span>
              <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
                <option value="open">open</option>
                <option value="resolved">resolved</option>
                <option value="all">all</option>
              </select>
            </label>
            <button className="button secondary" type="button" onClick={() => void refresh()} disabled={loading}>
              重新整理
            </button>
          </div>
        </div>
        {message && <div className="form-hint">{message}</div>}
        <ResponsiveTable>
          <thead>
            <tr>
              <th>Review</th>
              <th>事件</th>
              <th>員工</th>
              <th>票券</th>
              <th>狀態</th>
              <th>理由</th>
              <th>發生時間</th>
              <th>解析</th>
            </tr>
          </thead>
          <tbody>
            {filteredReviews.map((review) => (
              <tr key={review.review_id}>
                <td className="mono-cell">{review.review_id}</td>
                <td>{review.event_id}</td>
                <td>{review.employee_id}</td>
                <td>{review.ticket_id}</td>
                <td>
                  <StatusBadge tone={review.status === "resolved" ? "ok" : "warn"}>{review.status}</StatusBadge>
                </td>
                <td>
                  <div className="cell-vertical">
                    <span>{review.reason}</span>
                    {review.status !== "resolved" && (
                      <label className="field compact">
                        <span>resolve reason</span>
                        <input
                          type="text"
                          value={reasonByReview[review.review_id] || ""}
                          onChange={(event) =>
                            setReasonByReview((current) => ({
                              ...current,
                              [review.review_id]: event.target.value
                            }))
                          }
                        />
                      </label>
                    )}
                  </div>
                </td>
                <td>{formatDate(review.created_at)}</td>
                <td>
                  <button
                    className="button secondary compact-button"
                    type="button"
                    disabled={review.status === "resolved" || loading || resolving === review.review_id}
                    onClick={() => void resolve(review)}
                  >
                    {resolving === review.review_id ? "解析中" : "Resolve"}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </ResponsiveTable>
        {!loading && filteredReviews.length === 0 && <EmptyState title="目前沒有影響項目" action="HR 同步後，如有影響將會出現在此清單。" />}
      </div>
    </section>
  );
}
