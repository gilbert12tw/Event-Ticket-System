import { useEffect, useState } from "react";
import {
  listEligibilityImpactReviews,
  resolveEligibilityImpactReview,
} from "@/lib/api";
import type { EligibilityImpactReview } from "@/lib/api";
import { errorMessage, formatDate } from "@/lib/formatting";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Alert,
  CompactStatsBar,
  EmptyState,
  Field,
  ResponsiveTable,
  SelectField,
  StatusBadge,
} from "@/components/shared";
import {
  hrReviewStatusOptions,
  resolutionReasonOptions,
  reviewStatusView,
} from "@/lib/ui/options";

export function HrSyncSettingsPage() {
  const [reviews, setReviews] = useState<EligibilityImpactReview[]>([]);
  const [statusFilter, setStatusFilter] = useState("open");
  const [selectedID, setSelectedID] = useState("");
  const [reasonByReview, setReasonByReview] = useState<Record<string, string>>(
    {},
  );
  const [customReasonByReview, setCustomReasonByReview] = useState<
    Record<string, string>
  >({});
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(false);
  const [resolving, setResolving] = useState("");

  async function refresh() {
    setLoading(true);
    setMessage("");
    try {
      const rows = await listEligibilityImpactReviews();
      setReviews(rows);
      setSelectedID((current) => current || rows[0]?.review_id || "");
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
      const selectedReason =
        reasonByReview[review.review_id] === "custom"
          ? customReasonByReview[review.review_id]?.trim()
          : reasonByReview[review.review_id]?.trim();
      const reason = selectedReason || "人資已人工處置影響項目。";
      const updated = await resolveEligibilityImpactReview(review.review_id, {
        reason,
      });
      setReasonByReview((current) => {
        const next = { ...current };
        delete next[review.review_id];
        return next;
      });
      setCustomReasonByReview((current) => {
        const next = { ...current };
        delete next[review.review_id];
        return next;
      });
      setReviews((current) =>
        current.map((candidate) =>
          candidate.review_id === review.review_id ? updated : candidate,
        ),
      );
      setMessage(`已處理影響項目 ${review.review_id}。`);
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
  const unresolvedCount = reviews.filter(
    (review) => review.status !== "resolved",
  ).length;
  const resolvedCount = reviews.length - unresolvedCount;
  const selected =
    filteredReviews.find((review) => review.review_id === selectedID) ||
    filteredReviews[0];
  const selectedResolution = selected ? reasonByReview[selected.review_id] : "";
  const selectedCustomResolution = selected
    ? customReasonByReview[selected.review_id]?.trim()
    : "";
  const resolutionReady =
    Boolean(selectedResolution) &&
    (selectedResolution !== "custom" || Boolean(selectedCustomResolution));

  return (
    <section className="content-grid">
      <Card className="panel span-8">
        <div className="section-heading">
          <div>
            <h2>影響項目清單</h2>
            <p>每列對應一筆資格影響，保留人工處置紀錄以進行稽核交接。</p>
          </div>
          <div className="toolbar">
            <SelectField
              label="狀態"
              value={statusFilter}
              options={hrReviewStatusOptions}
              onChange={setStatusFilter}
            />
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
        <CompactStatsBar
          items={[
            { label: "待處理", value: unresolvedCount },
            { label: "已處理", value: resolvedCount },
            { label: "總筆數", value: reviews.length },
          ]}
          label="人資同步摘要"
        />
        {message && (
          <Alert tone={message.includes("失敗") ? "fail" : "info"}>
            {message}
          </Alert>
        )}
        <ResponsiveTable>
          <thead>
            <tr>
              <th>審核編號</th>
              <th>事件</th>
              <th>員工</th>
              <th>票券</th>
              <th>狀態</th>
              <th>理由</th>
              <th>發生時間</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {filteredReviews.map((review) => (
              <tr
                key={review.review_id}
                className={
                  selected?.review_id === review.review_id ? "selected-row" : ""
                }
                aria-selected={selected?.review_id === review.review_id}
              >
                <td className="mono-cell">{review.review_id}</td>
                <td>{review.event_id}</td>
                <td>{review.employee_ref}</td>
                <td>{review.ticket_id}</td>
                <td>
                  <StatusBadge tone={reviewStatusView(review.status).tone}>
                    {reviewStatusView(review.status).label}
                  </StatusBadge>
                </td>
                <td>
                  <span>{review.reason}</span>
                </td>
                <td>{formatDate(review.created_at)}</td>
                <td>
                  <Button
                    variant="outline"
                    size="sm"
                    type="button"
                    disabled={loading}
                    onClick={() => setSelectedID(review.review_id)}
                  >
                    檢視
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </ResponsiveTable>
        {!loading && filteredReviews.length === 0 && (
          <EmptyState
            title="目前沒有影響項目"
            action="人資同步後，如有影響將會出現在此清單。"
          />
        )}
      </Card>
      <Card asChild className="panel span-4 detail-panel">
        <aside>
          <div className="section-heading">
            <div>
              <h2>處置面板</h2>
              <p>選取單筆影響項目後再填寫原因，表格保持可掃描。</p>
            </div>
          </div>
          {!selected ? (
            <EmptyState
              title="尚未選擇項目"
              action="從左側表格選取一筆人資影響項目。"
            />
          ) : (
            <div className="summary-block">
              <dl className="meta-list vertical">
                <div>
                  <dt>審核編號</dt>
                  <dd className="mono-cell">{selected.review_id}</dd>
                </div>
                <div>
                  <dt>員工</dt>
                  <dd>{selected.employee_ref}</dd>
                </div>
                <div>
                  <dt>票券</dt>
                  <dd>{selected.ticket_id}</dd>
                </div>
                <div>
                  <dt>理由</dt>
                  <dd>{selected.reason}</dd>
                </div>
              </dl>
              {selected.status === "resolved" ? (
                <StatusBadge tone="ok">已處理</StatusBadge>
              ) : (
                <>
                  <SelectField
                    label="處置模板"
                    value={reasonByReview[selected.review_id] || ""}
                    options={[
                      { value: "", label: "選擇處置方式" },
                      ...resolutionReasonOptions,
                    ]}
                    onChange={(value) =>
                      setReasonByReview((current) => ({
                        ...current,
                        [selected.review_id]: value,
                      }))
                    }
                  />
                  {reasonByReview[selected.review_id] === "custom" && (
                    <Field
                      label="自訂處置原因"
                      value={customReasonByReview[selected.review_id] || ""}
                      onChange={(value) =>
                        setCustomReasonByReview((current) => ({
                          ...current,
                          [selected.review_id]: value,
                        }))
                      }
                      hint="請輸入會寫入稽核的人工處置說明。"
                    />
                  )}
                  <div className="helper-strip">
                    <StatusBadge tone="warn">將寫入稽核</StatusBadge>
                    <span>
                      將關閉此資格影響：員工 {selected.employee_ref} · 活動{" "}
                      {selected.event_id} · 票券 {selected.ticket_id}
                    </span>
                  </div>
                  <Button
                    type="button"
                    disabled={
                      loading ||
                      resolving === selected.review_id ||
                      !resolutionReady
                    }
                    onClick={() => void resolve(selected)}
                  >
                    {resolving === selected.review_id ? "處理中" : "標記已處理"}
                  </Button>
                </>
              )}
            </div>
          )}
        </aside>
      </Card>
    </section>
  );
}
