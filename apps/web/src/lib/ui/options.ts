import { knownMessageMap } from "./messages";

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

const eventStatusRows = [
  ["draft", "草稿", "neutral"],
  ["published", "已發布", "ok"],
  ["closed", "已關閉", "warn"],
  ["cancelled", "已取消", "fail"],
  ["archived", "已封存", "neutral"],
] as const satisfies readonly ViewSeed[];

const registrationStatusRows = [
  ["confirmed", "已報名", "ok"],
  ["waitlisted", "候補中", "warn"],
  ["cancelled", "已取消", "fail"],
  ["rejected", "已拒絕", "fail"],
] as const satisfies readonly ViewSeed[];

const ticketStatusRows = [
  ["active", "可使用", "ok"],
  ["revoked", "已撤銷", "fail"],
  ["redeemed", "已核銷", "neutral"],
] as const satisfies readonly ViewSeed[];

const deliveryStatusRows = [
  ["pending", "待處理", "warn"],
  ["failed", "失敗", "fail"],
  ["dead_letter", "投遞終止", "fail"],
  ["sent", "已送達", "ok"],
  ["suppressed", "已抑制", "info"],
] as const satisfies readonly ViewSeed[];

const reviewStatusRows = [
  ["open", "待處理", "warn"],
  ["pending", "待處理", "warn"],
  ["resolved", "已處理", "ok"],
] as const satisfies readonly ViewSeed[];

const roleRows = [
  ["employee", "員工"],
  ["activity_admin", "活動主辦"],
  ["checkin_staff", "驗票人員"],
  ["hr_admin", "人資管理員"],
  ["system_admin", "系統管理員"],
] as const satisfies readonly OptionSeed[];

const entityRows = [
  ["event", "活動"],
  ["registration", "報名"],
  ["ticket", "票券"],
  ["checkin", "驗票"],
  ["report_export", "報表匯出"],
  ["offline_checkin_batch", "離線驗票批次"],
] as const satisfies readonly OptionSeed[];

const departmentRows = [
  ["Engineering", "工程部"],
  ["Operations", "營運部"],
  ["Sales", "業務部"],
] as const satisfies readonly OptionSeed[];

const siteRows = [
  ["HQ", "總部"],
  ["Taipei", "台北"],
  ["Taipei HQ", "台北總部"],
  ["Taipei HQ Auditorium", "台北總部禮堂"],
] as const satisfies readonly OptionSeed[];

const employmentStatusRows = [
  ["active", "在職"],
  ["inactive", "離職"],
] as const satisfies readonly OptionSeed[];

const channelRows = [
  ["email", "電子郵件"],
  ["in-app", "站內通知"],
  ["in_app", "站內通知"],
] as const satisfies readonly OptionSeed[];

export const capacityTypeOptions = toOptions([
  ["limited", "限量票", "本人一張票，容量會扣庫存。"],
  ["unlimited", "不限量", "不扣庫存，可記錄家屬人數。"],
]);

export const eventStatusOptions = toStatusOptions(eventStatusRows);
export const eligibilityDepartmentOptions = toOptions([
  ["*", "所有部門"],
  ["Engineering", "工程部"],
  ["Sales", "業務部"],
]);
export const eligibilitySiteOptions = toOptions([
  ["*", "所有廠區"],
  ["Taipei", "台北"],
]);
export const gradeOptions: Option[] = Array.from({ length: 10 }, (_, index) => {
  const grade = String(index + 1);
  return { value: grade, label: `G${grade}+` };
});
export const employmentStatusOptions = toOptions([
  ["*", "所有雇用狀態"],
  ["active", "在職"],
]);
export const entryMethodOptions = toOptions([
  ["qr", "二維碼驗票"],
  ["manual", "人工名單"],
]);
export const visibilityOptions = toOptions([
  ["eligible", "僅符合資格者"],
  ["internal", "內部可見"],
]);
export const categoryOptions = toOptions([
  ["", "不分類"],
  ["family", "家庭活動"],
  ["sports", "運動活動"],
  ["learning", "學習活動"],
  ["wellness", "健康活動"],
]);

export const eventTemplateOptions = toOptions([
  ["family", "家庭活動", "不限量或大容量，預設開放家屬與 QR 入場。"],
  ["learning", "學習課程", "限量座位，適合部門或職等條件。"],
  ["wellness", "健康活動", "中等容量，適合跨部門活動。"],
  ["sports", "運動活動", "限量名額，保留候補治理。"],
  ["company", "全公司活動", "所有部門與廠區可見，預設草稿。"],
]);

