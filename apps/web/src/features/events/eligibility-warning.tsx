import { Alert } from "@/components/shared";
import type { EligibilityWarning } from "@/lib/api/contracts";

interface EligibilityWarningListProps {
  warnings: EligibilityWarning[];
}

export function EligibilityWarningList({
  warnings,
}: Readonly<EligibilityWarningListProps>) {
  if (warnings.length === 0) return null;

  return (
    <div aria-label="資格提醒" className="space-y-2">
      {warnings.map((warning, index) => (
        <Alert key={`${warning.code}-${index}`} tone="warn">
          <strong>
            {warning.code === "cross_city" ? "跨城市活動提醒" : "資格提醒"}
          </strong>
          <p>{formatEligibilityWarningMessage(warning)}</p>
        </Alert>
      ))}
    </div>
  );
}

export function formatEligibilityWarningMessage(warning: EligibilityWarning) {
  if (warning.code === "cross_city") {
    if (warning.event_city && warning.employee_city) {
      return `此活動位於 ${warning.event_city}，你的登錄城市為 ${warning.employee_city}。`;
    }
    if (warning.event_city) {
      return `此活動位於 ${warning.event_city}，請確認後再報名。`;
    }
    return "此活動位於不同城市，請確認後再報名。";
  }
  return warning.message;
}
