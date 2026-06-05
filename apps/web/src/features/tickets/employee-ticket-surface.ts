import type { Ticket } from "@/lib/api";
import { formatDate } from "@/lib/formatting";
import { siteLabel } from "@/lib/ui/options";
import {
  type CalendarExportArtifact,
  employeeCalendarExport,
} from "@/features/events/employee-calendar-export";
import {
  addEmployeeCalendarDays,
  employeeCalendarWeekdayLabel,
  formatEmployeeCalendarMonthDay,
  localDateKey,
  parseEmployeeCalendarDateKey,
} from "@/features/events/employee-calendar-date";
import { ticketEntryReadinessView } from "./ticket-readiness";

export type EmployeeTicketSurfaceState = {
  actionLabel: string;
  canAddToCalendar: boolean;
  canShowQr: boolean;
  copy: string;
  kind:
    | "entry-ready"
    | "upcoming"
    | "qr-pending"
    | "redeemed"
    | "expired"
    | "revoked"
    | "unavailable";
  label: string;
  tone: "ok" | "warn" | "fail" | "neutral" | "info";
};

export type EmployeeTicketDateGroup = {
  dateKey: string;
  label: string;
  tickets: Ticket[];
};

export function employeeTicketSurfaceState(
  ticket: Ticket,
  now: Date = new Date(),
): EmployeeTicketSurfaceState {
  const readiness = ticketEntryReadinessView(ticket, now);
  if (readiness.kind === "entry-ready") {
    return {
      actionLabel: "出示票券",
      canAddToCalendar: true,
      canShowQr: true,
      copy: "入口出示 QR code 即可。",
      kind: "entry-ready",
      label: "可入場",
      tone: "ok",
    };
  }
  if (readiness.kind === "not-open") {
    return {
      actionLabel: "查看票券",
      canAddToCalendar: true,
      canShowQr: false,
      copy: "活動開始後再開啟票券出示 QR code。",
      kind: "upcoming",
      label: "即將到來",
      tone: "info",
    };
  }
  if (readiness.kind === "qr-pending") {
    return {
      actionLabel: "更新票券",
      canAddToCalendar: true,
      canShowQr: false,
      copy: readiness.copy,
      kind: "qr-pending",
      label: readiness.label,
      tone: readiness.tone,
    };
  }
  return {
    actionLabel: "查看票券",
    canAddToCalendar: false,
    canShowQr: false,
    copy: readiness.copy,
    kind: readiness.kind,
    label: readiness.kind === "redeemed" ? "已使用" : readiness.label,
    tone: readiness.tone,
  };
}

export function groupEmployeeTicketsByDate(
  tickets: Ticket[],
  now: Date = new Date(),
): EmployeeTicketDateGroup[] {
  const groups = new Map<string, Ticket[]>();
  for (const ticket of [...tickets].sort((left, right) =>
    compareTickets(left, right, now),
  )) {
    const dateKey = localDateKey(ticketDate(ticket));
    groups.set(dateKey, [...(groups.get(dateKey) ?? []), ticket]);
  }
  return [...groups.entries()].map(([dateKey, groupTickets]) => ({
    dateKey,
    label: ticketDateLabel(dateKey, now),
    tickets: groupTickets,
  }));
}

export function shouldShowCompanionCount(ticket: Ticket) {
  return (ticket.family_count ?? 0) > 0;
}

export function ticketCalendarExport(ticket: Ticket): CalendarExportArtifact {
  return employeeCalendarExport({
    description: "公司活動",
    ends_at: ticket.expires_at || ticket.event_starts_at || ticket.issued_at,
    event_id: ticket.event_id,
    event_site: ticket.event_location || "",
    location: ticket.event_location || "",
    starts_at: ticket.event_starts_at || ticket.issued_at,
    title: ticket.event_title || "公司活動",
  });
}

export function ticketTimeLocation(ticket: Ticket) {
  return [
    ticket.event_starts_at ? formatDate(ticket.event_starts_at) : "",
    ticket.event_location ? siteLabel(ticket.event_location) : "",
  ]
    .filter(Boolean)
    .join(" · ");
}

function compareTickets(left: Ticket, right: Ticket, now: Date) {
  const priority = ticketPriority(left, now) - ticketPriority(right, now);
  if (priority !== 0) return priority;
  const leftTime = ticketDate(left).getTime();
  const rightTime = ticketDate(right).getTime();
  if (ticketPriority(left, now) >= 3) return rightTime - leftTime;
  return leftTime - rightTime;
}

function ticketPriority(ticket: Ticket, now: Date) {
  const kind = employeeTicketSurfaceState(ticket, now).kind;
  if (kind === "entry-ready") return 0;
  if (kind === "upcoming") return 1;
  if (kind === "qr-pending") return 2;
  if (kind === "redeemed" || kind === "expired") return 3;
  return 4;
}

function ticketDate(ticket: Ticket) {
  const date = new Date(ticket.event_starts_at || ticket.issued_at);
  if (Number.isNaN(date.getTime())) return new Date(0);
  return date;
}

function ticketDateLabel(dateKey: string, now: Date) {
  const todayKey = localDateKey(now);
  const tomorrow = addEmployeeCalendarDays(now, 1);
  if (dateKey === todayKey) return "今天";
  if (dateKey === localDateKey(tomorrow)) return "明天";
  const date = parseEmployeeCalendarDateKey(dateKey, now);
  return `${formatEmployeeCalendarMonthDay(date)} ${employeeCalendarWeekdayLabel(date)}`;
}
