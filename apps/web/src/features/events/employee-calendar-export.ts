import type { EventSummary, Ticket } from "@/lib/api";

const defaultDurationMs = 2 * 60 * 60 * 1000;
const productID = "-//CETS//Employee Events//ZH-TW";
const icsBackslash = String.fromCodePoint(92);
const icsEscapedBackslash = String.raw`\\`;
const icsEscapedNewLine = String.raw`\n`;
const icsEscapedSemicolon = String.raw`\;`;
const icsEscapedComma = String.raw`\,`;
const utf8Encoder = new TextEncoder();
const icsLineOctetLimit = 75;
const icsContinuationOctetLimit = 74;

export type CalendarExportArtifact = {
  content: string;
  filename: string;
  mimeType: "text/calendar;charset=utf-8";
};

export type CalendarExportEvent = Pick<
  EventSummary,
  | "description"
  | "ends_at"
  | "event_id"
  | "event_site"
  | "location"
  | "starts_at"
  | "title"
>;

export function canAddToCalendar(event: EventSummary, ticket?: Ticket) {
  return (
    ticket?.status === "active" || event.current_user_status === "confirmed"
  );
}

export function employeeCalendarExport(
  event: CalendarExportEvent,
  now: Date = new Date(),
): CalendarExportArtifact {
  const startsAt = parseDate(event.starts_at);
  const parsedEndsAt = parseDate(event.ends_at);
  const endsAt =
    parsedEndsAt.getTime() > startsAt.getTime()
      ? parsedEndsAt
      : new Date(startsAt.getTime() + defaultDurationMs);
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

function eventUID(event: CalendarExportEvent) {
  const uidSeed = `${event.event_id}:${event.starts_at}`;
  return `cets-${stableHash(uidSeed)}@calendar.local`;
}

function stableHash(value: string) {
  let hash = 0x811c9dc5;
  for (const character of value) {
    hash ^= character.codePointAt(0) ?? 0;
    hash = Math.imul(hash, 0x01000193);
  }
  return (hash >>> 0).toString(36);
}

function calendarFilename(event: CalendarExportEvent, startsAt: Date) {
  const datePrefix = [
    startsAt.getUTCFullYear(),
    String(startsAt.getUTCMonth() + 1).padStart(2, "0"),
    String(startsAt.getUTCDate()).padStart(2, "0"),
  ].join("-");
  const title = event.title
    .trim()
    .replaceAll(/[\\/:*?"<>|#%{}~&]/g, "-")
    .replaceAll(/\s+/g, "-")
    .replaceAll(/-+/g, "-")
    .slice(0, 48)
    .replaceAll(/^-|-$/g, "");
  return `${datePrefix}-${title || "event"}.ics`;
}

function escapeICSValue(value: string) {
  return value
    .replaceAll(icsBackslash, icsEscapedBackslash)
    .replaceAll(/\r\n|\r|\n/g, icsEscapedNewLine)
    .replaceAll(";", icsEscapedSemicolon)
    .replaceAll(",", icsEscapedComma);
}

function foldICSLine(line: string) {
  if (utf8OctetLength(line) <= icsLineOctetLimit) return line;
  const chunks: string[] = [];
  let chunk = "";
  let chunkOctets = 0;
  let octetLimit = icsLineOctetLimit;
  for (const character of line) {
    const characterOctets = utf8OctetLength(character);
    if (chunk && chunkOctets + characterOctets > octetLimit) {
      chunks.push(chunk);
      chunk = character;
      chunkOctets = characterOctets;
      octetLimit = icsContinuationOctetLimit;
      continue;
    }
    chunk += character;
    chunkOctets += characterOctets;
  }
  if (chunk) chunks.push(chunk);
  return chunks.join("\r\n ");
}

function utf8OctetLength(value: string) {
  return utf8Encoder.encode(value).length;
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
