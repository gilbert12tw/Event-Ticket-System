import type { EventSummary } from "@/lib/api";

const currentEventWindowMs = 24 * 60 * 60 * 1000;

export function isCurrentEvent(
  event: Pick<EventSummary, "starts_at" | "status">,
  now: Date = new Date(),
) {
  if (event.status === "draft" || event.status === "archived") return false;
  const startsAt = parseTime(event.starts_at);
  if (startsAt === 0) return false;
  const nowTime = now.getTime();
  return startsAt <= nowTime && nowTime < startsAt + currentEventWindowMs;
}

export function selectCurrentEvent(
  events: EventSummary[],
  now: Date = new Date(),
): EventSummary | undefined {
  return events
    .filter((event) => isCurrentEvent(event, now))
    .sort(
      (left, right) => parseTime(right.starts_at) - parseTime(left.starts_at),
    )[0];
}

export function selectCurrentEventID(
  events: EventSummary[],
  currentID = "",
  now: Date = new Date(),
) {
  const currentEvent = selectCurrentEvent(events, now);
  if (currentEvent) return currentEvent.event_id;
  if (events.some((event) => event.event_id === currentID)) return currentID;
  return events[0]?.event_id ?? "";
}

export function sortEventsByManagementPriority(
  events: EventSummary[],
  now: Date = new Date(),
) {
  return [...events].sort((left, right) => {
    const priorityDiff =
      eventManagementPriority(left, now) - eventManagementPriority(right, now);
    if (priorityDiff !== 0) return priorityDiff;
    return parseTime(right.starts_at) - parseTime(left.starts_at);
  });
}

function eventManagementPriority(event: EventSummary, now: Date) {
  if (isCurrentEvent(event, now)) return 0;
  if (event.status === "published" && event.waitlist_count > 0) return 1;
  if (event.status === "published" && event.remaining_capacity === 0) return 2;
  if (event.status === "draft") return 3;
  if (event.status === "published") return 4;
  if (event.status === "archived") return 6;
  return 5;
}

function parseTime(value?: string) {
  if (!value) return 0;
  const time = new Date(value).getTime();
  return Number.isNaN(time) ? 0 : time;
}
