import { useEffect, useMemo, useState } from "react";
import { createReportExport, getReportExport, reports } from "@/lib/api";
import type { ReportExport, ReportRow } from "@/lib/api";
import { errorMessage } from "@/lib/formatting";
import { Alert, EmptyState, Kpi, ProgressMeter, ResponsiveTable, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";

export function HrReportsPage() {
  const [reportRows, setReportRows] = useState<ReportRow[]>([]);
  const [lastExport, setLastExport] = useState<ReportExport | null>(null);
  const [filter, setFilter] = useState("");
  const [message, setMessage] = useState("");
  const [exporting, setExporting] = useState(false);

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
      const requested = await createReportExport({ report_type: "participation" });
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
          checkins: acc.checkins + row.checkin_count
        }),
        { confirmed: 0, waitlist: 0, tickets: 0, checkins: 0 }
      ),
    [reportRows]
  );

  const filteredReports = reportRows.filter((row) => {
    const needle = filter.toLowerCase();
    return `${row.title} ${row.event_id}`.toLowerCase().includes(needle);
  });
  const attendanceRate = totals.confirmed > 0 ? `${Math.round((totals.checkins / totals.confirmed) * 100)}%` : "0%";

  return (
    <section className="content-grid">
      <div className="panel span-12 workspace-context admin-context">
        <div>
          <div className="eyebrow">Admin Console</div>
          <h2>HR 報表入口</h2>
          <p>報表只呈現彙總與活動層級數字，避免讓 HR 看到不必要的員工個資。</p>
        </div>
        <label className="search-field">
          <span className="sr-only">篩選 report table</span>
          <input value={filter} onChange={(event) => setFilter(event.target.value)} placeholder="搜尋活動或 event id" />
        </label>
      </div>
      <div className="panel span-12">
        <div className="section-heading">
          <div>
            <h2>即時參與報表</h2>
            <p>彙總 confirmed、候補、票券、check-in 與到場率。</p>
          </div>
          <button className="button secondary" type="button" onClick={() => void refresh()}>
            <Icon name="refresh" />
            重新整理
          </button>
          <button className="button" type="button" onClick={() => void exportParticipationReport()} disabled={exporting}>
            <Icon name="save" />
            {exporting ? "匯出中" : "匯出 CSV"}
          </button>
        </div>
        {message && <Alert tone="warn">{message}</Alert>}
        {lastExport && (
          <div className="helper-strip" role="status" aria-live="polite">
            <StatusBadge tone={lastExport.status === "ready" ? "ok" : lastExport.status === "failed" ? "fail" : "warn"}>{lastExport.status}</StatusBadge>
            <span>
              Export {lastExport.export_id}
              {lastExport.object_key ? ` · ${lastExport.object_key}` : ""}
            </span>
          </div>
        )}
        <div className="kpi-row four">
          <Kpi label="Confirmed" value={totals.confirmed} />
          <Kpi label="Waitlist" value={totals.waitlist} />
          <Kpi label="Tickets" value={totals.tickets} />
          <Kpi label="Check-ins" value={totals.checkins} />
          <Kpi label="到場率" value={attendanceRate} />
        </div>
        <ResponsiveTable>
          <thead>
            <tr>
              <th>活動</th>
              <th>容量</th>
              <th>Confirmed</th>
              <th>Waitlist</th>
              <th>Tickets</th>
              <th>Check-ins</th>
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
                    helper={row.confirmed_count > 0 ? `${Math.round((row.checkin_count / row.confirmed_count) * 100)}%` : "0%"}
                    compact
                  />
                </td>
                <td>{row.remaining_capacity}</td>
              </tr>
            ))}
          </tbody>
        </ResponsiveTable>
        {!message && filteredReports.length === 0 && <EmptyState title="沒有符合條件的報表" action="調整搜尋字串或先建立活動報名資料。" />}
      </div>
    </section>
  );
}
