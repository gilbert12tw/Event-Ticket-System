import { knownMessageMap } from "./messages";
import optionData from "./options-data.json";

export type Tone = "ok" | "warn" | "fail" | "info" | "neutral";

export type Option = {
  value: string;
  label: string;
  helper?: string;
};

export type StatusView = {
  label: string;
  tone: Tone;
};

type OptionSeed = readonly [value: string, label: string, helper?: string];
type ViewSeed = readonly [
  value: string,
  label: string,
  tone: Tone,
  helper?: string,
];
type OptionData = Record<string, readonly OptionSeed[]>;
type ViewData = Record<string, readonly ViewSeed[]>;

const optionRows = optionData as unknown as OptionData;
const viewRows = optionData as unknown as ViewData;

const toOptions = (rows: readonly OptionSeed[]): Option[] =>
  rows.map(([value, label, helper]) =>
    helper ? { value, label, helper } : { value, label },
  );

const toStatusOptions = (rows: readonly ViewSeed[]): Option[] =>
  rows.map((row) =>
    row[3]
      ? { value: row[0], label: row[1], helper: row[3] }
      : { value: row[0], label: row[1] },
  );
const viewOptionRows = (rows: readonly ViewSeed[]): OptionSeed[] =>
  rows.map(([value, label]) => [value, label]);

function viewMap(rows: readonly ViewSeed[]): Record<string, StatusView> {
  const result: Record<string, StatusView> = {};
  for (const [value, label, tone] of rows) result[value] = { label, tone };
  return result;
}

function labelMap(rows: readonly OptionSeed[]): Record<string, string> {
  const result: Record<string, string> = {};
  for (const [value, label] of rows) result[value] = label;
  return result;
}

const fallbackStatus = (
  views: Record<string, StatusView>,
  value: string,
  tone: Tone,
  label = "未知",
): StatusView => views[value] || { label: value || label, tone };

function fallbackLabel(
  labels: Record<string, string>,
  value: string | null | undefined,
  emptyLabel: string,
) {
  return value ? labels[value] || value : emptyLabel;
}

const eventStatusRows = viewRows.eventStatuses;
const registrationStatusRows = viewRows.registrationStatuses;
const ticketStatusRows = viewRows.ticketStatuses;
const deliveryStatusRows = viewRows.deliveryStatuses;
const reviewStatusRows = viewRows.reviewStatuses;
const roleRows = optionRows.roles;
const entityRows = optionRows.entities;
const departmentRows = optionRows.departments;
const siteRows = optionRows.sites;
const employmentStatusRows = optionRows.employmentStatuses;
const channelRows = optionRows.channels;

export const capacityTypeOptions = toOptions(optionRows.capacityTypes);
export const eventStatusOptions = toStatusOptions(eventStatusRows);
export const eligibilityDepartmentOptions = toOptions(
  optionRows.eligibilityDepartments,
);
export const eligibilitySiteOptions = toOptions(optionRows.eligibilitySites);
export const gradeOptions: Option[] = Array.from({ length: 10 }, (_, index) => {
  const grade = String(index + 1);
  return { value: grade, label: `G${grade}+` };
});
export const employmentStatusOptions = toOptions(
  optionRows.employmentStatusFilters,
);
export const entryMethodOptions = toOptions(optionRows.entryMethods);
export const visibilityOptions = toOptions(optionRows.visibilities);
export const categoryOptions = toOptions(optionRows.categories);
export const eventTemplateOptions = toOptions(optionRows.eventTemplates);
export const schedulePresetOptions = toOptions(optionRows.schedulePresets);
export const venueOptions = toOptions(optionRows.venues);
export const devicePresetOptions = toOptions(optionRows.devicePresets);
export const tagSuggestionOptions = toOptions(optionRows.tagSuggestions);

export const auditRoleOptions = toOptions([["", "所有角色"], ...roleRows]);
export const auditActionOptions = toOptions(optionRows.auditActions);
export const auditEntityTypeOptions = toOptions([
  ["", "所有物件"],
  ...entityRows,
]);
export const auditLimitOptions = toOptions(optionRows.auditLimits);
export const deliveryStatusOptions = toOptions([
  ["all", "全部"],
  ...viewOptionRows(deliveryStatusRows),
]);
export const hrReviewStatusOptions = toOptions(optionRows.hrReviewStatuses);
export const cancellationReasonOptions = toOptions(
  optionRows.cancellationReasons,
);
export const revocationReasonOptions = toOptions(optionRows.revocationReasons);
export const resolutionReasonOptions = toOptions(optionRows.resolutionReasons);
export const reportPresetOptions = toOptions(optionRows.reportPresets);

