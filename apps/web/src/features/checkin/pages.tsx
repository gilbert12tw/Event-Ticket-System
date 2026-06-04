import { type KeyboardEvent, useEffect, useRef, useState } from "react";
import { ApiError, checkIn, listAdminEvents } from "@/lib/api";
import type { CheckinResponse, EventSummary } from "@/lib/api";
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
import {
  isCheckinReady,
  selectedCheckinEventID,
  shouldSubmitDetectedToken,
  type RecentScan,
} from "./checkin-flow";

export { CheckinResult } from "./checkin-result";
export { OfflineCheckinBoundaryPage } from "./offline-page";

export function CheckinPage() {
  const [token, setToken] = useState(
    () => readHistoryCheckinToken() || getDemoCheckinToken(),
  );
  const [devicePreset, setDevicePreset] = useState("gate-1");
  const [customDeviceID, setCustomDeviceID] = useState("");
  const [holderMismatchReason, setHolderMismatchReason] = useState("");
  const [result, setResult] = useState<CheckinResponse | null>(null);
  const [message, setMessage] = useState("");
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [eventID, setEventID] = useState("");
  const [busy, setBusy] = useState(false);
  const resultRef = useRef<HTMLDivElement>(null);
  const lastScanRef = useRef<RecentScan | null>(null);
  const recentToken = hasDemoCheckinToken() ? getDemoCheckinToken() : "";
  const deviceID =
    devicePreset === "custom" ? customDeviceID.trim() : devicePreset;
  const selectedEventID =
    eventID === "" ? (events[0]?.event_id ?? "") : eventID;
  const selectedEvent = events.find(
    (event) => event.event_id === selectedEventID,
  );

  async function submit(nextToken = token) {
    const signedToken = nextToken.trim();
    if (
      !isCheckinReady({
        token: signedToken,
        deviceID,
        eventID: selectedEventID,
      }) ||
      busy
    ) {
      return;
    }
    setBusy(true);
    setMessage("");
    setResult(null);
    try {
      const response = await checkIn(
        signedToken,
        deviceID,
        selectedEventID.trim(),
        holderMismatchReason.trim(),
      );
      setResult(response);
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

  function readinessMessage() {
    if (!selectedEventID.trim()) {
      return "目前沒有可驗票活動，請確認活動是否已載入。";
    }
    if (tokenOnlyReady) {
      return "請選擇或填寫裝置代號。";
    }
    return "請掃描 QR code；手動貼上只作為備援。";
  }

  useEffect(() => {
    listAdminEvents()
      .then((nextEvents) => {
        setEvents(nextEvents);
        setEventID((current) => selectedCheckinEventID(nextEvents, current));
      })
      .catch((error) => setMessage(errorMessage(error)));
  }, []);

  useEffect(() => {
    if (result || message) resultRef.current?.focus();
  }, [message, result]);

  const tokenReady = isCheckinReady({
    token,
    deviceID,
    eventID: selectedEventID,
  });
  const tokenOnlyReady = token.trim().length > 0;

  function handleDetectedToken(detectedToken: string) {
    const tokenText = detectedToken.trim();
    const nowMs = Date.now();
    if (
      !shouldSubmitDetectedToken({
        detectedToken: tokenText,
        lastScan: lastScanRef.current,
        nowMs,
      })
    ) {
      setMessage("已忽略重複掃描，請等待目前驗票結果。");
      return;
    }
    lastScanRef.current = { token: tokenText, scannedAtMs: nowMs };
    setToken(tokenText);
    if (
      isCheckinReady({ token: tokenText, deviceID, eventID: selectedEventID })
    ) {
      void submit(tokenText);
      return;
    }
    setMessage("已讀取 QR code，請先確認活動已載入且裝置代號可用。");
  }

  function handleTokenKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key !== "Enter" || !tokenReady) return;
    event.preventDefault();
    event.currentTarget.form?.requestSubmit();
  }

  return (
    <section className="content-grid checkin-workspace">
      <Card asChild className="panel span-6 checkin-form">
        <form
          aria-busy={busy}
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <div className="section-heading">
            <div>
              <h2>線上驗票</h2>
              <p>當前活動會自動鎖定；掃描 QR code 後立即驗票。</p>
            </div>
          </div>
          <MobileQrScanner onTokenDetected={handleDetectedToken} />
          <CompactStatsBar
            items={[
              { label: "活動", value: selectedEvent?.title || "未選擇" },
              { label: "狀態", value: selectedEvent?.status || "未載入" },
              { label: "裝置", value: deviceID || "未設定" },
              { label: "模式", value: "線上" },
            ]}
            label="驗票摘要"
          />
          <div className="helper-strip" aria-label="目前驗票活動">
            <StatusBadge tone={selectedEvent ? "ok" : "warn"}>
              當前活動
            </StatusBadge>
            <span>{selectedEvent?.title || "尚未載入可驗票活動"}</span>
            {events.length > 1 && (
              <span className="table-muted">
                已自動選定活動時間最接近現在的活動。
              </span>
            )}
          </div>
          <Field
            autoComplete="off"
            label="掃描或貼上票券"
            name="signed-token"
            value={token}
            onChange={setToken}
            onKeyDown={handleTokenKeyDown}
            required
            hint="QR 掃描成功後會自動驗票；長簽章碼可展開手動貼上。"
          />
          <details className="advanced-filter">
            <summary>
              手動貼上 <span>進階</span>
            </summary>
            <div className="mt-14">
              <TextareaField
                label="完整簽章碼"
                value={token}
                onChange={setToken}
                rows={5}
              />
              <TextareaField
                label="持票人不符原因"
                value={holderMismatchReason}
                onChange={setHolderMismatchReason}
                rows={3}
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
                ? "已偵測當前活動、簽章碼與裝置代號；掃描 QR 會自動送出。"
                : readinessMessage()
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
          <Kpi label="活動" value={selectedEvent?.title || "未選擇"} />
          <Kpi label="裝置" value={deviceID || "未設定"} />
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
