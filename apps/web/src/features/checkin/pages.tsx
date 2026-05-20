import { useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import { ApiError, checkIn, reports } from "@/lib/api";
import type { CheckinResponse, ReportRow } from "@/lib/api";
import { errorMessage } from "@/lib/formatting";
import {
  Alert,
  CompactStatsBar,
  EmptyState,
  Field,
  Kpi,
  ReadinessMessage,
  SelectField,
  StatusBadge,
  TextareaField,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { devicePresetOptions } from "@/lib/ui/options";
import {
  getDemoCheckinToken,
  hasDemoCheckinToken,
  readHistoryCheckinToken,
} from "./checkin-token";
import { CheckinResult } from "./checkin-result";
import { MobileQrScanner } from "./mobile-qr-scanner";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";

export { CheckinResult } from "./checkin-result";
export { OfflineCheckinBoundaryPage } from "./offline-page";

export function CheckinPage() {
  const [token, setToken] = useState(
    () => readHistoryCheckinToken() || getDemoCheckinToken(),
  );
  const [devicePreset, setDevicePreset] = useState("gate-1");
  const [customDeviceID, setCustomDeviceID] = useState("");
  const [result, setResult] = useState<CheckinResponse | null>(null);
  const [message, setMessage] = useState("");
  const [rows, setRows] = useState<ReportRow[]>([]);
  const [busy, setBusy] = useState(false);
  const resultRef = useRef<HTMLDivElement>(null);
  const recentToken = hasDemoCheckinToken() ? getDemoCheckinToken() : "";
  const deviceID =
    devicePreset === "custom" ? customDeviceID.trim() : devicePreset;

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    setResult(null);
    try {
      const response = await checkIn(token.trim(), deviceID);
      setResult(response);
      void reports()
        .then(setRows)
        .catch(() => setRows([]));
    } catch (error) {
      if (error instanceof ApiError && error.response.data) {
        setResult(error.response.data as CheckinResponse);
      } else {
        setResult({
          checkin_id: "",
          ticket_id: "無法辨識",
          event_id: "",
          employee_id: "未知",
          status: "rejected",
          reason_code: "invalid_ticket",
          scanned_at: new Date().toISOString(),
          conflict_reason: "invalid_ticket",
          rejection_message: errorMessage(error),
          duplicate: false,
          holder: null,
          family_count: 0,
        });
      }
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    reports()
      .then(setRows)
      .catch(() => setRows([]));
  }, []);

  useEffect(() => {
    if (result || message) resultRef.current?.focus();
  }, [message, result]);

  const total = rows.reduce(
    (acc, row) => ({
      capacity: acc.capacity + (row.capacity ?? 0),
      checkedIn: acc.checkedIn + row.checkin_count,
    }),
    { capacity: 0, checkedIn: 0 },
  );
  const tokenReady = token.trim().length > 0 && deviceID.length > 0;
  const tokenOnlyReady = token.trim().length > 0;
  const checkinRate =
    total.capacity > 0
      ? `${Math.round((total.checkedIn / total.capacity) * 100)}%`
      : "0%";

  return (
    <section className="content-grid checkin-workspace">
      <Card asChild className="panel span-6 checkin-form">
        <form aria-busy={busy} onSubmit={(event) => void submit(event)}>
          <div className="section-heading">
            <div>
              <h2>線上驗票</h2>
              <p>送出後會立即顯示可入場、重複掃描或拒絕原因。</p>
            </div>
          </div>
          <MobileQrScanner onTokenDetected={setToken} />
          <CompactStatsBar
            items={[
              { label: "已入場", value: total.checkedIn },
              { label: "總名額", value: total.capacity },
              { label: "入場率", value: checkinRate },
              { label: "裝置", value: deviceID || "未設定" },
            ]}
            label="驗票摘要"
          />
          <Field
            autoComplete="off"
            label="掃描或貼上票券"
            name="signed-token"
            value={token}
            onChange={setToken}
            onKeyDown={(event) => {
              if (event.key === "Enter" && tokenReady) {
                event.preventDefault();
                event.currentTarget.form?.requestSubmit();
              }
            }}
            required
            hint="掃描器送出 Enter 時會直接驗票；長簽章碼可展開手動貼上。"
          />
          <details className="advanced-filter">
            <summary>
              手動貼上
              <span>進階</span>
            </summary>
            <div className="mt-14">
              <TextareaField
                label="完整簽章碼"
                value={token}
                onChange={setToken}
                rows={5}
              />
            </div>
          </details>
          <SelectField
            label="驗票裝置"
            value={devicePreset}
            options={devicePresetOptions}
            onChange={setDevicePreset}
          />
          {devicePreset === "custom" && (
            <Field
              autoComplete="off"
              label="自訂裝置代號"
              name="custom-device-id"
              value={customDeviceID}
              onChange={setCustomDeviceID}
              required
            />
          )}
          <ReadinessMessage
            tone={tokenReady ? "ok" : "warn"}
            label={tokenReady ? "可驗票" : "資料未齊"}
            message={
              tokenReady
                ? "已偵測簽章碼與裝置代號，可以送出驗票。"
                : tokenOnlyReady
                  ? "請選擇或填寫裝置代號。"
                  : "請掃描或貼上票券簽章碼。"
            }
          />
          <div className="helper-strip">
            <StatusBadge tone={recentToken ? "info" : "neutral"}>
              最近票券
            </StatusBadge>
            <span>
              {recentToken
                ? "可載入最近票券簽章碼。"
                : "取得票券後會保留最近簽章碼。"}
            </span>
            <Button
              variant="outline"
              type="button"
              onClick={() => setToken(recentToken)}
              disabled={!recentToken}
            >
              使用最近票券
            </Button>
          </div>
          <Button type="submit" disabled={busy || !tokenReady}>
            <Icon name="scan" />
            {busy ? "驗票中" : "送出驗票"}
          </Button>
        </form>
      </Card>
      <Card
        className="panel span-6"
        ref={resultRef}
        role="region"
        aria-labelledby="checkin-result-title"
        aria-live="polite"
        tabIndex={-1}
      >
        <h2 id="checkin-result-title">驗票結果</h2>
        <div className="kpi-row">
          <Kpi label="已入場" value={total.checkedIn} />
          <Kpi label="總名額" value={total.capacity} />
        </div>
        {message && (
          <Alert tone={message.includes("已核銷") ? "warn" : "fail"}>
            {message}
          </Alert>
        )}
        {!result && !message && (
          <EmptyState
            title="等待掃描"
            action="掃描或貼上票券簽章碼後送出，結果會在此顯示。"
          />
        )}
        {result && <CheckinResult result={result} />}
      </Card>
    </section>
  );
}
