import { useEffect, useState } from "react";
import {
  listAdminEvents,
  offlineCheckinPackage,
  syncOfflineCheckins,
} from "@/lib/api";
import type {
  EventSummary,
  OfflineCheckinPackage,
  OfflineCheckinSyncResponse,
} from "@/lib/api";
import { errorMessage, formatDate } from "@/lib/formatting";
import {
  Alert,
  BoundaryContext,
  EmptyState,
  Field,
  MetaList,
  ReadinessMessage,
  ResponsiveTable,
  SelectField,
  TextareaField,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useUrlTab } from "@/hooks/use-url-tab";
import { Button } from "@/components/ui/button";
import { devicePresetOptions } from "@/lib/ui/options";
import { fingerprint } from "@/lib/api/redaction";
import { OfflineResultStep } from "./offline-result-step";

type OfflineTab = "package" | "scan" | "results";
const offlineTabs = ["package", "scan", "results"] as const;

export function OfflineCheckinBoundaryPage() {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [eventID, setEventID] = useState("");
  const [devicePreset, setDevicePreset] = useState("gate-offline-1");
  const [customDeviceID, setCustomDeviceID] = useState("");
  const [batchForm, setBatchForm] = useState("");
  const [message, setMessage] = useState("");
  const [packageSummary, setPackageSummary] =
    useState<OfflineCheckinPackage | null>(null);
  const [syncResult, setSyncResult] =
    useState<OfflineCheckinSyncResponse | null>(null);
  const [activeTab, setActiveTab] = useUrlTab<OfflineTab>(
    "tab",
    offlineTabs,
    "package",
  );
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const deviceID =
    devicePreset === "custom" ? customDeviceID.trim() : devicePreset;
  const parsedBatchTokens = batchForm
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
  const ignoredBatchLines =
    batchForm.length === 0
      ? 0
      : batchForm.split("\n").filter((line) => !line.trim()).length;

  useEffect(() => {
    void loadEvents();
  }, []);

  useEffect(() => {
    if (!packageSummary && activeTab !== "package") setActiveTab("package");
    if (activeTab === "results" && !syncResult) {
      setActiveTab(packageSummary ? "scan" : "package");
    }
  }, [activeTab, packageSummary, setActiveTab, syncResult]);

  async function loadEvents() {
    setLoading(true);
    setMessage("");
    try {
      const nextEvents = await listAdminEvents();
      setEvents(nextEvents);
      if (!eventID) setEventID(nextEvents[0]?.event_id ?? "");
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  async function downloadPackage() {
    if (!eventID.trim() || !deviceID) {
      setMessage("請先選擇活動與驗票裝置。");
      return;
    }
    setBusy(true);
    setMessage("");
    setSyncResult(null);
    try {
      const next = await offlineCheckinPackage(eventID.trim(), deviceID);
      setPackageSummary(next);
      setBatchForm("");
      setSyncResult(null);
      setActiveTab("scan");
      setMessage(
        `已下載批次 ${next.batch_id}，共 ${next.ticket_count} 張票券。`,
      );
    } catch (error) {
      setMessage(errorMessage(error));
      setPackageSummary(null);
    } finally {
      setBusy(false);
    }
  }

  async function syncBatch() {
    if (!packageSummary || parsedBatchTokens.length === 0) {
      setMessage(packageSummary ? "掃描名單不能為空。" : "請先下載離線名單。");
      return;
    }
    setBusy(true);
    setMessage("");
    try {
      const scanned_at = new Date().toISOString();
      const result = await syncOfflineCheckins({
        batch_id: packageSummary.batch_id,
        event_id: packageSummary.event_id,
        device_id: packageSummary.device_id,
        package_signature: packageSummary.package_signature,
        scans: parsedBatchTokens.map((signed_token) => ({
          signed_token,
          scanned_at,
        })),
      });
      setSyncResult(result);
      setActiveTab("results");
      setMessage(
        `已同步 ${parsedBatchTokens.length} 筆掃描，成功 ${result.accepted}，衝突 ${result.conflict}。`,
      );
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="content-grid">
      <BoundaryContext
        title="離線驗票同步"
        description="選擇活動與設備下載簽章名單，再上傳簽章碼批次同步結果。"
        icon="wifiOff"
      />
      <Tabs
        className="panel span-12 focused-tabs"
        value={activeTab}
        onValueChange={(value) => setActiveTab(value as OfflineTab)}
      >
        <div className="section-heading">
          <div>
            <h2>離線驗票步驟</h2>
            <p>依序下載名單、輸入掃描簽章碼、確認同步結果。</p>
          </div>
          <TabsList>
            <TabsTrigger value="package">1 下載名單</TabsTrigger>
            <TabsTrigger value="scan" disabled={!packageSummary}>
              2 輸入掃描
            </TabsTrigger>
            <TabsTrigger value="results" disabled={!syncResult}>
              3 同步結果
            </TabsTrigger>
          </TabsList>
        </div>
        <TabsContent value="package">
          <OfflinePackageStep
            events={events}
            eventID={eventID}
            deviceID={deviceID}
            devicePreset={devicePreset}
            customDeviceID={customDeviceID}
            busy={busy}
            loading={loading}
            message={message}
            packageSummary={packageSummary}
            onCustomDeviceIDChange={setCustomDeviceID}
            onDevicePresetChange={setDevicePreset}
            onDownload={() => void downloadPackage()}
            onEventChange={setEventID}
            onLoadEvents={() => void loadEvents()}
          />
        </TabsContent>
        <TabsContent value="scan">
          <OfflineScanStep
            batchForm={batchForm}
            busy={busy}
            ignoredBatchLines={ignoredBatchLines}
            packageSummary={packageSummary}
            parsedBatchTokens={parsedBatchTokens}
            onBatchFormChange={setBatchForm}
            onSync={() => void syncBatch()}
          />
        </TabsContent>
        <TabsContent value="results">
          <OfflineResultStep syncResult={syncResult} />
        </TabsContent>
      </Tabs>
    </section>
  );
}

function OfflinePackageStep({
  busy,
  customDeviceID,
  deviceID,
  devicePreset,
  events,
  eventID,
  loading,
  message,
  packageSummary,
  onCustomDeviceIDChange,
  onDevicePresetChange,
  onDownload,
  onEventChange,
  onLoadEvents,
}: Readonly<{
  busy: boolean;
  customDeviceID: string;
  deviceID: string;
  devicePreset: string;
  events: EventSummary[];
  eventID: string;
  loading: boolean;
  message: string;
  packageSummary: OfflineCheckinPackage | null;
  onCustomDeviceIDChange: (value: string) => void;
  onDevicePresetChange: (value: string) => void;
  onDownload: () => void;
  onEventChange: (value: string) => void;
  onLoadEvents: () => void;
}>) {
  return (
    <div className="split-grid offline-package-grid">
      <div>
        <div className="section-heading">
          <div>
            <h2>離線名單</h2>
            <p>先下載活動票券清單，避免在離線端保留完整明文票券資訊。</p>
          </div>
          <Button
            variant="outline"
            type="button"
            onClick={onLoadEvents}
            disabled={loading}
          >
            重新載入活動
          </Button>
        </div>
        <SelectField
          label="活動"
          value={eventID}
          options={[
            { value: "", label: "選擇活動" },
            ...events.map((event) => ({
              value: event.event_id,
              label: event.title,
            })),
          ]}
          onChange={onEventChange}
        />
        <div className="form-grid mt-14">
          <SelectField
            label="驗票裝置"
            value={devicePreset}
            options={devicePresetOptions}
            onChange={onDevicePresetChange}
          />
          {devicePreset === "custom" && (
            <Field
              autoComplete="off"
              label="自訂裝置代號"
              name="offline-custom-device-id"
              value={customDeviceID}
              onChange={onCustomDeviceIDChange}
            />
          )}
          <Button
            type="button"
            onClick={onDownload}
            disabled={busy || loading || !eventID || !deviceID}
          >
            <Icon name="wifiOff" />
            {busy ? "下載中" : "下載離線名單"}
          </Button>
        </div>
        <Alert tone={packageSummary ? "ok" : "info"}>
          {message || "請先下載離線名單。"}
        </Alert>
      </div>
      <div className="offline-preview">
        {packageSummary ? (
          <MetaList
            className="vertical detail-panel"
            rows={[
              ["批次編號", packageSummary.batch_id],
              ["有效至", formatDate(packageSummary.valid_until)],
              [
                "名單簽章指紋",
                `#${fingerprint(packageSummary.package_signature)}`,
                "mono-cell",
              ],
              ["可同步票券", packageSummary.ticket_count],
            ]}
          />
        ) : (
          <EmptyState
            title="名單尚未下載"
            action="下載後會顯示批次、有效期限與簽章摘要。"
          />
        )}
      </div>
    </div>
  );
}

function OfflineScanStep({
  batchForm,
  busy,
  ignoredBatchLines,
  packageSummary,
  parsedBatchTokens,
  onBatchFormChange,
  onSync,
}: Readonly<{
  batchForm: string;
  busy: boolean;
  ignoredBatchLines: number;
  packageSummary: OfflineCheckinPackage | null;
  parsedBatchTokens: string[];
  onBatchFormChange: (value: string) => void;
  onSync: () => void;
}>) {
  return (
    <div className="split-grid">
      <div>
        <div className="section-heading">
          <div>
            <h2>掃描批次同步</h2>
            <p>每行輸入一個簽章碼；未提供掃描時間時以目前時間補上。</p>
          </div>
        </div>
        <TextareaField
          label="掃描批次"
          name="offline-scan-batch"
          value={batchForm}
          rows={9}
          onChange={onBatchFormChange}
        />
        <ReadinessMessage
          tone={parsedBatchTokens.length > 0 ? "ok" : "warn"}
          label={
            parsedBatchTokens.length > 0
              ? `${parsedBatchTokens.length} 筆可同步`
              : "尚無簽章碼"
          }
          message={
            ignoredBatchLines > 0
              ? `${ignoredBatchLines} 行空白會被忽略。`
              : "每行貼上一個簽章碼。"
          }
        />
        <Button
          type="button"
          onClick={onSync}
          disabled={busy || !packageSummary || parsedBatchTokens.length === 0}
        >
          <Icon name="scan" />
          {busy ? "同步中" : "同步名單"}
        </Button>
      </div>
      {packageSummary ? (
        <ResponsiveTable label="離線驗票同步結果">
          <thead>
            <tr>
              <th>票券</th>
              <th>持票人</th>
              <th>同行</th>
              <th>簽章指紋</th>
            </tr>
          </thead>
          <tbody>
            {packageSummary.tickets.map((ticket) => (
              <tr key={ticket.ticket_id}>
                <td className="mono-cell">{ticket.ticket_id}</td>
                <td>
                  {ticket.holder?.display_name || ticket.employee_id}
                  <span className="table-muted">
                    {[
                      ticket.employee_id,
                      ticket.holder?.department,
                      ticket.holder?.city,
                    ]
                      .filter(Boolean)
                      .join(" / ") || ticket.employee_id}
                  </span>
                </td>
                <td>{ticket.family_count} 人</td>
                <td className="mono-cell">#{fingerprint(ticket.token_hash)}</td>
              </tr>
            ))}
          </tbody>
        </ResponsiveTable>
      ) : (
        <EmptyState
          title="尚未下載名單"
          action="請先回到第一步下載離線名單。"
        />
      )}
    </div>
  );
}
