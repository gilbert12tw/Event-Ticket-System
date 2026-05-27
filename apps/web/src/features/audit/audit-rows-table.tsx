import type { KeyboardEvent } from "react";
import type { AuditLog } from "@/lib/api";
import { formatDate } from "@/lib/formatting";
import { Button } from "@/components/ui/button";
import {
  EmptyState,
  MetaList,
  ResponsiveTable,
  StatusBadge,
} from "@/components/shared";
import {
  auditActionView,
  entityTypeLabel,
  roleViewLabel,
} from "@/lib/ui/options";

export function AuditRowsTable({
  cursor,
  hasPreviousCursor,
  nextCursor,
  onNextCursor,
  onPreviousCursor,
  onSelect,
  rows,
  selectedID,
}: {
  cursor: string;
  hasPreviousCursor: boolean;
  nextCursor: string;
  onNextCursor: () => void;
  onPreviousCursor: () => void;
  onSelect: (auditID: string) => void;
  rows: AuditLog[];
  selectedID: string;
}) {
  if (rows.length === 0) {
    return (
      <>
        <EmptyState
          title="沒有稽核紀錄"
          action={
            cursor
              ? "此游標後沒有更多紀錄，返回上一頁或調整篩選條件。"
              : "調整分頁或進階篩選條件。"
          }
        />
        {(cursor || nextCursor) && (
          <div className="toolbar report-toolbar">
            <Button
              variant="outline"
              type="button"
              onClick={onPreviousCursor}
              disabled={!hasPreviousCursor}
            >
              上一頁
            </Button>
            <Button
              variant="outline"
              type="button"
              onClick={onNextCursor}
              disabled={!nextCursor}
            >
              下一頁
            </Button>
            <span className="table-muted">
              {cursor ? `目前游標 ${cursor}` : "第一頁"}
            </span>
          </div>
        )}
      </>
    );
  }

  return (
    <>
      <ResponsiveTable
        label="稽核紀錄"
        mobileCards={rows.map((row) => (
          <AuditMobileCard
            key={row.audit_id}
            row={row}
            selected={selectedID === row.audit_id}
            onSelect={onSelect}
          />
        ))}
      >
        <thead>
          <tr>
            <th>時間</th>
            <th>操作</th>
            <th>執行者</th>
            <th>物件</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <AuditTableRow
              key={row.audit_id}
              row={row}
              selected={selectedID === row.audit_id}
              onSelect={onSelect}
            />
          ))}
        </tbody>
      </ResponsiveTable>
      <div className="toolbar report-toolbar">
        <Button
          variant="outline"
          type="button"
          onClick={onPreviousCursor}
          disabled={!hasPreviousCursor}
        >
          上一頁
        </Button>
        <Button
          variant="outline"
          type="button"
          onClick={onNextCursor}
          disabled={!nextCursor}
        >
          下一頁
        </Button>
        <span className="table-muted">
          {cursor ? `目前游標 ${cursor}` : "第一頁"}
        </span>
      </div>
    </>
  );
}

function AuditTableRow({
  onSelect,
  row,
  selected,
}: {
  onSelect: (auditID: string) => void;
  row: AuditLog;
  selected: boolean;
}) {
  return (
    <tr
      className={selected ? "interactive-row selected-row" : "interactive-row"}
      tabIndex={0}
      aria-selected={selected}
      aria-label={`檢視稽核 ${row.audit_id}`}
      onClick={() => onSelect(row.audit_id)}
      onKeyDown={(event) =>
        handleAuditRowKeyDown(event, row.audit_id, onSelect)
      }
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
    </tr>
  );
}

function AuditMobileCard({
  onSelect,
  row,
  selected,
}: {
  onSelect: (auditID: string) => void;
  row: AuditLog;
  selected: boolean;
}) {
  return (
    <article
      className={
        selected
          ? "mobile-summary-card audit-mobile-card selected-row"
          : "mobile-summary-card audit-mobile-card"
      }
    >
      <div>
        <h3>{formatDate(row.created_at)}</h3>
        <StatusBadge tone={auditActionView(row.action).tone}>
          {auditActionView(row.action).label}
        </StatusBadge>
      </div>
      <MetaList
        rows={[
          [
            "執行者",
            <>
              {row.actor_id}
              <span className="table-muted">{roleViewLabel(row.role)}</span>
            </>,
          ],
          [
            "物件",
            <>
              {entityTypeLabel(row.entity_type)}
              <span className="table-muted mono-cell">{row.entity_id}</span>
            </>,
          ],
        ]}
      />
      <Button
        aria-pressed={selected}
        variant="outline"
        type="button"
        onClick={() => onSelect(row.audit_id)}
      >
        檢視稽核明細
      </Button>
    </article>
  );
}

function handleAuditRowKeyDown(
  event: KeyboardEvent<HTMLTableRowElement>,
  auditID: string,
  onSelect: (auditID: string) => void,
) {
  if (event.key !== "Enter" && event.key !== " ") return;
  event.preventDefault();
  onSelect(auditID);
}