export const schedulePresetOptions = toOptions([
  ["open-now", "立即開放，活動前 24 小時截止", "適合短期公告與一般員工活動。"],
  ["one-week", "一週報名期", "今天開放，一週後截止，活動安排在 10 天後。"],
  ["next-week", "下週開放，週五截止", "適合先準備草稿再發布。"],
  ["custom", "自訂時間", "保留目前手動輸入值。"],
]);

export const venueOptions = toOptions([
  ["台北總部禮堂", "台北總部禮堂", "Taipei"],
  ["台北總部多功能廳", "台北總部多功能廳", "Taipei"],
  ["台北總部訓練教室", "台北總部訓練教室", "Taipei"],
  ["線上會議室", "線上會議室", "不限廠區"],
]);

export const devicePresetOptions = toOptions([
  ["gate-1", "Gate 1", "主要入口"],
  ["gate-2", "Gate 2", "備援入口"],
  ["gate-mobile", "Mobile tablet", "行動驗票設備"],
  ["gate-offline-1", "Offline gate 1", "離線名單同步"],
  ["custom", "自訂裝置", "必要時再手動輸入。"],
]);

export const tagSuggestionOptions = toOptions([
  ["家庭活動", "家庭活動"],
  ["台北", "台北"],
  ["學習", "學習"],
  ["健康", "健康"],
  ["運動", "運動"],
  ["全公司", "全公司"],
]);

export const auditRoleOptions = toOptions([["", "所有角色"], ...roleRows]);
export const auditActionOptions = toOptions([
  ["", "所有操作"],
  ["event.created", "活動建立"],
  ["event.updated", "活動更新"],
  ["event.state_changed", "活動狀態變更"],
  ["registration.confirmed", "報名確認"],
  ["registration.waitlisted", "加入候補"],
  ["registration.cancelled", "報名取消"],
  ["waitlist.promoted", "候補提升"],
  ["ticket.issued", "票券發行"],
  ["ticket.redeemed", "票券核銷"],
  ["checkin.rejected", "驗票拒絕"],
  ["offline_checkin.conflict", "離線衝突"],
  ["report.export.requested", "報表匯出"],
  ["eligibility_impact.created", "資格影響"],
  ["hr_sync.completed", "人資同步"],
]);
export const auditEntityTypeOptions = toOptions([
  ["", "所有物件"],
  ...entityRows,
]);
export const auditLimitOptions = toOptions([
  ["25", "25 筆"],
  ["50", "50 筆"],
  ["100", "100 筆"],
  ["200", "200 筆"],
]);
export const deliveryStatusOptions = toOptions([
  ["all", "全部"],
  ...deliveryStatusRows,
]);
export const hrReviewStatusOptions = toOptions([
  ["open", "待處理"],
  ["resolved", "已處理"],
  ["all", "全部"],
]);
export const cancellationReasonOptions = toOptions([
  ["employee cancellation", "個人行程變更"],
  ["duplicate registration", "重複報名"],
  ["manager request", "主管要求"],
  ["other", "其他原因"],
]);
export const revocationReasonOptions = toOptions([
  ["ticket issued by mistake", "票券誤發"],
  ["eligibility changed", "資格異動"],
  ["security review", "安全審核"],
  ["manager request", "主管要求"],
]);
export const resolutionReasonOptions = toOptions([
  ["人資已人工處置影響項目。", "人資已人工處置"],
  ["票券與資格狀態已確認。", "票券與資格已確認"],
  ["已通知活動主辦追蹤。", "已通知主辦追蹤"],
  ["custom", "自訂處置原因"],
]);
export const reportPresetOptions = toOptions([
  ["participation", "參與彙總", "報名、票券、入場率。"],
  ["waitlist", "候補壓力", "優先查看候補量。"],
  ["capacity", "容量利用", "剩餘名額與使用率。"],
  ["exceptions", "驗票例外", "低到場與異常活動。"],
]);

