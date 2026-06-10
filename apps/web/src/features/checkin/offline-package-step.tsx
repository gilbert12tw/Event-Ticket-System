import { useEffect, useState } from "react";
import { listAdminEvents, offlineCheckinPackage } from "@/lib/api";
import type { EventSummary, OfflineCheckinPackage } from "@/lib/api";
import type { StoredCheckinPackage } from "@/lib/offline/checkin-store";
import { loadActivePackages, savePackage } from "@/lib/offline/checkin-store";
import { isOffline } from "@/lib/offline/auth-cache";
import { isPackageExpired } from "@/lib/offline/checkin-logic";
import { errorMessage, formatDate } from "@/lib/formatting";
import {
  Alert,
  EmptyState,
  Field,
  MetaList,
  SelectField,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";
import { devicePresetOptions } from "@/lib/ui/options";
import { fingerprint } from "@/lib/api/redaction";

export function OfflinePackageStep({
  staffID,
  onPackageReady,
  onPackageRestored,
}: Readonly<{
  staffID: string;
  onPackageReady: (stored: StoredCheckinPackage) => void;
  onPackageRestored?: (stored: StoredCheckinPackage) => void;
}>) {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [eventID, setEventID] = useState("");
  const [devicePreset, setDevicePreset] = useState("gate-offline-1");
  const [customDeviceID, setCustomDeviceID] = useState("");
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [restoredPkg, setRestoredPkg] = useState<StoredCheckinPackage | null>(
    null,
  );
  const [activePackages, setActivePackages] = useState<StoredCheckinPackage[]>(
    [],
  );
  const deviceID =
    devicePreset === "custom" ? customDeviceID.trim() : devicePreset;

  useEffect(() => {
    void loadEvents();
    void restoreFromIDB();
  }, []);

  async function restoreFromIDB() {
    try {
      const active = await loadActivePackages();
      setActivePackages(active);
      // Auto-select only when unambiguous; with multiple batches let the user
      // choose explicitly instead of silently picking one. Restoring must not
      // navigate, or revisiting this step bounces straight back to the scan
      // tab and re-downloading another event becomes impossible.
      if (active.length === 1) {
        setRestoredPkg(active[0]);
        onPackageRestored?.(active[0]);
      }
    } catch {
      /* IDB unavailable */
    }
  }

  function selectActivePackage(pkg: StoredCheckinPackage) {
    setRestoredPkg(pkg);
    onPackageReady(pkg);
  }

  async function loadEvents() {
    setLoading(true);
    setMessage("");
    try {
      const nextEvents = await listAdminEvents();
      setEvents(nextEvents);
      if (!eventID) setEventID(nextEvents[0]?.event_id ?? "");
    } catch (error) {
      if (isOffline()) {
        setMessage("離線模式：無法載入活動清單，可使用已下載的離線名單。");
        return;
      }
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
    try {
      const next = await offlineCheckinPackage(eventID.trim(), deviceID);
      const stored = await savePackage(next, staffID);
      setActivePackages(await loadActivePackages());
      selectActivePackage(stored);
      setMessage(
        `已下載批次 ${next.batch_id}，共 ${next.ticket_count} 張票券。`,
      );
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  const pkg = restoredPkg?.package ?? null;
  const expired = pkg ? isPackageExpired(pkg) : false;

  return (
    <div className="split-grid offline-package-grid">
      <div>
        <div className="section-heading">
          <div>
            <h2>離線名單</h2>
            <p>
              先下載活動票券名單以供離線比對；同步完成後會自動清除裝置上的明文票券與名單資料。
            </p>
          </div>
          <Button
            variant="outline"
            type="button"
            onClick={() => void loadEvents()}
            disabled={loading}
          >
            重新載入活動
          </Button>
        </div>
        {activePackages.length > 1 && (
          <SelectField
            label="已下載離線名單"
            value={restoredPkg?.batch_id ?? ""}
            options={[
              { value: "", label: "選擇要使用的批次" },
              ...activePackages.map((p) => ({
                value: p.batch_id,
                label: `批次 ${p.batch_id}（${p.package.ticket_count} 張）`,
              })),
            ]}
            onChange={(value) => {
              const chosen = activePackages.find((p) => p.batch_id === value);
              if (chosen) selectActivePackage(chosen);
            }}
          />
        )}
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
          onChange={setEventID}
        />
        <div className="form-grid mt-14">
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
              name="offline-custom-device-id"
              value={customDeviceID}
              onChange={setCustomDeviceID}
            />
          )}
          <Button
            type="button"
            onClick={() => void downloadPackage()}
            disabled={busy || loading || !eventID || !deviceID}
          >
            <Icon name="wifiOff" />
            {busy ? "下載中" : "下載離線名單"}
          </Button>
        </div>
        <PackageAlert pkg={pkg} expired={expired} message={message} />
      </div>
      <div className="offline-preview">
        {pkg ? (
          <MetaList
            className="vertical detail-panel"
            rows={[
              ["批次編號", pkg.batch_id],
              ["有效至", formatDate(pkg.valid_until)],
              [
                "名單簽章指紋",
                `#${fingerprint(pkg.package_signature)}`,
                "mono-cell",
              ],
              ["可同步票券", pkg.ticket_count],
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

function PackageAlert({
  pkg,
  expired,
  message,
}: Readonly<{
  pkg: OfflineCheckinPackage | null;
  expired: boolean;
  message: string;
}>) {
  if (expired) {
    return <Alert tone="warn">離線名單已過期，請連線後重新下載。</Alert>;
  }
  if (pkg) {
    return <Alert tone="ok">{message || "請先下載離線名單。"}</Alert>;
  }
  return <Alert tone="info">{message || "請先下載離線名單。"}</Alert>;
}
