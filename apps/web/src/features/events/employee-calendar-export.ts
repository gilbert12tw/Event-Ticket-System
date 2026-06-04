import type { EventSummary, Ticket } from "@/lib/api";

const defaultDurationMs = 2 * 60 * 60 * 1000;
const productID = "-//CETS//Employee Events//ZH-TW";

export type CalendarExportArtifact = {
  content: string;
  filename: string;
  mimeType: "text/calendar;charset=utf-8";
};

export function canAddToCalendar(event: EventSummary, ticket?: Ticket) {
  return (
    ticket?.status === "active" || event.current_user_status === "confirmed"
  );
}

export function employeeCalendarExport(
  event: EventSummary,
  now: Date = new Date(),
): CalendarExportArtifact {
  const startsAt = parseDate(event.starts_at);
  const endsAt = new Date(startsAt.getTime() + defaultDurationMs);
  const lines = [
    "BEGIN:VCALENDAR",
    "VERSION:2.0",
    "CALSCALE:GREGORIAN",
    "METHOD:PUBLISH",
    `PRODID:${productID}`,
    "BEGIN:VEVENT",
    `UID:${eventUID(event)}`,
    `DTSTAMP:${formatICSDate(now)}`,
    `DTSTART:${formatICSDate(startsAt)}`,
    `DTEND:${formatICSDate(endsAt)}`,
    `SUMMARY:${escapeICSValue(event.title)}`,
    `DESCRIPTION:${escapeICSValue(event.description || "公司活動")}`,
    `LOCATION:${escapeICSValue(event.location || event.event_site || "")}`,
    "END:VEVENT",
    "END:VCALENDAR",
  ];
  return {
    content: `${lines.map(foldICSLine).join("\r\n")}\r\n`,
    filename: calendarFilename(event, startsAt),
    mimeType: "text/calendar;charset=utf-8",
  };
}

function eventUID(event: EventSummary) {
  return `cets-${stableHash(`${event.event_id}:${event.starts_at}`)}@calendar.local`;
}

function stableHash(value: string) {
  let hash = 0x811c9dc5;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193);
  }
  return (hash >>> 0).toString(36);
}

function calendarFilename(event: EventSummary, startsAt: Date) {
  const datePrefix = [
    startsAt.getUTCFullYear(),
    String(startsAt.getUTCMonth() + 1).padStart(2, "0"),
    String(startsAt.getUTCDate()).padStart(2, "0"),
  ].join("-");
  const title = event.title
    .trim()
    .replace(/[\\/:*?"<>|#%{}~&]/g, "-")
    .replace(/\s+/g, "-")
    .replace(/-+/g, "-")
    .slice(0, 48)
    .replace(/^-|-$/g, "");
  return `${datePrefix}-${title || "event"}.ics`;
}

function escapeICSValue(value: string) {
  return value
    .replace(/\\/g, "\\\\")
    .replace(/\r\n|\r|\n/g, "\\n")
    .replace(/;/g, "\\;")
    .replace(/,/g, "\\,");
}

function foldICSLine(line: string) {
  if (line.length <= 75) return line;
  const chunks: string[] = [];
  let remaining = line;
  while (remaining.length > 75) {
    chunks.push(remaining.slice(0, 75));
    remaining = remaining.slice(75);
  }
  chunks.push(remaining);
  return chunks.join("\r\n ");
}

function formatICSDate(date: Date) {
  return [
    date.getUTCFullYear(),
    String(date.getUTCMonth() + 1).padStart(2, "0"),
    String(date.getUTCDate()).padStart(2, "0"),
    "T",
    String(date.getUTCHours()).padStart(2, "0"),
    String(date.getUTCMinutes()).padStart(2, "0"),
    String(date.getUTCSeconds()).padStart(2, "0"),
    "Z",
  ].join("");
}

function parseDate(value?: string) {
  const date = new Date(value || "");
  if (Number.isNaN(date.getTime())) return new Date(0);
  return date;
}
