import { useEffect, useMemo, useState } from "react";
import { auditLogs, type AuditLog, type AuditLogFilters } from "@/lib/api";
import { errorMessage, normalizeAuditFilters } from "@/lib/formatting";
import {
  Alert,
  CompactStatsBar,
  EmptyState,
  Field,
  MetaList,
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
import { AuditRowsTable } from "./audit-rows-table";
import {
  auditCursorFromRow,
  readAuditUrlState,
  replaceAuditUrl,
} from "./url-state";

type AuditPreset = "all" | "conflicts" | "event" | "ticket" | "checkin";
const auditPresets = [
  "all",
  "conflicts",
  "event",
  "ticket",
  "checkin",
] as const;

export function AdminAuditPage() {
  const [initialUrlState] = useState(readAuditUrlState);
  const [auditRows, setAuditRows] = useState<AuditLog[]>([]);
  const [filters, setFilters] = useState<AuditLogFilters>(
    initialUrlState.filters,
  );
  const [selectedID, setSelectedID] = useState(initialUrlState.selectedID);
  const [cursorStack, setCursorStack] = useState<string[]>([]);
  const [message, setMessage] = useState("");
  const [activePreset, setActivePreset] = useUrlTab<AuditPreset>(
    "tab",
    auditPresets,
    "all",
  );

  async function refresh(
    nextFilters = filters,
    nextPreset = activePreset,
    nextSelectedID = selectedID,
  ) {
    setMessage("");
    try {
      const normalized = normalizeAuditFilters(nextFilters);
      const rows = await auditLogs(normalized);
      setAuditRows(rows);
      const matchingRowID = rows.find(
        (row) => row.audit_id === nextSelectedID,
      )?.audit_id;
      const firstRowID = rows[0]?.audit_id;
      const selected = matchingRowID ?? firstRowID ?? "";
      setSelectedID(selected);
      replaceAuditUrl(nextFilters, nextPreset, selected);
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
    presetRows.find((row) => row.audit_id === selectedID) ?? presetRows[0];
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
    ([key, value]) => key !== "limit" && key !== "cursor" && Boolean(value),
  ).length;
  const latestAudit = auditRows.at(-1);
  const nextCursor = latestAudit ? auditCursorFromRow(latestAudit) : "";

  function applyFilters(nextFilters: AuditLogFilters) {
    setFilters(nextFilters);
    void refresh(nextFilters, activePreset, "");
  }

  function selectAuditRow(auditID: string) {
    setSelectedID(auditID);
    replaceAuditUrl(filters, activePreset, auditID);
  }

  function changePreset(nextPreset: AuditPreset) {
    setActivePreset(nextPreset);
    replaceAuditUrl(filters, nextPreset, selectedID);
  }

  function goToNextCursorPage() {
    if (!nextCursor) return;
    const nextFilters = { ...filters, cursor: nextCursor };
    setCursorStack((stack) => [...stack, filters.cursor ?? ""]);
    applyFilters(nextFilters);
  }

  function goToPreviousCursorPage() {
    const previousCursor = cursorStack.at(-1);
    const nextFilters = { ...filters, cursor: previousCursor ?? undefined };
    setCursorStack((stack) => stack.slice(0, -1));
    applyFilters(nextFilters);
  }

  return (
    <section className="content-grid">
      <Card asChild className="panel span-12 audit-filter-panel">
        <form
          onSubmit={(event) => {
            event.preventDefault();
            setCursorStack([]);
            applyFilters({ ...filters, cursor: undefined });
          }}
        >
          <div className="toolbar">
            <SelectField
              label="角色"
              value={filters.role ?? ""}
              options={auditRoleOptions}
              onChange={(value) => setFilters({ ...filters, role: value })}
            />
            <SelectField
              label="操作"
              value={filters.action ?? ""}
              options={auditActionOptions}
              onChange={(value) => setFilters({ ...filters, action: value })}
            />
            <SelectField
              label="物件"
              value={filters.entity_type ?? ""}
              options={auditEntityTypeOptions}
              onChange={(value) =>
                setFilters({ ...filters, entity_type: value })
              }
            />
            <SelectField
              label="筆數"
              value={filters.limit ?? "50"}
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
              onClick={() => {
                setCursorStack([]);
                applyFilters({ limit: "50" });
              }}
            >
              重設
            </Button>
          </div>
          <details className="advanced-filter">
            <summary>
              進階篩選 <span>{activeFilterCount} 個條件</span>
            </summary>
            <div className="form-grid mt-14">
              <Field
                label="執行者編號"
                value={filters.actor_id ?? ""}
                onChange={(value) =>
                  setFilters({ ...filters, actor_id: value })
                }
              />
              <Field
                label="物件編號"
                value={filters.entity_id ?? ""}
                onChange={(value) =>
                  setFilters({ ...filters, entity_id: value })
                }
              />
              <Field
                label="起始時間"
                type="datetime-local"
                value={filters.from ?? ""}
                onChange={(value) => setFilters({ ...filters, from: value })}
              />
              <Field
                label="結束時間"
                type="datetime-local"
                value={filters.to ?? ""}
                onChange={(value) => setFilters({ ...filters, to: value })}
              />
            </div>
          </details>
        </form>
      </Card>
      <Tabs
        className="panel span-8 focused-tabs audit-list-panel"
        value={activePreset}
        onValueChange={(value) => changePreset(value as AuditPreset)}
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
            cursor={filters.cursor ?? ""}
            hasPreviousCursor={cursorStack.length > 0}
            nextCursor={nextCursor}
            onNextCursor={goToNextCursorPage}
            onPreviousCursor={goToPreviousCursorPage}
            rows={presetRows}
            selectedID={selected?.audit_id ?? ""}
            onSelect={selectAuditRow}
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
            <MetaList
              className="audit-detail"
              rows={[
                ["稽核編號", selected.audit_id, "mono-cell id-cell"],
                [
                  "執行者",
                  <span key="actor" className="meta-value-stack">
                    {selected.actor_id}
                    <span className="table-muted">
                      {roleViewLabel(selected.role)}
                    </span>
                  </span>,
                ],
                [
                  "物件",
                  <span key="entity" className="meta-value-stack">
                    {entityTypeLabel(selected.entity_type)}
                    <span className="table-muted mono-cell">
                      {selected.entity_id}
                    </span>
                  </span>,
                ],
                [
                  "稽核中繼資料，敏感值已由系統遮蔽",
                  <AuditMetadata key="metadata" metadata={selected.metadata} />,
                  undefined,
                  "full",
                ],
              ]}
            />
          )}
        </aside>
      </Card>
    </section>
  );
}

function auditPresetMatches(row: AuditLog, preset: AuditPreset) {
  if (preset === "all") return true;
  if (preset === "conflicts") return row.action.includes("conflict");
  return row.entity_type === preset;
}

function AuditMetadata({ metadata }: Readonly<{ metadata: string }>) {
  const formatMetadataValue = (value: unknown) => {
    if (value === null || value === undefined) return "";
    if (typeof value === "string") return value;
    return JSON.stringify(value) ?? "";
  };

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
            <dd className="mono-cell">{formatMetadataValue(value)}</dd>
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
