import { StatusBadge } from "@/components/shared";
import type { EmployeeRegistrationDeadlineView } from "./employee-calendar-planner";

export function EmployeeRegistrationDeadlineChip({
  deadline,
}: Readonly<{ deadline: EmployeeRegistrationDeadlineView }>) {
  return (
    <span
      className="employee-deadline-chip"
      title={deadline.detail}
      data-deadline-kind={deadline.kind}
    >
      <StatusBadge tone={deadline.tone}>{deadline.label}</StatusBadge>
    </span>
  );
}