const eventStatuses = viewMap(eventStatusRows);
const registrationStatuses = viewMap(registrationStatusRows);
const ticketStatuses = viewMap(ticketStatusRows);
const deliveryStatuses = viewMap(deliveryStatusRows);
const reportExportStatuses = viewMap(viewRows.reportExportStatuses);
const reviewStatuses = viewMap(reviewStatusRows);
const checkinStatuses = viewMap(viewRows.checkinStatuses);
const auditActions = viewMap(viewRows.auditActionViews);

const entityTypes = labelMap(entityRows);
const roleLabels = labelMap(roleRows);
const departmentLabels = labelMap(departmentRows);
const siteLabels = labelMap(siteRows);
const employmentStatusLabels = labelMap(employmentStatusRows);
const channelLabels = labelMap(channelRows);

export const eventStatusView = (status: string): StatusView =>
  fallbackStatus(eventStatuses, status, "info");
export const registrationStatusView = (status: string): StatusView =>
  fallbackStatus(registrationStatuses, status, "info");
export const ticketStatusView = (status: string): StatusView =>
  fallbackStatus(ticketStatuses, status, "neutral");
export const deliveryStatusView = (status: string): StatusView =>
  fallbackStatus(deliveryStatuses, status, "info");
export const checkinStatusView = (
  status: string,
  duplicate = false,
): StatusView =>
  fallbackStatus(checkinStatuses, duplicate ? "duplicate" : status, "info");
export const auditActionView = (action: string): StatusView =>
  fallbackStatus(auditActions, action, "info", "未知操作");
export const entityTypeLabel = (value: string): string =>
  fallbackLabel(entityTypes, value, "未知物件");
export const roleViewLabel = (value: string): string =>
  fallbackLabel(roleLabels, value, "未知角色");
export const departmentLabel = (value?: string | null): string =>
  fallbackLabel(departmentLabels, value, "未設定");
export const siteLabel = (value?: string | null): string =>
  fallbackLabel(siteLabels, value, "未設定");
export const employmentStatusLabel = (value?: string | null): string =>
  fallbackLabel(employmentStatusLabels, value, "未設定");
export const channelLabel = (value?: string | null): string =>
  fallbackLabel(channelLabels, value, "系統");
export const reportExportStatusView = (status: string): StatusView =>
  fallbackStatus(reportExportStatuses, status, "info");
export const reviewStatusView = (status: string): StatusView =>
  fallbackStatus(reviewStatuses, status, "warn", "待處理");

export function eligibilityReasonLabel(
  reason: string,
  eligible: boolean,
): string {
  const trimmed = reason.trim();
  if (!trimmed) return eligible ? "符合資格" : "不符合資格";
  if (trimmed === "eligible") return "符合資格";
  if (trimmed === "employee not found") return "找不到員工資料";
  if (trimmed === "provider claims employee identity is required") {
    return "缺少員工身分宣告";
  }
  if (trimmed === "no_show_cooldown") return "缺席冷卻期間";
  const departmentMatch = /^department (.+) is not eligible$/.exec(trimmed);
  if (departmentMatch) {
    return `部門 ${departmentLabel(departmentMatch[1])} 不符合資格`;
  }
  const siteMatch = /^site (.+) is not eligible$/.exec(trimmed);
  if (siteMatch) return `廠區 ${siteLabel(siteMatch[1])} 不符合資格`;
  const employmentMatch = /^employment status (.+) is not eligible$/.exec(
    trimmed,
  );
  if (employmentMatch) {
    return `雇用狀態 ${employmentStatusLabel(employmentMatch[1])} 不符合資格`;
  }
  const gradeMatch = /^job grade (\d+) is below minimum (\d+)$/.exec(trimmed);
  if (gradeMatch) {
    return `你的職等 G${gradeMatch[1]} 未達此活動要求 G${gradeMatch[2]}+。`;
  }
  return trimmed;
}

const localizedMessagePatterns: readonly [RegExp, string][] = [
  [
    /^limited event booking blocked by no-show cooldown until (.+)$/,
    "限量活動因缺席冷卻期暫停報名，開放時間：$1。",
  ],
  [
    /^cannot transition event from (.+) to (.+)$/,
    "活動狀態不能從 $1 轉為 $2。",
  ],
];

export function localizedMessage(message: string): string {
  const trimmed = message.trim();
  if (!trimmed) return "操作未完成，請稍後再試。";
  if (knownMessageMap[trimmed]) return knownMessageMap[trimmed];
  for (const [pattern, replacement] of localizedMessagePatterns) {
    if (pattern.test(trimmed)) return trimmed.replace(pattern, replacement);
  }
  return trimmed;
}
