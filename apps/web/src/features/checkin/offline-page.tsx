import { useEffect, useMemo, useState } from "react";
import type {
  OfflineScanRecord,
  StoredCheckinPackage,
} from "@/lib/offline/checkin-store";
import { loadPackage } from "@/lib/offline/checkin-store";
import { loadCachedAuthSession } from "@/lib/offline/auth-cache";
import { Alert, BoundaryContext, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { useUrlTab } from "@/hooks/use-url-tab";
import { OfflinePackageStep } from "./offline-package-step";
import { OfflineScanStep } from "./offline-scan-step";
import { OfflineResultStep } from "./offline-result-step";
import { useOfflineSync } from "./use-offline-sync";

type OfflineTab = "package" | "scan" | "results";
const offlineTabs = ["package", "scan", "results"] as const;

export function OfflineCheckinBoundaryPage() {
  const [stored, setStored] = useState<StoredCheckinPackage | null>(null);
  const [online, setOnline] = useState(navigator.onLine);
  const [activeTab, setActiveTab] = useUrlTab<OfflineTab>(
    "tab",
    offlineTabs,
    "package",
  );

  const batchID = stored?.batch_id;
  const { syncState, syncResult, syncError, syncNow, canSync } = useOfflineSync(
    batchID,
    handleAutoSynced,
  );

  useEffect(() => {
    const goOnline = () => setOnline(true);
    const goOffline = () => setOnline(false);
    globalThis.addEventListener("online", goOnline);
    globalThis.addEventListener("offline", goOffline);
    return () => {
      globalThis.removeEventListener("online", goOnline);
      globalThis.removeEventListener("offline", goOffline);
    };
  }, []);

  useEffect(() => {
    if (!stored && activeTab !== "package") setActiveTab("package");
    if (activeTab === "results" && stored?.scans.length === 0) {
      setActiveTab("scan");
    }
  }, [activeTab, stored, setActiveTab]);

  function handlePackageReady(next: StoredCheckinPackage) {
    setStored(next);
    setActiveTab("scan");
  }

  function handlePackageRestored(next: StoredCheckinPackage) {
    setStored(next);
  }

  function handleAutoSynced(refreshed: StoredCheckinPackage) {
    setStored(refreshed);
    if (refreshed.status === "synced") setActiveTab("results");
  }

  function handleScansChanged(scans: OfflineScanRecord[]) {
    if (!stored) return;
    setStored({ ...stored, scans });
  }

  async function handleSync() {
    const updated = await syncNow();
    if (updated) {
      setStored(updated);
      setActiveTab("results");
    } else if (batchID) {
      const refreshed = await loadPackage(batchID);
      if (refreshed) setStored(refreshed);
    }
  }

  const staffID = useMemo(() => loadCachedAuthSession()?.actor.id ?? "", []);

  return (
    <section className="content-grid">
      <BoundaryContext
        title="離線驗票"
        description="下載簽章名單後可斷網掃描，恢復連線再同步。"
        icon="wifiOff"
      />

      <div className="span-12" style={{ display: "flex", gap: "0.5rem" }}>
        <StatusBadge tone={online ? "ok" : "neutral"}>
          {online ? "線上" : "離線"}
        </StatusBadge>
        {stored && (
          <StatusBadge tone={stored.status === "synced" ? "ok" : "info"}>
            {stored.status === "synced" ? "已同步" : `批次 ${stored.batch_id}`}
          </StatusBadge>
        )}
      </div>

      <Tabs
        className="panel span-12 focused-tabs"
        value={activeTab}
        onValueChange={(value) => setActiveTab(value as OfflineTab)}
      >
        <div className="section-heading">
          <div>
            <h2>離線驗票步驟</h2>
            <p>依序下載名單、掃描驗票、同步結果。</p>
          </div>
          <TabsList>
            <TabsTrigger value="package">1 下載名單</TabsTrigger>
            <TabsTrigger value="scan" disabled={!stored}>
              2 掃描驗票
            </TabsTrigger>
            <TabsTrigger
              value="results"
              disabled={!stored || stored.scans.length === 0}
            >
              3 同步結果
            </TabsTrigger>
          </TabsList>
        </div>
        <TabsContent value="package">
          <OfflinePackageStep
            staffID={staffID}
            onPackageReady={handlePackageReady}
            onPackageRestored={handlePackageRestored}
          />
        </TabsContent>
        <TabsContent value="scan">
          {stored && (
            <OfflineScanStep
              stored={stored}
              onScansChanged={handleScansChanged}
              onGoToResults={() => setActiveTab("results")}
            />
          )}
        </TabsContent>
        <TabsContent value="results">
          {syncError && <Alert tone="fail">{syncError}</Alert>}

          <div
            style={{
              display: "flex",
              gap: "0.5rem",
              alignItems: "center",
              marginBottom: "1rem",
            }}
          >
            <Button
              type="button"
              onClick={() => void handleSync()}
              disabled={!canSync || syncState === "syncing"}
            >
              <Icon name="refresh" />
              {syncState === "syncing" ? "同步中…" : "立即同步"}
            </Button>
            {syncState === "synced" && (
              <StatusBadge tone="ok">同步完成</StatusBadge>
            )}
          </div>

          <OfflineResultStep syncResult={syncResult} />
        </TabsContent>
      </Tabs>
    </section>
  );
}
