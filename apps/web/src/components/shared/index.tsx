import type { ReactNode } from "react";
import type { EmployeeProfile } from "@/lib/api";
import type { IconName } from "@/app/routes";
import { mockProviderProfiles } from "@/app/routes";
import { roleLabel } from "@/lib/formatting";
import { Alert as UiAlert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Empty as UiEmpty } from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Icon } from "./icon";

export function BoundaryContext({ title, description, icon }: { title: string; description: string; icon: IconName }) {
  return (
    <div className="panel span-12 workspace-context admin-context">
      <div>
        <div className="eyebrow">Phase Boundary</div>
        <h2>{title}</h2>
        <p>{description}</p>
      </div>
      <div className="boundary-icon" aria-hidden="true">
        <Icon name={icon} />
      </div>
    </div>
  );
}

export function IdentityCard({ principalID }: { principalID: string }) {
  const principal = mockProviderProfiles.find((candidate) => candidate.id === principalID);
  return (
    <div className="identity-card" aria-label="目前登入身份">
      <span>{principal?.label || principalID}</span>
      <strong>{principalID}</strong>
      <small>{principal ? roleLabel(principal.role) : "employee"}</small>
    </div>
  );
}

export function EmployeeProfileCard({ employee }: { employee: EmployeeProfile }) {
  return (
    <aside className="panel span-4">
      <h2>HR 屬性</h2>
      <dl className="meta-list vertical">
        <div>
          <dt>員工</dt>
          <dd>
            {employee.full_name}
            <span className="table-muted">{employee.employee_id}</span>
          </dd>
        </div>
        <div>
          <dt>部門</dt>
          <dd>{employee.department}</dd>
        </div>
        <div>
          <dt>廠區</dt>
          <dd>{employee.site}</dd>
        </div>
        <div>
          <dt>職等</dt>
          <dd>G{employee.job_grade}</dd>
        </div>
        <div>
          <dt>狀態</dt>
          <dd>{employee.employment_status}</dd>
        </div>
      </dl>
    </aside>
  );
}

export function Field({
  label,
  value,
  onChange,
  type = "text",
  required = false
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  type?: string;
  required?: boolean;
}) {
  return (
    <label className="field">
      <span>
        {label}
        {required && <em aria-label="必填"> *</em>}
      </span>
      <Input type={type} value={value} onChange={(event) => onChange(event.target.value)} required={required} />
    </label>
  );
}

export function Alert({ children, tone }: { children: ReactNode; tone: "ok" | "warn" | "fail" | "info" }) {
  return (
    <UiAlert className={`alert ${tone}`} role="status" aria-live="polite">
      {children}
    </UiAlert>
  );
}

export function StatusBadge({ children, tone }: { children: ReactNode; tone: "ok" | "warn" | "fail" | "info" | "neutral" }) {
  return (
    <Badge className={`status-badge ${tone}`} variant="outline">
      {children}
    </Badge>
  );
}

export function Kpi({ label, value }: { label: string; value: number | string }) {
  return (
    <div className="kpi">
      <strong>{value}</strong>
      <span>{label}</span>
    </div>
  );
}

export function ProgressMeter({ label, value, max, helper, compact = false }: { label: string; value: number; max: number; helper: string; compact?: boolean }) {
  const ratio = max > 0 ? Math.min(Math.max(value / max, 0), 1) : 0;
  return (
    <div className={compact ? "progress-meter compact" : "progress-meter"} aria-label={`${label} ${helper}`}>
      <div className="progress-meter-head">
        <span>{label}</span>
        <strong>{helper}</strong>
      </div>
      <div className="progress-track">
        <span style={{ width: `${Math.round(ratio * 100)}%` }} />
      </div>
    </div>
  );
}

export function EmptyState({ title, action }: { title: string; action: string }) {
  return (
    <UiEmpty className="empty-state">
      <strong>{title}</strong>
      <span>{action}</span>
    </UiEmpty>
  );
}

export function ResponsiveTable({ children }: { children: ReactNode }) {
  return (
    <div className="table-scroll">
      <table>{children}</table>
    </div>
  );
}

export function SkeletonRows({ rows }: { rows: number }) {
  return (
    <div className="skeleton-stack">
      {Array.from({ length: rows }, (_, index) => (
        <Skeleton className="skeleton-row" key={index} />
      ))}
    </div>
  );
}
