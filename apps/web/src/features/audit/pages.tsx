import { useEffect, useMemo, useState } from "react";
import { auditLogs } from "@/lib/api";
import type { AuditLog, AuditLogFilters } from "@/lib/api";
import { errorMessage, formatDate, normalizeAuditFilters } from "@/lib/formatting";
import { Alert, EmptyState, Field, Kpi, ResponsiveTable, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";

export function AdminAuditPage() {
  const [auditRows, setAuditRows] = useState<AuditLog[]>([]);
  const [filters, setFilters] = useState<AuditLogFilters>({ limit: "50" });
  const [selectedID, setSelectedID] = useState("");
  const [message, setMessage] = useState("");

  async function refresh() {
    setMessage("");
    try {
      const rows = await auditLogs(normalizeAuditFilters(filters));
      setAuditRows(rows);
      setSelectedID((current) => current || rows[0]?.audit_id || "");
    } catch (error) {
      setMessage(errorMessage(error));
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  const selected = auditRows.find((row) => row.audit_id === selectedID) || auditRows[0];
  const auditStats = useMemo(
    () => ({
      total: auditRows.length,
      conflicts: auditRows.filter((row) => row.action.includes("conflict")).length,
      events: auditRows.filter((row) => row.entity_type === "event").length,
      tickets: auditRows.filter((row) => row.entity_type === "ticket").length
    }),
    [auditRows]
  );

  return (
    <section className="content-grid">
      <div className="panel span-12 workspace-context admin-context">
        <div>
          <div className="eyebrow">Admin Console</div>
          <h2>稽核入口</h2>
          <p>透過後端 query params 篩選 actor、role、action、entity、時間區間與 limit，避免只靠前端文字過濾。</p>
        </div>
        <button className="button secondary" type="button" onClick={() => void refresh()}>
          <Icon name="refresh" />
          查詢
        </button>
      </div>
      <form
        className="panel span-12 form-grid"
        onSubmit={(event) => {
          event.preventDefault();
          void refresh();
        }}
      >
        <Field label="Actor ID" value={filters.actor_id || ""} onChange={(value) => setFilters({ ...filters, actor_id: value })} />
        <label className="field">
          <span>Role</span>
          <select value={filters.role || ""} onChange={(event) => setFilters({ ...filters, role: event.target.value })}>
            <option value="">any</option>
            <option value="employee">employee</option>
            <option value="activity_admin">activity_admin</option>
            <option value="checkin_staff">checkin_staff</option>
            <option value="hr_admin">hr_admin</option>
            <option value="system_admin">system_admin</option>
          </select>
        </label>
        <Field label="Action" value={filters.action || ""} onChange={(value) => setFilters({ ...filters, action: value })} />
        <Field label="Entity type" value={filters.entity_type || ""} onChange={(value) => setFilters({ ...filters, entity_type: value })} />
        <Field label="Entity ID" value={filters.entity_id || ""} onChange={(value) => setFilters({ ...filters, entity_id: value })} />
        <Field label="From" type="datetime-local" value={filters.from || ""} onChange={(value) => setFilters({ ...filters, from: value })} />
        <Field label="To" type="datetime-local" value={filters.to || ""} onChange={(value) => setFilters({ ...filters, to: value })} />
        <Field label="Limit" type="number" value={filters.limit || "50"} onChange={(value) => setFilters({ ...filters, limit: value })} />
        <div className="form-actions full">
          <button className="button" type="submit">
            <Icon name="audit" />
            套用 server filters
          </button>
          <button className="button secondary" type="button" onClick={() => setFilters({ limit: "50" })}>
            重設
          </button>
        </div>
      </form>
      <div className="panel span-12">
        <div className="section-heading">
          <div>
            <h2>Audit log</h2>
            <p>敏感操作依時間倒序顯示，metadata 保留在下方 detail drawer。</p>
          </div>
          <button className="button secondary" type="button" onClick={() => void refresh()}>
            <Icon name="refresh" />
            重新整理
          </button>
        </div>
        {message && <Alert tone="warn">{message}</Alert>}
        <div className="kpi-row four">
          <Kpi label="Audit records" value={auditStats.total} />
          <Kpi label="Conflicts" value={auditStats.conflicts} />
          <Kpi label="Events" value={auditStats.events} />
          <Kpi label="Tickets" value={auditStats.tickets} />
        </div>
        <ResponsiveTable>
          <thead>
            <tr>
              <th>時間</th>
              <th>Action</th>
              <th>Actor</th>
              <th>Entity</th>
              <th>Detail</th>
            </tr>
          </thead>
          <tbody>
            {auditRows.map((row) => (
              <tr className={selected?.audit_id === row.audit_id ? "selected-row" : ""} key={row.audit_id}>
                <td>{formatDate(row.created_at)}</td>
                <td>
                  <StatusBadge tone={row.action.includes("conflict") ? "warn" : "info"}>{row.action}</StatusBadge>
                </td>
                <td>
                  {row.actor_id}
                  <span className="table-muted">{row.role}</span>
                </td>
                <td>
                  {row.entity_type}
                  <span className="table-muted">{row.entity_id}</span>
                </td>
                <td>
                  <button className="button secondary compact-button" type="button" onClick={() => setSelectedID(row.audit_id)}>
                    檢視
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </ResponsiveTable>
      </div>
      <aside className="panel span-12 audit-drawer" aria-live="polite">
        <div className="section-heading">
          <div>
            <h2>Metadata detail drawer</h2>
            <p>保留原始 metadata 文字，方便 demo 驗證與稽核追蹤。</p>
          </div>
          {selected && <StatusBadge tone={selected.action.includes("conflict") ? "warn" : "info"}>{selected.action}</StatusBadge>}
        </div>
        {!selected && <EmptyState title="沒有 audit log" action="執行 Demo Runbook 或建立活動後會出現稽核紀錄。" />}
        {selected && (
          <dl className="meta-list audit-detail">
            <div>
              <dt>Audit ID</dt>
              <dd>{selected.audit_id}</dd>
            </div>
            <div>
              <dt>Actor</dt>
              <dd>
                {selected.actor_id}
                <span className="table-muted">{selected.role}</span>
              </dd>
            </div>
            <div>
              <dt>Entity</dt>
              <dd>
                {selected.entity_type}
                <span className="table-muted">{selected.entity_id}</span>
              </dd>
            </div>
            <div className="full">
              <dt>Metadata</dt>
              <dd className="mono-cell">{selected.metadata}</dd>
            </div>
          </dl>
        )}
      </aside>
    </section>
  );
}
