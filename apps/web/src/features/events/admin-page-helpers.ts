import type { CreateEventRequest, UpdateEventRequest } from "@/lib/api";
import {
  defaultEditEventForm,
  defaultEventForm,
  splitTags,
  toISO,
} from "@/lib/formatting";
import type { Option } from "@/lib/ui/options";
import type { AdminEventTab } from "./admin-event-crud-types";

export function createBody(
  form: ReturnType<typeof defaultEventForm>,
): CreateEventRequest {
  const unlimited = form.capacity_type === "unlimited";
  return {
    title: form.title.trim(),
    description: form.description.trim(),
    location: form.location.trim(),
    event_city: form.event_city.trim() || undefined,
    event_site: form.event_site.trim() || undefined,
    starts_at: toISO(form.starts_at),
    ends_at: toISO(form.ends_at),
    registration_start: toISO(form.registration_start),
    registration_close: toISO(form.registration_close),
    capacity_type: form.capacity_type,
    capacity: unlimited ? null : Number(form.capacity),
    allows_family: unlimited,
    status: form.status,
    category: form.category.trim(),
    tags: splitTags(form.tags),
    entry_method: form.entry_method.trim(),
    visibility: form.visibility.trim(),
    rule: {
      department: form.department.trim() || "*",
      site: form.site.trim() || "*",
      min_grade: Number(form.min_grade),
      employment_status: form.employment_status.trim() || "active",
    },
  };
}

export function updateBody(
  editForm: ReturnType<typeof defaultEditEventForm>,
): UpdateEventRequest {
  const unlimited = editForm.capacity_type === "unlimited";
  return {
    title: editForm.title.trim(),
    description: editForm.description.trim(),
    location: editForm.location.trim(),
    event_city: editForm.event_city.trim() || undefined,
    event_site: editForm.event_site.trim() || undefined,
    starts_at: toISO(editForm.starts_at),
    ends_at: toISO(editForm.ends_at),
    registration_start: toISO(editForm.registration_start),
    registration_close: toISO(editForm.registration_close),
    capacity_type: editForm.capacity_type,
    capacity: unlimited ? null : Number(editForm.capacity),
    allows_family: unlimited,
    category: editForm.category.trim(),
    tags: splitTags(editForm.tags),
    entry_method: editForm.entry_method.trim(),
    visibility: editForm.visibility.trim(),
  };
}

export function eventSiteOptions(siteOptions: Option[]) {
  return [
    { value: "", label: "未設定" },
    ...siteOptions.filter((option) => option.value !== "*"),
  ];
}

export function initialTab(
  pathname = globalThis.location.pathname,
): AdminEventTab {
  if (pathname.includes("/new")) return "create";
  if (pathname.includes("/edit")) return "edit";
  if (pathname.includes("/eligibility")) return "eligibility";
  return "list";
}

export function windowReady(
  starts: string,
  ends: string,
  start: string,
  close: string,
) {
  const startsAt = new Date(starts);
  const endsAt = new Date(ends);
  const registrationStart = new Date(start);
  const registrationClose = new Date(close);
  return (
    [startsAt, endsAt, registrationStart, registrationClose].every(
      (date) => !Number.isNaN(date.getTime()),
    ) &&
    startsAt < endsAt &&
    registrationStart <= registrationClose &&
    registrationClose <= startsAt
  );
}
