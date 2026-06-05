import type { CapacityType, EventSummary, Ticket } from "@/lib/api";
import type { Option } from "@/lib/ui/options";
import {
  type EmployeeCalendarViewMode,
  type EmployeeCalendarEvent,
} from "./employee-calendar-planner";
import { employeeEventDisplayState } from "./employee-calendar";

export type EmployeeDiscoveryStatus =
  | "all"
  | "bookable"
  | "registered"
  | "waitlisted";
export type EmployeeDiscoveryCapacity = "all" | CapacityType;

export type EmployeeDiscoveryState = {
  q: string;
  capacity: EmployeeDiscoveryCapacity;
  city: string;
  status: EmployeeDiscoveryStatus;
};

export const defaultEmployeeDiscoveryState: EmployeeDiscoveryState = {
  q: "",
  capacity: "all",
  city: "all",
  status: "all",
};

export const employeeDiscoveryStatusOptions: Option[] = [
  { value: "all", label: "全部" },
  { value: "bookable", label: "可報名" },
  { value: "registered", label: "已報名" },
  { value: "waitlisted", label: "候補" },
];

export const employeeDiscoveryCapacityOptions: Option[] = [
  { value: "all", label: "所有名額" },
  { value: "limited", label: "限量" },
  { value: "unlimited", label: "不限量" },
];

export function parseEmployeeDiscoveryQuery(
  searchParams: URLSearchParams | string,
): EmployeeDiscoveryState {
  const params =
    typeof searchParams === "string"
      ? new URLSearchParams(searchParams)
      : searchParams;
  return {
    q: (params.get("q") || "").trim(),
    capacity: discoveryCapacity(params.get("capacity") || ""),
    city: params.get("city") || "all",
    status: discoveryStatus(params.get("status") || ""),
  };
}

export function employeeEventsPath(
  view: EmployeeCalendarViewMode,
  dateKey: string,
  discovery: EmployeeDiscoveryState,
) {
  const params = new URLSearchParams();
  params.set("view", view);
  params.set("date", dateKey);
  const query = discovery.q.trim();
  if (query) params.set("q", query);
  if (discovery.capacity !== "all") params.set("capacity", discovery.capacity);
  if (discovery.city !== "all") params.set("city", discovery.city);
  if (discovery.status !== "all") params.set("status", discovery.status);
  return `/user/events?${params.toString()}`;
}

export function filterEmployeeDiscoveryEvents(
  events: EventSummary[],
  tickets: Ticket[],
  discovery: EmployeeDiscoveryState,
  now: Date,
) {
  const query = normalizeSearch(discovery.q);
  return events.filter((event) => {
    if (
      discovery.capacity !== "all" &&
      event.capacity_type !== discovery.capacity
    ) {
      return false;
    }
    if (discovery.city !== "all" && event.event_city !== discovery.city) {
      return false;
    }
    if (query && !eventSearchText(event).includes(query)) {
      return false;
    }
    if (discovery.status === "all") {
      return true;
    }
    const state = employeeEventDisplayState(
      event,
      tickets.find((ticket) => ticket.event_id === event.event_id),
      now,
    );
    if (discovery.status === "bookable") {
      return state.kind === "bookable";
    }
    if (discovery.status === "registered") {
      return state.kind === "registered" || state.kind === "entry-ready";
    }
    return state.kind === "waitlisted" || state.kind === "waitlist-available";
  });
}

export function employeeDiscoveryCityOptions(
  events: EventSummary[],
  selectedCity: string,
): Option[] {
  const cities = Array.from(
    new Set(events.map((event) => event.event_city).filter(isPresentString)),
  ).sort((left, right) => left.localeCompare(right, "zh-TW"));
  if (
    selectedCity !== "all" &&
    selectedCity &&
    !cities.includes(selectedCity)
  ) {
    cities.push(selectedCity);
  }
  return [
    { value: "all", label: "所有地點" },
    ...cities.map((city) => ({ value: city, label: city })),
  ];
}

export function employeeDiscoverySummary(
  view: EmployeeCalendarViewMode,
  events: EmployeeCalendarEvent[],
  discovery: EmployeeDiscoveryState,
) {
  const scope = employeeDiscoveryScope(view);
  const count = events.length;
  return discoveryIsActive(discovery)
    ? `${scope}符合 ${count} 場`
    : `${scope}有 ${count} 場活動`;
}

function employeeDiscoveryScope(view: EmployeeCalendarViewMode) {
  switch (view) {
    case "day":
      return "這天";
    case "month":
      return "本月";
    default:
      return "本週";
  }
}

export function discoveryIsActive(discovery: EmployeeDiscoveryState) {
  return (
    discovery.q.trim() !== "" ||
    discovery.capacity !== "all" ||
    discovery.city !== "all" ||
    discovery.status !== "all"
  );
}

function discoveryCapacity(value: string): EmployeeDiscoveryCapacity {
  return value === "limited" || value === "unlimited" ? value : "all";
}

function discoveryStatus(value: string): EmployeeDiscoveryStatus {
  return value === "bookable" ||
    value === "registered" ||
    value === "waitlisted"
    ? value
    : "all";
}

function eventSearchText(event: EventSummary) {
  return normalizeSearch(
    [
      event.title,
      event.description,
      event.location,
      event.event_city,
      event.event_site,
      event.category,
      ...(event.tags || []),
    ].join(" "),
  );
}

function normalizeSearch(value: string) {
  return value.trim().toLocaleLowerCase();
}

function isPresentString(value: string | undefined): value is string {
  return Boolean(value);
}
