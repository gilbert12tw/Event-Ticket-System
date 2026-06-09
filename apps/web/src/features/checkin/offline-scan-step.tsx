import { useCallback, useRef, useState } from "react";
import type {
  OfflineScanRecord,
  StoredCheckinPackage,
} from "@/lib/offline/checkin-store";
import { addScanRecord } from "@/lib/offline/checkin-store";
import {
  computeScanCounts,
  isPackageExpired,
  judgeOfflineScan,
} from "@/lib/offline/checkin-logic";
import { hashToken } from "@/lib/offline/hash";
import {
  Alert,
  Kpi,
  ReadinessMessage,
  ResponsiveTable,
  StatusBadge,
  TextareaField,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";
import { MobileQrScanner } from "./mobile-qr-scanner";

export function OfflineScanStep({
  stored,
  onScansChanged,
}: Readonly<{
  stored: StoredCheckinPackage;
  onScansChanged: (scans: OfflineScanRecord[]) => void;
}>) {
  const [lastResult, setLastResult] = useState<OfflineScanRecord | null>(null);
  const [manualInput, setManualInput] = useState("");
  const [processing, setProcessing] = useState(false);
  const scansRef = useRef(stored.scans);
  scansRef.current = stored.scans;

  const pkg = stored.package;
  const expired = isPackageExpired(pkg);
  const counts = computeScanCounts(stored.scans);
  const isClosed = stored.status === "synced";

  const handleToken = useCallback(
    async (token: string) => {
      if (processing || isClosed) return;
      const trimmed = token.trim();
      if (!trimmed) return;

      setProcessing(true);
      try {
        const judgment = await judgeOfflineScan(trimmed, pkg, scansRef.current);
        const tokenHash = await hashToken(trimmed);
        const record: OfflineScanRecord = {
          local_scan_id: crypto.randomUUID(),
          signed_token: trimmed,
          token_hash: tokenHash,
          scanned_at: new Date().toISOString(),
          local_status: judgment.local_status,
          local_reason: judgment.local_reason,
          matched_ticket: judgment.matched_ticket,
          sync_status: "queued",
          updated_at: new Date().toISOString(),
        };

        await addScanRecord(stored.batch_id, record);
        const updated = [...scansRef.current, record];
        scansRef.current = updated;
        onScansChanged(updated);
        setLastResult(record);
      } catch {
        /* scan processing error — ignore for demo */
      } finally {
        setProcessing(false);
      }
    },
    [pkg, stored.batch_id, onScansChanged, processing, isClosed],
  );

  async function handleManualBatch() {
    const tokens = manualInput
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean);
    for (const t of tokens) {
      await handleToken(t);
    }
    setManualInput("");
  }

  return (
    <div className="split-grid">
      <div>
        <div className="section-heading">
          <div>
            <h2>掃描驗票</h2>
            <p>使用相機掃描 QR 碼或手動輸入簽章碼。</p>
          </div>
        </div>

        {isClosed && (
          <Alert tone="warn">
            此批次已同步完成，不可再新增掃描。請重新下載離線名單。
          </Alert>
        )}

        {expired && !isClosed && (
          <Alert tone="warn">
            離線名單已過期，不可再新增掃描。請連線後重新下載。
          </Alert>
        )}

        {!expired && !isClosed && (
          <>
            <MobileQrScanner onTokenDetected={handleToken} />

            <details className="mt-14">
              <summary>手動輸入簽章碼</summary>
              <div className="mt-8">
                <TextareaField
                  label="簽章碼批次"
                  name="offline-manual-tokens"
                  value={manualInput}
                  rows={5}
                  onChange={setManualInput}
                />
                <Button
                  type="button"
                  onClick={() => void handleManualBatch()}
                  disabled={processing || !manualInput.trim()}
                >
                  <Icon name="scan" />
                  加入掃描
                </Button>
              </div>
            </details>
          </>
        )}

        {lastResult && <ScanResultBanner record={lastResult} />}

        <div className="kpi-row mt-14">
          <Kpi label="本地通過" value={counts.accepted} />
          <Kpi label="重複" value={counts.duplicate} />
          <Kpi label="衝突" value={counts.conflict} />
          <Kpi label="待同步" value={counts.queued} />
          <Kpi label="已同步" value={counts.synced} />
          {counts.failed > 0 && <Kpi label="同步失敗" value={counts.failed} />}
        </div>
      </div>

      <div>
        {stored.scans.length > 0 ? (
          <ResponsiveTable label="掃描紀錄">
            <thead>
              <tr>
                <th>狀態</th>
                <th>持票人</th>
                <th>同行</th>
                <th>掃描時間</th>
                <th>同步</th>
              </tr>
            </thead>
            <tbody>
              {[...stored.scans].reverse().map((scan) => (
                <tr key={scan.local_scan_id}>
                  <td>
                    <ScanStatusBadge record={scan} />
                  </td>
                  <td>
                    {scan.matched_ticket?.holder?.display_name ||
                      scan.matched_ticket?.employee_id ||
                      "-"}
                  </td>
                  <td>{scan.matched_ticket?.family_count ?? "-"}</td>
                  <td className="mono-cell">
                    {new Date(scan.scanned_at).toLocaleTimeString()}
                  </td>
                  <td>
                    <SyncStatusLabel status={scan.sync_status} />
                  </td>
                </tr>
              ))}
            </tbody>
          </ResponsiveTable>
        ) : (
          <ReadinessMessage
            tone="warn"
            label="尚無掃描"
            message="掃描 QR 或手動輸入簽章碼後，紀錄會出現在這裡。"
          />
        )}
      </div>
    </div>
  );
}

function ScanResultBanner({ record }: Readonly<{ record: OfflineScanRecord }>) {
  if (record.local_status === "accepted") {
    return (
      <Alert tone="ok">
        本地通過，等待同步 —{" "}
        {record.matched_ticket?.holder?.display_name || "未知持票人"}
        {record.matched_ticket?.family_count
          ? ` · 同行 ${record.matched_ticket.family_count} 人`
          : ""}
      </Alert>
    );
  }
  if (record.local_status === "duplicate") {
    return <Alert tone="warn">重複掃描 — 此票券已在本批次掃描過。</Alert>;
  }
  const reasonMap: Record<string, string> = {
    not_in_offline_package: "衝突：不在離線名單",
    package_expired: "離線名單已過期",
    empty_token: "空白簽章碼",
  };
  return (
    <Alert tone="fail">
      {reasonMap[record.local_reason ?? ""] ?? "衝突：驗票失敗"}
    </Alert>
  );
}

function ScanStatusBadge({ record }: Readonly<{ record: OfflineScanRecord }>) {
  if (record.local_status === "accepted")
    return <StatusBadge tone="ok">本地通過</StatusBadge>;
  if (record.local_status === "duplicate")
    return <StatusBadge tone="warn">重複</StatusBadge>;
  return <StatusBadge tone="fail">衝突</StatusBadge>;
}

function SyncStatusLabel({
  status,
}: Readonly<{ status: OfflineScanRecord["sync_status"] }>) {
  const map: Record<string, string> = {
    queued: "待同步",
    syncing: "同步中",
    synced: "已同步",
    sync_failed: "同步失敗",
  };
  return <span className="table-muted">{map[status] ?? status}</span>;
}
