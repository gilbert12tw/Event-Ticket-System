import { useEffect, useMemo, useState } from "react";
import { createReportExport, getReportExport, reports } from "@/lib/api";
import type { ReportExport, ReportRow } from "@/lib/api";
import { errorMessage } from "@/lib/formatting";
import {
  Alert,
  CompactStatsBar,
  EmptyState,
  Field,
  ProgressMeter,
  ResponsiveTable,
  SelectField,
  StatusBadge,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useUrlTab } from "@/hooks/use-url-tab";
import { reportExportStatusView, reportPresetOptions } from "@/lib/ui/options";
import { Button } from "@/components/ui/button";

type ReportTab = "participation" | "exports";
const reportTabs = ["participation", "exports"] as const;

export function HrReportsPage() {
  const [reportRows, setReportRows] = useState<ReportRow[]>([]);
  const [lastExport, setLastExport] = useState<ReportExport | null>(null);
  const [filter, setFilter] = useState("");
  const [reportPreset, setReportPreset] = useState("participation");
  const [message, setMessage] = useState("");
  const [exporting, setExporting] = useState(false);
  const [activeTab, setActiveTab] = useUrlTab<ReportTab>(
    "tab",
    reportTabs,
    "participation",
  );

  async function refresh() {
    setMessage("");
    try {
      setReportRows(await reports());
    } catch (error) {
      setMessage(errorMessage(error));
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function exportParticipationReport() {
    setExporting(true);
    setMessage("");
    try {
      const requested = await createReportExport({
        report_type: "participation",
      });
      setLastExport(requested);
      for (let attempt = 0; attempt < 12; attempt++) {
        const current = await getReportExport(requested.export_id);
        setLastExport(current);
        if (current.status === "ready" || current.status === "failed") return;
        await new Promise((resolve) => window.setTimeout(resolve, 500));
      }
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setExporting(false);
    }
  }

  const totals = useMemo(
    () =>
      reportRows.reduce(
        (acc, row) => ({
          confirmed: acc.confirmed + row.confirmed_count,
          waitlist: acc.waitlist + row.waitlist_count,
          tickets: acc.tickets + row.ticket_count,
          checkins: acc.checkins + row.checkin_count,
        }),
        { confirmed: 0, waitlist: 0, tickets: 0, checkins: 0 },
      ),
    [reportRows],
  );

  const filteredReports = reportRows.filter((row) => {
    const needle = filter.toLowerCase();
    const matchesText = `${row.title} ${row.event_id}`
      .toLowerCase()
      .includes(needle);
    return matchesText && reportPresetMatches(row, reportPreset);
  });
  const attendanceRate =
    totals.confirmed > 0
      ? `${Math.round((totals.checkins / totals.confirmed) * 100)}%`
      : "0%";

  return (
    <section className="content-grid">
      <Tabs
        className="panel span-12 focused-tabs"
        value={activeTab}
        onValueChange={(value) => setActiveTab(value as ReportTab)}
      >
        <div className="section-heading">
          <div>
            <h2>報表工作區</h2>
            <p>參與數據與匯出狀態分開檢視，避免表格和投遞狀態混在一起。</p>
          </div>
          <div className="toolbar">
            <Field
              className="compact-field"
              id="report-search"
              label="篩選報表表格"
              name="report-search"
              value={filter}
              onChange={setFilter}
              autoComplete="off"
              placeholder="搜尋活動或活動編號"
            />
            <SelectField
              className="compact-field"
              label="報表模板"
              value={reportPreset}
              options={reportPresetOptions}
              onChange={setReportPreset}
            />
            <TabsList>
              <TabsTrigger value="participation">參與報表</TabsTrigger>
              <TabsTrigger value="exports">匯出狀態</TabsTrigger>
            </TabsList>
          </div>
        </div>
        {message && <Alert tone="warn">{message}</Alert>}
        <TabsContent value="participation">
          <div className="toolbar report-toolbar">
            <Button
              variant="outline"
              type="button"
              onClick={() => void refresh()}
            >
              <Icon name="refresh" />
              重新整理
            </Button>
            <Button
              type="button"
              onClick={() => void exportParticipationReport()}
              disabled={exporting}
            >
              <Icon name="save" />
              {exporting ? "匯出中" : "匯出完整參與報表"}
            </Button>
          </div>
          <CompactStatsBar
            items={[
              { label: "已報名", value: totals.confirmed },
              { label: "候補", value: totals.waitlist },
              { label: "票券", value: totals.tickets },
              { label: "已入場", value: totals.checkins },
              { label: "到場率", value: attendanceRate },
            ]}
            label="人資報表摘要"
          />
          <ResponsiveTable
            label="人資參與報表"
            mobileCards={filteredReports.map((row) => (
              <article className="mobile-summary-card" key={row.event_id}>
                <div>
                  <h3>{row.title}</h3>
                  <p className="table-muted">{row.event_id}</p>
                </div>
                <CompactStatsBar
                  items={[
                    { label: "容量", value: row.capacity },
                    { label: "已報名", value: row.confirmed_count },
                    { label: "候補", value: row.waitlist_count },
                    { label: "票券", value: row.ticket_count },
                    { label: "已入場", value: row.checkin_count },
                  ]}
                  label={`${row.title}報表摘要`}
                />
                <ProgressMeter
                  label="到場率"
                  value={row.checkin_count}
                  max={row.confirmed_count}
                  helper={
                    row.confirmed_count > 0
                      ? `${Math.round((row.checkin_count / row.confirmed_count) * 100)}%`
                      : "0%"
                  }
                  compact
                />
                <dl className="meta-list vertical">
                  <div>
                    <dt>剩餘</dt>
                    <dd>{row.remaining_capacity}</dd>
                  </div>
                </dl>
              </article>
            ))}
          >
            <thead>
              <tr>
                <th>活動</th>
                <th>容量</th>
                <th>已報名</th>
                <th>候補</th>
                <th>票券</th>
                <th>已入場</th>
                <th>到場率</th>
                <th>剩餘</th>
              </tr>
            </thead>
            <tbody>
              {filteredReports.map((row) => (
                <tr key={row.event_id}>
                  <td>{row.title}</td>
                  <td>{row.capacity}</td>
                  <td>{row.confirmed_count}</td>
                  <td>{row.waitlist_count}</td>
                  <td>{row.ticket_count}</td>
                  <td>{row.checkin_count}</td>
                  <td>
                    <ProgressMeter
                      label="到場率"
                      value={row.checkin_count}
                      max={row.confirmed_count}
                      helper={
                        row.confirmed_count > 0
                          ? `${Math.round((row.checkin_count / row.confirmed_count) * 100)}%`
                          : "0%"
                      }
                      compact
                    />
                  </td>
                  <td>{row.remaining_capacity}</td>
                </tr>
              ))}
            </tbody>
          </ResponsiveTable>
          {!message && filteredReports.length === 0 && (
            <EmptyState
              title="沒有符合條件的報表"
              action="調整搜尋字串或先建立活動報名資料。"
            />
          )}
        </TabsContent>
        <TabsContent value="exports">
          {lastExport ? (
            <div className="export-detail" role="status" aria-live="polite">
              <div className="helper-strip">
                <StatusBadge
                  tone={reportExportStatusView(lastExport.status).tone}
                >
                  {reportExportStatusView(lastExport.status).label}
                </StatusBadge>
                <span>
                  {lastExport.status === "failed"
                    ? "匯出失敗，請重新產生。"
                    : "最新匯出狀態已更新。"}
                </span>
              </div>
              <dl className="meta-list vertical">
                <div>
                  <dt>匯出編號</dt>
                  <dd className="mono-cell">{lastExport.export_id}</dd>
                </div>
                <div>
                  <dt>請求者</dt>
                  <dd>{lastExport.requested_by}</dd>
                </div>
                <div>
                  <dt>建立時間</dt>
                  <dd>{lastExport.created_at}</dd>
                </div>
                <div>
                  <dt>完成時間</dt>
                  <dd>{lastExport.completed_at || "尚未完成"}</dd>
                </div>
                <div>
                  <dt>物件 key</dt>
                  <dd className="mono-cell">
                    {lastExport.object_key || "尚未產生"}
                  </dd>
                </div>
              </dl>
            </div>
          ) : (
            <EmptyState
              title="尚無匯出紀錄"
              action="從參與報表分頁匯出逗號分隔檔後，這裡會顯示最新匯出狀態。"
            />
          )}
        </TabsContent>
      </Tabs>
    </section>
  );
}

function reportPresetMatches(row: ReportRow, preset: string) {
  if (preset === "waitlist") return row.waitlist_count > 0;
  if (preset === "capacity") return row.remaining_capacity > 0;
  if (preset === "exceptions") {
    return (
      row.confirmed_count > 0 && row.checkin_count / row.confirmed_count < 0.5
    );
  }
  return true;
}
