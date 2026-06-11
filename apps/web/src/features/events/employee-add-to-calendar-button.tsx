import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";
import type { EventSummary } from "@/lib/api";
import { cn } from "@/lib/utils";
import {
  downloadCalendarArtifact,
  employeeCalendarExport,
} from "./employee-calendar-export";

export function EmployeeAddToCalendarButton({
  className,
  event,
  label,
  onDownloaded,
}: Readonly<{
  className?: string;
  event: EventSummary;
  label?: string;
  onDownloaded?: (filename: string) => void;
}>) {
  const accessibleLabel = `${label || "加入行事曆"}：${event.title}`;
  return (
    <Button
      aria-label={accessibleLabel}
      className={cn("employee-add-calendar-button", className)}
      size={label ? "sm" : "icon-sm"}
      title={accessibleLabel}
      type="button"
      variant="outline"
      onClick={() => {
        const artifact = downloadEventCalendar(event);
        onDownloaded?.(artifact.filename);
      }}
    >
      <Icon name="calendarPlus" />
      {label ? <span>{label}</span> : null}
    </Button>
  );
}

function downloadEventCalendar(event: EventSummary) {
  const artifact = employeeCalendarExport(event);
  downloadCalendarArtifact(artifact);
  return artifact;
}
