import type { IconName } from "@/app/routes";
import type { AuthMeClaims, EmployeeProfile } from "@/lib/api";
import { roleLabel } from "@/lib/formatting";
import {
  departmentLabel,
  employmentStatusLabel,
  siteLabel,
} from "@/lib/ui/options";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Icon } from "./icon";

export function BoundaryContext({
  title,
  description,
  icon,
}: {
  title: string;
  description: string;
  icon: IconName;
}) {
  return (
    <section className="boundary-strip span-12">
      <div>
        <div className="eyebrow">作業範圍</div>
        <h2>{title}</h2>
        <p>{description}</p>
      </div>
      <div className="boundary-icon" aria-hidden="true">
        <Icon name={icon} />
      </div>
    </section>
  );
}

export function IdentityCard({ claims }: { claims: AuthMeClaims }) {
  const role = claims.mapped_roles[0] || "employee";
  return (
    <div className="identity-card" aria-label="目前登入身份">
      <span>{claims.display_name || claims.employee_id}</span>
      <strong>{claims.employee_id}</strong>
      <small>{roleLabel(role)}</small>
    </div>
  );
}

export function ProviderClaimsCard({ claims }: { claims: AuthMeClaims }) {
  return (
    <Card asChild className="panel span-4">
      <aside>
        <CardHeader>
          <CardTitle>身分宣告</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="meta-list vertical">
            <div>
              <dt>員工</dt>
              <dd>
                {claims.display_name || claims.employee_id}
                <span className="table-muted">{claims.employee_id}</span>
              </dd>
            </div>
            <div>
              <dt>部門</dt>
              <dd>{departmentLabel(claims.department)}</dd>
            </div>
            <div>
              <dt>廠區</dt>
              <dd>{siteLabel(claims.site)}</dd>
            </div>
            <div>
              <dt>職稱</dt>
              <dd>{claims.job_title || "未提供"}</dd>
            </div>
            <div>
              <dt>城市</dt>
              <dd>{siteLabel(claims.city)}</dd>
            </div>
          </dl>
        </CardContent>
      </aside>
    </Card>
  );
}

export function EmployeeProfileCard({
  employee,
}: {
  employee: EmployeeProfile;
}) {
  return (
    <Card asChild className="panel span-4">
      <aside>
        <CardHeader>
          <CardTitle>人資屬性</CardTitle>
        </CardHeader>
        <CardContent>
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
              <dd>{departmentLabel(employee.department)}</dd>
            </div>
            <div>
              <dt>廠區</dt>
              <dd>{siteLabel(employee.site)}</dd>
            </div>
            <div>
              <dt>職等</dt>
              <dd>G{employee.job_grade}</dd>
            </div>
            <div>
              <dt>狀態</dt>
              <dd>{employmentStatusLabel(employee.employment_status)}</dd>
            </div>
          </dl>
        </CardContent>
      </aside>
    </Card>
  );
}
