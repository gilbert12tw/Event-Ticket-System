import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";
import type { EventSummary } from "@/lib/api";
import { employeeCalendarExport } from "./employee-calendar-export";

export function EmployeeAddToCalendarButton({
  event,
}: Readonly<{ event: EventSummary }>) {
  return (
    <Button
      aria-label={`加入行事曆：${event.title}`}
      className="employee-add-calendar-button"
      size="icon-sm"
      title={`加入行事曆：${event.title}`}
      type="button"
      variant="outline"
      onClick={() => downloadEventCalendar(event)}
    >
      <Icon name="calendarPlus" />
    </Button>
  );
}

function downloadEventCalendar(event: EventSummary) {
  const artifact = employeeCalendarExport(event);
  const blob = new Blob([artifact.content], { type: artifact.mimeType });
  const url = globalThis.URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = artifact.filename;
  document.body.append(link);
  link.click();
  link.remove();
  globalThis.URL.revokeObjectURL(url);
}
