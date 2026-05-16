import { EmptyState, StatusBadge } from "@/components/shared";
import type { EmployeeProfile, EventSummary } from "@/lib/api";
import { SelectEventFirst } from "./admin-event-form-controls";

export function AdminEventEligibilityTab({
  selectedEvent,
  matchingEmployees,
}: {
  selectedEvent?: EventSummary;
  matchingEmployees: EmployeeProfile[];
}) {
  if (!selectedEvent) return <SelectEventFirst action="檢視資格" />;
  return (
    <div className="task-panel">
      <div className="section-heading">
        <div>
          <h2>資格預覽</h2>
          <p>只檢視目前資格命中情形，不在同一頁混入活動編輯或封存操作。</p>
        </div>
        <StatusBadge tone={matchingEmployees.length > 0 ? "ok" : "warn"}>
          {matchingEmployees.length} 人符合
        </StatusBadge>
      </div>
      <dl className="meta-list">
        <div>
          <dt>部門</dt>
          <dd>{selectedEvent.rule.department}</dd>
        </div>
        <div>
          <dt>廠區</dt>
          <dd>{selectedEvent.rule.site}</dd>
        </div>
        <div>
          <dt>最低職等</dt>
          <dd>G{selectedEvent.rule.min_grade}+</dd>
        </div>
        <div>
          <dt>雇用狀態</dt>
          <dd>{selectedEvent.rule.employment_status}</dd>
        </div>
      </dl>
      <div className="readiness-grid">
        {matchingEmployees.map((employee) => (
          <div className="readiness-item ok" key={employee.employee_id}>
            <StatusBadge tone="ok">{employee.employee_id}</StatusBadge>
            <span>
              {employee.department} / {employee.site} / G{employee.job_grade}
            </span>
          </div>
        ))}
      </div>
      {matchingEmployees.length === 0 && (
        <EmptyState
          title="沒有符合人員"
          action="請回到建立活動或資格設定調整規則。"
        />
      )}
    </div>
  );
}