const eventStatuses = viewMap(eventStatusRows);
const registrationStatuses = viewMap(registrationStatusRows);
const ticketStatuses = viewMap(ticketStatusRows);
const deliveryStatuses = viewMap(deliveryStatusRows);
const reportExportStatuses = viewMap([
  ["pending", "準備中", "warn"],
  ["ready", "已完成", "ok"],
  ["failed", "失敗", "fail"],
]);
const reviewStatuses = viewMap(reviewStatusRows);
const auditActions = viewMap([
  ["booking.confirmed", "報名確認", "ok"],
  ["booking.waitlisted", "加入候補", "warn"],
  ["event.created", "活動建立", "info"],
  ["event.updated", "活動更新", "info"],
  ["event.state_changed", "活動狀態變更", "warn"],
  ["registration.confirmed", "報名確認", "ok"],
  ["registration.waitlisted", "加入候補", "warn"],
  ["registration.cancelled", "報名取消", "warn"],
  ["waitlist.promoted", "候補提升", "ok"],
  ["waitlist.promotion_noop", "候補無可提升", "neutral"],
  ["ticket.issued", "票券核發", "ok"],
  ["ticket.redeemed", "票券核銷", "ok"],
  ["ticket.revoked", "票券撤銷", "fail"],
  ["checkin.rejected", "驗票拒絕", "fail"],
  ["offline_checkin.conflict", "離線驗票衝突", "warn"],
  ["report.export.requested", "報表匯出請求", "info"],
  ["eligibility_impact.created", "資格影響建立", "warn"],
  ["hr_sync.completed", "人資同步完成", "ok"],
]);

const entityTypes = labelMap(entityRows);
const roleLabels = labelMap(roleRows);
const departmentLabels = labelMap(departmentRows);
const siteLabels = labelMap(siteRows);
const employmentStatusLabels = labelMap(employmentStatusRows);
const channelLabels = labelMap(channelRows);

export function eventStatusView(status: string): StatusView {
  return eventStatuses[status] || { label: status || "未知", tone: "info" };
}

export function registrationStatusView(status: string): StatusView {
  return (
    registrationStatuses[status] || { label: status || "未知", tone: "info" }
  );
}

export function ticketStatusView(status: string): StatusView {
  return ticketStatuses[status] || { label: status || "未知", tone: "neutral" };
}

export function deliveryStatusView(status: string): StatusView {
  return deliveryStatuses[status] || { label: status || "未知", tone: "info" };
}

export function checkinStatusView(
  status: string,
  duplicate = false,
): StatusView {
  if (duplicate || status === "duplicate") {
    return { label: "重複掃描", tone: "warn" };
  }
  if (status === "accepted") return { label: "驗票成功", tone: "ok" };
  if (status === "rejected") return { label: "驗票拒絕", tone: "fail" };
  return { label: status || "未知", tone: "info" };
}

export function auditActionView(action: string): StatusView {
  return auditActions[action] || { label: action || "未知操作", tone: "info" };
}

export function entityTypeLabel(value: string): string {
  return entityTypes[value] || value || "未知物件";
}

export function roleViewLabel(value: string): string {
  return roleLabels[value] || value || "未知角色";
}

export function departmentLabel(value?: string | null): string {
  if (!value) return "未設定";
  return departmentLabels[value] || value;
}

export function siteLabel(value?: string | null): string {
  if (!value) return "未設定";
  return siteLabels[value] || value;
}

export function employmentStatusLabel(value?: string | null): string {
  if (!value) return "未設定";
  return employmentStatusLabels[value] || value;
}

export function channelLabel(value?: string | null): string {
  if (!value) return "系統";
  return channelLabels[value] || value;
}

export function reportExportStatusView(status: string): StatusView {
  return (
    reportExportStatuses[status] || { label: status || "未知", tone: "info" }
  );
}

export function reviewStatusView(status: string): StatusView {
  return reviewStatuses[status] || { label: status || "待處理", tone: "warn" };
}

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

export function localizedMessage(message: string): string {
  const trimmed = message.trim();
  if (!trimmed) return "操作未完成，請稍後再試。";
  if (knownMessageMap[trimmed]) return knownMessageMap[trimmed];
  if (
    /^limited event booking blocked by no-show cooldown until (.+)$/.test(
      trimmed,
    )
  ) {
    return trimmed.replace(
      /^limited event booking blocked by no-show cooldown until (.+)$/,
      "限量活動因缺席冷卻期暫停報名，開放時間：$1。",
    );
  }
  if (/^cannot transition event from (.+) to (.+)$/.test(trimmed)) {
    return trimmed.replace(
      /^cannot transition event from (.+) to (.+)$/,
      "活動狀態不能從 $1 轉為 $2。",
    );
  }
  return trimmed;
}
