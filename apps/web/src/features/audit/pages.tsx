import { useEffect, useMemo, useState } from "react";
import { auditLogs } from "@/lib/api";
import type { AuditLog, AuditLogFilters } from "@/lib/api";
import {
  errorMessage,
  formatDate,
  normalizeAuditFilters,
} from "@/lib/formatting";
import {
  Alert,
  CompactStatsBar,
  EmptyState,
  Field,
  ResponsiveTable,
  SelectField,
  StatusBadge,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useUrlTab } from "@/hooks/use-url-tab";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  auditActionView,
  auditActionOptions,
  auditEntityTypeOptions,
  auditLimitOptions,
  auditRoleOptions,
  entityTypeLabel,
  roleViewLabel,
} from "@/lib/ui/options";

type AuditPreset = "all" | "conflicts" | "event" | "ticket" | "checkin";
const auditPresets = [
  "all",
  "conflicts",
  "event",
  "ticket",
  "checkin",
] as const;

export function AdminAuditPage() {
  const [auditRows, setAuditRows] = useState<AuditLog[]>([]);
  const [filters, setFilters] = useState<AuditLogFilters>({ limit: "50" });
  const [selectedID, setSelectedID] = useState("");
  const [message, setMessage] = useState("");
  const [activePreset, setActivePreset] = useUrlTab<AuditPreset>(
    "tab",
    auditPresets,
    "all",
  );

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

  const presetRows = auditRows.filter((row) =>
    auditPresetMatches(row, activePreset),
  );
  const selected =
    presetRows.find((row) => row.audit_id === selectedID) || presetRows[0];
  const auditStats = useMemo(
    () => ({
      total: presetRows.length,
      conflicts: presetRows.filter((row) => row.action.includes("conflict"))
        .length,
      events: presetRows.filter((row) => row.entity_type === "event").length,
      tickets: presetRows.filter((row) => row.entity_type === "ticket").length,
    }),
    [presetRows],
  );
  const activeFilterCount = Object.entries(filters).filter(
    ([key, value]) => key !== "limit" && Boolean(value),
  ).length;

  return (
    <section className="content-grid">
      <Card asChild className="panel span-12 audit-filter-panel">
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void refresh();
          }}
        >
          <div className="toolbar">
            <SelectField
              label="角色"
              value={filters.role || ""}
              options={auditRoleOptions}
              onChange={(value) => setFilters({ ...filters, role: value })}
            />
            <SelectField
              label="操作"
              value={filters.action || ""}
              options={auditActionOptions}
              onChange={(value) => setFilters({ ...filters, action: value })}
            />
            <SelectField
              label="物件"
              value={filters.entity_type || ""}
              options={auditEntityTypeOptions}
              onChange={(value) =>
                setFilters({ ...filters, entity_type: value })
              }
            />
            <SelectField
              label="筆數"
              value={filters.limit || "50"}
              options={auditLimitOptions}
              onChange={(value) => setFilters({ ...filters, limit: value })}
            />
            <Button type="submit">
              <Icon name="audit" />
              套用篩選
            </Button>
            <Button
              variant="outline"
              type="button"
              onClick={() => setFilters({ limit: "50" })}
            >
              重設
            </Button>
          </div>
          <details className="advanced-filter">
            <summary>
              進階篩選
              <span>{activeFilterCount} 個條件</span>
            </summary>
            <div className="form-grid mt-14">
              <Field
                label="執行者編號"
                value={filters.actor_id || ""}
                onChange={(value) =>
                  setFilters({ ...filters, actor_id: value })
                }
              />
              <Field
                label="物件編號"
                value={filters.entity_id || ""}
                onChange={(value) =>
                  setFilters({ ...filters, entity_id: value })
                }
              />
              <Field
                label="起始時間"
                type="datetime-local"
                value={filters.from || ""}
                onChange={(value) => setFilters({ ...filters, from: value })}
              />
              <Field
                label="結束時間"
                type="datetime-local"
                value={filters.to || ""}
                onChange={(value) => setFilters({ ...filters, to: value })}
              />
            </div>
          </details>
        </form>
      </Card>
      <Tabs
        className="panel span-8 focused-tabs"
        value={activePreset}
        onValueChange={(value) => setActivePreset(value as AuditPreset)}
      >
        <div className="section-heading">
          <div>
            <h2>稽核紀錄</h2>
            <p>先用事件類型分頁聚焦，再選取單列查看中繼資料。</p>
          </div>
          <TabsList>
            <TabsTrigger value="all">全部</TabsTrigger>
            <TabsTrigger value="conflicts">衝突</TabsTrigger>
            <TabsTrigger value="event">活動</TabsTrigger>
            <TabsTrigger value="ticket">票券</TabsTrigger>
            <TabsTrigger value="checkin">驗票</TabsTrigger>
          </TabsList>
        </div>
        {message && <Alert tone="warn">{message}</Alert>}
        <CompactStatsBar
          items={[
            { label: "稽核筆數", value: auditStats.total },
            { label: "衝突", value: auditStats.conflicts },
            { label: "活動", value: auditStats.events },
            { label: "票券", value: auditStats.tickets },
          ]}
          label="稽核摘要"
        />
        <TabsContent value={activePreset}>
          <AuditRowsTable
            rows={presetRows}
            selectedID={selected?.audit_id || ""}
            onSelect={setSelectedID}
          />
        </TabsContent>
      </Tabs>
      <Card asChild className="panel span-4 audit-drawer">
        <aside
          aria-live="polite"
          aria-label={selected ? `稽核明細 ${selected.audit_id}` : "稽核明細"}
        >
          <div className="section-heading">
            <div>
              <h2>中繼資料明細</h2>
              <p>選取一列後，在此檢查稽核中繼資料。</p>
            </div>
            {selected && (
              <StatusBadge tone={auditActionView(selected.action).tone}>
                {auditActionView(selected.action).label}
              </StatusBadge>
            )}
          </div>
          {!selected && (
            <EmptyState
              title="沒有稽核紀錄"
              action="完成活動、報名、驗票或匯出操作後會出現稽核紀錄。"
            />
          )}
          {selected && (
            <dl className="meta-list audit-detail">
              <div>
                <dt>稽核編號</dt>
                <dd>{selected.audit_id}</dd>
              </div>
              <div>
                <dt>執行者</dt>
                <dd>
                  {selected.actor_id}
                  <span className="table-muted">
                    {roleViewLabel(selected.role)}
                  </span>
                </dd>
              </div>
              <div>
                <dt>物件</dt>
                <dd>
                  {entityTypeLabel(selected.entity_type)}
                  <span className="table-muted">{selected.entity_id}</span>
                </dd>
              </div>
              <div className="full">
                <dt>稽核中繼資料，敏感值已由系統遮蔽</dt>
                <dd>
                  <AuditMetadata metadata={selected.metadata} />
                </dd>
              </div>
            </dl>
          )}
        </aside>
      </Card>
    </section>
  );
}

