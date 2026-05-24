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
import { MetaList } from "./meta-list";

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
          <MetaList
            rows={[
              [
                "員工",
                <>
                  {claims.display_name || claims.employee_id}
                  <span className="table-muted">{claims.employee_id}</span>
                </>,
              ],
              ["部門", departmentLabel(claims.department)],
              ["廠區", siteLabel(claims.site)],
              ["職稱", claims.job_title || "未提供"],
              ["城市", siteLabel(claims.city)],
            ]}
          />
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
          <MetaList
            rows={[
              [
                "員工",
                <>
                  {employee.full_name}
                  <span className="table-muted">{employee.employee_id}</span>
                </>,
              ],
              ["部門", departmentLabel(employee.department)],
              ["廠區", siteLabel(employee.site)],
              ["職等", `G${employee.job_grade}`],
              ["狀態", employmentStatusLabel(employee.employment_status)],
            ]}
          />
        </CardContent>
      </aside>
    </Card>
  );
}
