import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { ApiError, checkIn, listAdminEvents, offlineCheckinPackage, reports, syncOfflineCheckins } from "@/lib/api";
import type { CheckinResponse, EventSummary, OfflineCheckinPackage, OfflineCheckinSyncResponse, ReportRow } from "@/lib/api";
import { errorMessage, formatDate } from "@/lib/formatting";
import { Alert, BoundaryContext, EmptyState, Field, Kpi, ResponsiveTable, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { getDemoCheckinToken, hasDemoCheckinToken, readHistoryCheckinToken } from "./checkin-token";

export function CheckinPage() {
  const [token, setToken] = useState(() => readHistoryCheckinToken() || getDemoCheckinToken());
  const [deviceID, setDeviceID] = useState("gate-1");
  const [result, setResult] = useState<CheckinResponse | null>(null);
  const [message, setMessage] = useState("");
  const [rows, setRows] = useState<ReportRow[]>([]);
  const [busy, setBusy] = useState(false);
  const recentToken = hasDemoCheckinToken() ? getDemoCheckinToken() : "";

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    setResult(null);
    try {
      const response = await checkIn(token.trim(), deviceID.trim());
      setResult(response);
      setRows(await reports());
    } catch (error) {
      if (error instanceof ApiError && error.response.data) {
        setResult(error.response.data as CheckinResponse);
      }
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    reports().then(setRows).catch(() => setRows([]));
  }, []);

  const total = rows.reduce(
    (acc, row) => ({
      capacity: acc.capacity + row.capacity,
      checkedIn: acc.checkedIn + row.checkin_count
    }),
    { capacity: 0, checkedIn: 0 }
  );
  const tokenReady = token.trim().length > 0 && deviceID.trim().length > 0;
  const checkinRate = total.capacity > 0 ? `${Math.round((total.checkedIn / total.capacity) * 100)}%` : "0%";

  return (
    <section className="content-grid">
      <div className="panel span-12 workspace-context admin-context">
        <div>
          <div className="eyebrow">Admin Console</div>
          <h2>驗票員入口</h2>
          <p>Phase 1 以手動 token 模擬掃描器輸入；真實相機掃描與離線同步保留為 Phase 2 範圍。</p>
        </div>
        <div className="context-kpis">
          <Kpi label="已入場" value={total.checkedIn} />
          <Kpi label="總名額" value={total.capacity} />
          <Kpi label="裝置" value={deviceID} />
          <Kpi label="入場率" value={checkinRate} />
        </div>
      </div>
      <form className="panel span-6 checkin-form" onSubmit={(event) => void submit(event)}>
        <div className="section-heading">
          <div>
            <h2>線上驗票</h2>
            <p>DB unique constraint 是重複核銷的最終保證。</p>
          </div>
        </div>
        <label className="field">
          <span>Signed token</span>
          <textarea value={token} onChange={(event) => setToken(event.target.value)} rows={7} required />
        </label>
        <Field label="裝置 ID" value={deviceID} onChange={setDeviceID} required />
        <div className="helper-strip">
          <StatusBadge tone={recentToken ? "info" : "neutral"}>Demo helper</StatusBadge>
          <span>{recentToken ? "可載入最近票券 token。" : "完成票券頁或 Demo Runbook 後會保留最近 token。"}</span>
          <button className="button secondary" type="button" onClick={() => setToken(recentToken)} disabled={!recentToken}>
            使用最近票券
          </button>
        </div>
        <button className="button" type="submit" disabled={busy || !tokenReady}>
          <Icon name="scan" />
          {busy ? "驗票中" : "送出驗票"}
        </button>
        <Alert tone="info">相機掃描、離線名單下載與同步衝突處理不在本次 Phase 1 UI 重整範圍。</Alert>
      </form>
      <div className="panel span-6">
        <h2>驗票結果</h2>
        <div className="kpi-row">
          <Kpi label="已入場" value={total.checkedIn} />
          <Kpi label="總名額" value={total.capacity} />
        </div>
        {message && <Alert tone={message.includes("already") ? "warn" : "fail"}>{message}</Alert>}
        {!result && !message && <EmptyState title="等待掃描" action="貼上票券 token 後送出，結果會在此顯示。" />}
        {result && <CheckinResult result={result} />}
      </div>
    </section>
  );
}