function AuditRowsTable({
  onSelect,
  rows,
  selectedID,
}: {
  onSelect: (auditID: string) => void;
  rows: AuditLog[];
  selectedID: string;
}) {
  if (rows.length === 0) {
    return (
      <EmptyState title="沒有稽核紀錄" action="調整分頁或進階篩選條件。" />
    );
  }
  return (
    <ResponsiveTable>
      <thead>
        <tr>
          <th>時間</th>
          <th>操作</th>
          <th>執行者</th>
          <th>物件</th>
          <th>明細</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr
            className={selectedID === row.audit_id ? "selected-row" : ""}
            aria-selected={selectedID === row.audit_id}
            key={row.audit_id}
          >
            <td>{formatDate(row.created_at)}</td>
            <td>
              <StatusBadge tone={auditActionView(row.action).tone}>
                {auditActionView(row.action).label}
              </StatusBadge>
            </td>
            <td>
              {row.actor_id}
              <span className="table-muted">{roleViewLabel(row.role)}</span>
            </td>
            <td>
              {entityTypeLabel(row.entity_type)}
              <span className="table-muted">{row.entity_id}</span>
            </td>
            <td>
              <Button
                variant="outline"
                size="sm"
                type="button"
                onClick={() => onSelect(row.audit_id)}
              >
                檢視
              </Button>
            </td>
          </tr>
        ))}
      </tbody>
    </ResponsiveTable>
  );
}

function auditPresetMatches(row: AuditLog, preset: AuditPreset) {
  if (preset === "all") return true;
  if (preset === "conflicts") return row.action.includes("conflict");
  return row.entity_type === preset;
}

function AuditMetadata({ metadata }: { metadata: string }) {
  try {
    const parsed = JSON.parse(metadata) as unknown;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return <span className="mono-cell">{metadata}</span>;
    }
    return (
      <dl className="metadata-list">
        {Object.entries(parsed).map(([key, value]) => (
          <div key={key}>
            <dt>{key}</dt>
            <dd className="mono-cell">{String(value)}</dd>
          </div>
        ))}
      </dl>
    );
  } catch {
    return (
      <div className="summary-block">
        <span className="form-hint">無法解析中繼資料，顯示原始遮蔽內容。</span>
        <span className="mono-cell">{metadata}</span>
      </div>
    );
  }
}
