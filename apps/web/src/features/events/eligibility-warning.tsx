import { Alert } from "@/components/shared";
import type { EligibilityWarning } from "@/lib/api/contracts";

interface EligibilityWarningListProps {
  warnings: EligibilityWarning[];
}

export function EligibilityWarningList({ warnings }: EligibilityWarningListProps) {
  if (warnings.length === 0) return null;

  return (
    <div aria-label="Eligibility warnings" className="space-y-2">
      {warnings.map((warning, index) => (
        <Alert key={`${warning.code}-${index}`} tone="warn">
          <strong>{warning.code === "cross_city" ? "Cross-city event notice" : "Eligibility notice"}</strong>
          <p>{warning.message}</p>
        </Alert>
      ))}
    </div>
  );
}