export function CheckinResult({ result }: { result: CheckinResponse }) {
  return (
    <div className={result.duplicate ? "checkin-result warn" : "checkin-result ok"} role="status" aria-live="polite">
      <StatusBadge tone={result.duplicate ? "warn" : "ok"}>{result.duplicate ? "duplicate" : result.status}</StatusBadge>
      <h3>{result.duplicate ? "重複掃描被拒絕" : "驗票成功"}</h3>
      <dl className="meta-list vertical">
        <div>
          <dt>Ticket</dt>
          <dd>{result.ticket_id}</dd>
        </div>
        <div>
          <dt>Employee</dt>
          <dd>{result.employee_id}</dd>
        </div>
        <div>
          <dt>Scanned at</dt>
          <dd>{formatDate(result.scanned_at || result.first_scanned_at)}</dd>
        </div>
        {result.first_scanned_by && (
          <div>
            <dt>First scanned by</dt>
            <dd>{result.first_scanned_by}</dd>
          </div>
        )}
      </dl>
    </div>
  );
}

export function OfflineCheckinBoundaryPage() {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [eventID, setEventID] = useState("");
  const [deviceID, setDeviceID] = useState("gate-offline-1");
  const [batchForm, setBatchForm] = useState("");
  const [message, setMessage] = useState("");
  const [packageSummary, setPackageSummary] = useState<OfflineCheckinPackage | null>(null);
  const [syncResult, setSyncResult] = useState<OfflineCheckinSyncResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    void loadEvents();
  }, []);

  async function loadEvents() {
    setLoading(true);
    setMessage("");
    try {
      const nextEvents = await listAdminEvents();
      setEvents(nextEvents);
      if (!eventID) {
        setEventID(nextEvents[0]?.event_id || "");
      }
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  async function downloadPackage() {
    if (!eventID.trim()) {
      setMessage("請先選擇活動。");
      return;
    }
    setBusy(true);
    setMessage("");
    setSyncResult(null);
    try {
      const next = await offlineCheckinPackage(eventID.trim(), deviceID.trim());
      setPackageSummary(next);
      setBatchForm("");
      setMessage(`已下載 batch ${next.batch_id}，共 ${next.ticket_count} 張票券。`);
    } catch (error) {
      setMessage(errorMessage(error));
      setPackageSummary(null);
    } finally {
      setBusy(false);
    }
  }

  async function syncBatch() {
    if (!packageSummary) {
      setMessage("請先下載離線名單。");
      return;
    }
    const scanned_at = new Date().toISOString();
    const scans = batchForm
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean)
      .map((signed_token) => ({ signed_token, scanned_at }));

    if (scans.length === 0) {
      setMessage("掃描名單不能為空。");
      return;
    }

    setBusy(true);
    setMessage("");
    try {
      const result = await syncOfflineCheckins({
        batch_id: packageSummary.batch_id,
        event_id: packageSummary.event_id,
        device_id: packageSummary.device_id,
        package_signature: packageSummary.package_signature,
        scans
      });
      setSyncResult(result);
      setMessage(`已同步 ${scans.length} 筆掃描，accepted ${result.accepted}，conflict ${result.conflict}。`);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="content-grid">
      <BoundaryContext
        title="離線驗票邊界"
        description="選擇活動與設備下載簽章名單，再上傳簽章 token 批次同步結果。"
        icon="wifiOff"
      />
      <div className="panel span-6">
        <div className="section-heading">
          <div>
            <h2>離線名單</h2>
            <p>先下載活動票券清單，避免在離線端保留完整明文票券資訊。</p>
          </div>
          <button className="button secondary" type="button" onClick={() => void loadEvents()} disabled={loading}>
            重新載入活動
          </button>
        </div>
        <label className="field">
          <span>活動</span>
          <select value={eventID} onChange={(event) => setEventID(event.target.value)} disabled={loading}>
            <option value="">選擇活動</option>
            {events.map((event) => (
              <option value={event.event_id} key={event.event_id}>
                {event.title}
              </option>
            ))}
          </select>
        </label>
        <div className="form-grid mt-14">
          <Field label="裝置 ID" value={deviceID} onChange={setDeviceID} />
          <button className="button" type="button" onClick={() => void downloadPackage()} disabled={busy || loading || !eventID}>
            <Icon name="wifiOff" />
            {busy ? "下載中" : "下載離線名單"}
          </button>
        </div>
        <Alert tone={packageSummary ? "ok" : "info"}>{message || "請先下載離線名單。"} </Alert>
        {packageSummary && (
          <dl className="meta-list vertical">
            <div>
              <dt>Batch ID</dt>
              <dd>{packageSummary.batch_id}</dd>
            </div>
            <div>
              <dt>有效至</dt>
              <dd>{formatDate(packageSummary.valid_until)}</dd>
            </div>
            <div>
              <dt>Package signature</dt>
              <dd className="mono-cell">{packageSummary.package_signature}</dd>
            </div>
            <div>
              <dt>可同步票券</dt>
              <dd>{packageSummary.ticket_count}</dd>
            </div>
          </dl>
        )}
      </div>
      <div className="panel span-6">
        <div className="section-heading">
          <div>
            <h2>掃描 batch 同步</h2>
            <p>每行輸入一個簽章 token；未提供掃描時間時以目前時間補上。</p>
          </div>
        </div>
        <label className="field full">
          <span>scan batch</span>
          <textarea value={batchForm} rows={9} onChange={(event) => setBatchForm(event.target.value)} />
        </label>
        <button className="button" type="button" onClick={() => void syncBatch()} disabled={busy || !packageSummary}>
          <Icon name="scan" />
          {busy ? "同步中" : "同步名單"}
        </button>
        {syncResult && (
          <div className="kpi-row">
            <Kpi label="accepted" value={syncResult.accepted} />
            <Kpi label="duplicate" value={syncResult.duplicate} />
            <Kpi label="conflict" value={syncResult.conflict} />
          </div>
        )}
      </div>
      <div className="panel span-12">
        <h2>離線名單快照（hash）</h2>
        {!packageSummary ? (
          <EmptyState title="名單尚未下載" action="先選擇活動並下載名單後，才可預覽可核銷 token hash。" />
        ) : (
          <ResponsiveTable>
            <thead>
              <tr>
                <th>Ticket</th>
                <th>Employee</th>
                <th>token_hash</th>
              </tr>
            </thead>
            <tbody>
              {packageSummary.tickets.map((ticket) => (
                <tr key={ticket.ticket_id}>
                  <td className="mono-cell">{ticket.ticket_id}</td>
                  <td>{ticket.employee_id}</td>
                  <td className="mono-cell">{ticket.token_hash}</td>
                </tr>
              ))}
            </tbody>
          </ResponsiveTable>
        )}
        <h2>同步結果</h2>
        {syncResult && syncResult.results.length > 0 ? (
          <ResponsiveTable>
            <thead>
              <tr>
                <th>Ticket</th>
                <th>Employee</th>
                <th>Status</th>
                <th>Reason</th>
                <th>Scanned at</th>
              </tr>
            </thead>
            <tbody>
              {syncResult.results.map((result) => (
                <tr key={`${result.checkin_id || result.ticket_id || result.scanned_at}-${result.status}`}>
                  <td className="mono-cell">{result.ticket_id}</td>
                  <td>{result.employee_id}</td>
                  <td>
                    <StatusBadge tone={result.duplicate ? "warn" : result.status === "accepted" ? "ok" : "fail"}>{result.status}</StatusBadge>
                  </td>
                  <td>{result.conflict_reason || "—"}</td>
                  <td>{formatDate(result.scanned_at)}</td>
                </tr>
              ))}
            </tbody>
          </ResponsiveTable>
        ) : (
          <div className="empty-state">
            {syncResult && <p className="table-muted">本次無結果可顯示。</p>}
          </div>
        )}
      </div>
    </section>
  );
}
