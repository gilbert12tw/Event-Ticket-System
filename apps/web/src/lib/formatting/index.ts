import {
  ApiError,
  type AuditLogFilters,
  type CreateEventRequest,
  type EmployeeProfile,
  type EventSummary,
  type Role,
} from "@/lib/api";
import { localizedMessage, roleViewLabel } from "@/lib/ui/options";

export function defaultEventForm() {
  return {
    title: "台北家庭電影夜",
    description: "員工家庭電影活動，支援報名、候補與現場驗票。",
    location: "台北總部禮堂",
    event_city: "Taipei",
    event_site: "Taipei HQ",
    starts_at: localInputDate(72),
    registration_start: localInputDate(-1),
    registration_close: localInputDate(48),
    capacity_type: "limited" as "limited" | "unlimited",
    capacity: "1",
    status: "published",
    department: "Engineering",
    site: "Taipei HQ",
    min_grade: "5",
    employment_status: "active",
    category: "family",
    tags: "家庭活動, 台北",
    entry_method: "qr",
    visibility: "eligible",
    // UI-only: tracks the currently-selected quick-setup option so the
    // dropdowns stay in sync with the form state. Never sent to the API.
    _selectedTemplate: "",
    _selectedSchedule: "custom",
  };
}

export function defaultEditEventForm() {
  return {
    title: "",
    description: "",
    location: "",
    event_city: "",
    event_site: "",
    starts_at: localInputDate(72),
    registration_start: localInputDate(-1),
    registration_close: localInputDate(48),
    capacity_type: "limited" as "limited" | "unlimited",
    capacity: "1",
    category: "",
    tags: "",
    entry_method: "qr",
    visibility: "eligible",
  };
}

export function editFormFromEvent(event: EventSummary) {
  const capacityType = event.capacity_type || "limited";
  return {
    title: event.title || "",
    description: event.description || "",
    location: event.location || "",
    event_city: event.event_city || "",
    event_site: event.event_site || "",
    starts_at: dateToLocalInput(event.starts_at),
    registration_start: dateToLocalInput(event.registration_start),
    registration_close: dateToLocalInput(event.registration_close),
    capacity_type: capacityType,
    capacity: capacityType === "unlimited" ? "" : String(event.capacity ?? 1),
    category: event.category || "",
    tags: (event.tags || []).join(", "),
    entry_method: event.entry_method || "qr",
    visibility: event.visibility || "eligible",
  };
}

export function splitTags(value: string) {
  const seen = new Set<string>();
  return value
    .split(",")
    .map((tag) => tag.trim())
    .filter((tag) => {
      if (!tag) return false;
      const key = tag.toLowerCase();
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
}

export function employeeMatchesRule(
  employee: EmployeeProfile,
  rule: Pick<
    CreateEventRequest["rule"],
    "department" | "site" | "min_grade" | "employment_status"
  >,
) {
  const department = rule.department.trim();
  const site = rule.site.trim();
  const employmentStatus = rule.employment_status.trim();
  return (
    (!department || department === "*" || employee.department === department) &&
    (!site || site === "*" || employee.site === site) &&
    employee.job_grade >= Number(rule.min_grade || 0) &&
    (!employmentStatus ||
      employmentStatus === "*" ||
      employee.employment_status === employmentStatus)
  );
}

export function localInputDate(hours: number) {
  const date = new Date(Date.now() + hours * 60 * 60 * 1000);
  date.setMinutes(date.getMinutes() - date.getTimezoneOffset());
  return date.toISOString().slice(0, 16);
}

export function dateToLocalInput(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  date.setMinutes(date.getMinutes() - date.getTimezoneOffset());
  return date.toISOString().slice(0, 16);
}

export function futureISO(hours: number) {
  return new Date(Date.now() + hours * 60 * 60 * 1000).toISOString();
}

export function toISO(value: string) {
  return new Date(value).toISOString();
}

export function normalizeAuditFilters(
  filters: AuditLogFilters,
): AuditLogFilters {
  return {
    ...filters,
    from: filters.from ? toISO(filters.from) : undefined,
    to: filters.to ? toISO(filters.to) : undefined,
    limit: filters.limit || "50",
  };
}

export function formatDate(value?: string) {
  if (!value) return "未設定";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-TW", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function roleLabel(role: Role) {
  return roleViewLabel(role);
}

export function errorMessage(error: unknown) {
  if (error instanceof ApiError) {
    return localizedMessage(error.response.error || error.message);
  }
  if (error instanceof Error) return localizedMessage(error.message);
  return localizedMessage(String(error));
}
