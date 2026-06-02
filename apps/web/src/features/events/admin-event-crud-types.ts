import type { EmployeeProfile, EventSummary } from "@/lib/api";
import {
  defaultEditEventForm,
  defaultEventForm,
  employeeMatchesRule,
  localInputDate,
} from "@/lib/formatting";

export type AdminEventTab =
  | "list"
  | "create"
  | "edit"
  | "status"
  | "eligibility"
  | "danger";

export const adminEventTabs = [
  "list",
  "create",
  "edit",
  "status",
  "eligibility",
  "danger",
] as const;

export type AdminCreateForm = ReturnType<typeof defaultEventForm>;
export type AdminEditForm = ReturnType<typeof defaultEditEventForm>;
export type EventStateForm = {
  status: string;
  reason: string;
};

export function applyEventTemplate(form: AdminCreateForm, template: string) {
  if (!template) return form;
  const presets: Record<string, Partial<AdminCreateForm>> = {
    family: {
      title: "台北家庭電影夜",
      description: "邀請員工與家屬參與的企業活動。",
      category: "family",
      tags: "家庭活動, 台北",
      event_city: "Taipei",
      event_site: "Taipei HQ",
      capacity_type: "unlimited",
      capacity: "",
      department: "*",
      site: "Taipei",
    },
    learning: {
      title: "內部學習工作坊",
      description: "針對符合資格員工的限量學習活動。",
      category: "learning",
      tags: "學習, 台北",
      event_city: "Taipei",
      event_site: "Taipei HQ",
      capacity_type: "limited",
      capacity: "24",
      department: "Engineering",
      site: "Taipei",
    },
    wellness: {
      title: "健康生活講座",
      description: "跨部門健康促進活動。",
      category: "wellness",
      tags: "健康, 台北",
      event_city: "Taipei",
      event_site: "Taipei HQ",
      capacity_type: "limited",
      capacity: "80",
      department: "*",
      site: "Taipei",
    },
    sports: {
      title: "企業運動日",
      description: "限量報名並保留候補治理的運動活動。",
      category: "sports",
      tags: "運動, 台北",
      event_city: "Taipei",
      event_site: "Taipei HQ",
      capacity_type: "limited",
      capacity: "40",
      department: "*",
      site: "Taipei",
    },
    company: {
      title: "全公司交流活動",
      description: "開放所有部門與廠區員工報名。",
      category: "",
      tags: "全公司",
      event_city: "Taipei",
      event_site: "*",
      capacity_type: "limited",
      capacity: "240",
      department: "*",
      site: "*",
      status: "draft",
    },
  };
  return { ...form, ...presets[template] };
}

export function applySchedulePreset(form: AdminCreateForm, preset: string) {
  if (preset === "open-now") {
    return {
      ...form,
      registration_start: localInputDate(0),
      registration_close: localInputDate(48),
      starts_at: localInputDate(72),
    };
  }
  if (preset === "one-week") {
    return {
      ...form,
      registration_start: localInputDate(0),
      registration_close: localInputDate(24 * 7),
      starts_at: localInputDate(24 * 10),
    };
  }
  if (preset === "next-week") {
    return {
      ...form,
      registration_start: localInputDate(24 * 7),
      registration_close: localInputDate(24 * 11),
      starts_at: localInputDate(24 * 14),
    };
  }
  return form;
}

export function matchingEmployeesForEvent(
  employees: EmployeeProfile[],
  event?: EventSummary,
) {
  if (!event) return [];
  return employees.filter((employee) =>
    employeeMatchesRule(employee, {
      department: event.rule.department,
      site: event.rule.site,
      min_grade: event.rule.min_grade,
      employment_status: event.rule.employment_status,
    }),
  );
}
