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
}: Readonly<{
  staffID: string;
  onPackageReady: (stored: StoredCheckinPackage) => void;
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
  const deviceID =
    devicePreset === "custom" ? customDeviceID.trim() : devicePreset;

  useEffect(() => {
    void loadEvents();
    void restoreFromIDB();
  }, []);

  async function restoreFromIDB() {
    try {
      const active = await loadActivePackages();
      if (active.length > 0) {
        const latest = active.at(-1)!;
        setRestoredPkg(latest);
        onPackageReady(latest);
      }
    } catch {
      /* IDB unavailable */
    }
  }

  async function loadEvents() {
    setLoading(true);
    setMessage("");
    try {
      const nextEvents = await listAdminEvents();
      setEvents(nextEvents);
      if (!eventID) setEventID(nextEvents[0]?.event_id ?? "");
    } catch (error) {
      if (isOffline() && restoredPkg) {
        setMessage("離線模式：使用已下載的離線名單");
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
      setRestoredPkg(stored);
      onPackageReady(stored);
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
            <p>先下載活動票券清單，避免在離線端保留完整明文票券資訊。</p>
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
