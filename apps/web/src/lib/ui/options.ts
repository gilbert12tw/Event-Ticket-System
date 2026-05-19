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

export const capacityTypeOptions: Option[] = [
  {
    value: "limited",
    label: "限量票",
    helper: "本人一張票，容量會扣庫存。",
  },
  {
    value: "unlimited",
    label: "不限量",
    helper: "不扣庫存，可記錄家屬人數。",
  },
];

export const eventStatusOptions: Option[] = [
  { value: "draft", label: "草稿" },
  { value: "published", label: "已發布" },
  { value: "closed", label: "已關閉" },
  { value: "cancelled", label: "已取消" },
  { value: "archived", label: "已封存" },
];

export const eligibilityDepartmentOptions: Option[] = [
  { value: "*", label: "所有部門" },
  { value: "Engineering", label: "工程部" },
  { value: "Sales", label: "業務部" },
];

export const eligibilitySiteOptions: Option[] = [
  { value: "*", label: "所有廠區" },
  { value: "Taipei", label: "台北" },
];

export const gradeOptions: Option[] = Array.from({ length: 10 }, (_, index) => {
  const grade = String(index + 1);
  return { value: grade, label: `G${grade}+` };
});

export const employmentStatusOptions: Option[] = [
  { value: "*", label: "所有雇用狀態" },
  { value: "active", label: "在職" },
];

export const entryMethodOptions: Option[] = [
  { value: "qr", label: "二維碼驗票" },
  { value: "manual", label: "人工名單" },
];

export const visibilityOptions: Option[] = [
  { value: "eligible", label: "僅符合資格者" },
  { value: "internal", label: "內部可見" },
];

export const categoryOptions: Option[] = [
  { value: "", label: "不分類" },
  { value: "family", label: "家庭活動" },
  { value: "sports", label: "運動活動" },
  { value: "learning", label: "學習活動" },
  { value: "wellness", label: "健康活動" },
];

export const eventTemplateOptions: Option[] = [
  {
    value: "family",
    label: "家庭活動",
    helper: "不限量或大容量，預設開放家屬與 QR 入場。",
  },
  {
    value: "learning",
    label: "學習課程",
    helper: "限量座位，適合部門或職等條件。",
  },
  {
    value: "wellness",
    label: "健康活動",
    helper: "中等容量，適合跨部門活動。",
  },
  {
    value: "sports",
    label: "運動活動",
    helper: "限量名額，保留候補治理。",
  },
  {
    value: "company",
    label: "全公司活動",
    helper: "所有部門與廠區可見，預設草稿。",
  },
];

export const schedulePresetOptions: Option[] = [
  {
    value: "open-now",
    label: "立即開放，活動前 24 小時截止",
    helper: "適合短期公告與一般員工活動。",
  },
  {
    value: "one-week",
    label: "一週報名期",
    helper: "今天開放，一週後截止，活動安排在 10 天後。",
  },
  {
    value: "next-week",
    label: "下週開放，週五截止",
    helper: "適合先準備草稿再發布。",
  },
  { value: "custom", label: "自訂時間", helper: "保留目前手動輸入值。" },
];

export const venueOptions: Option[] = [
  { value: "台北總部禮堂", label: "台北總部禮堂", helper: "Taipei" },
  { value: "台北總部多功能廳", label: "台北總部多功能廳", helper: "Taipei" },
  { value: "台北總部訓練教室", label: "台北總部訓練教室", helper: "Taipei" },
  { value: "線上會議室", label: "線上會議室", helper: "不限廠區" },
];

export const devicePresetOptions: Option[] = [
  { value: "gate-1", label: "Gate 1", helper: "主要入口" },
  { value: "gate-2", label: "Gate 2", helper: "備援入口" },
  { value: "gate-mobile", label: "Mobile tablet", helper: "行動驗票設備" },
  { value: "gate-offline-1", label: "Offline gate 1", helper: "離線名單同步" },
  { value: "custom", label: "自訂裝置", helper: "必要時再手動輸入。" },
];

export const tagSuggestionOptions: Option[] = [
  { value: "家庭活動", label: "家庭活動" },
  { value: "台北", label: "台北" },
  { value: "學習", label: "學習" },
  { value: "健康", label: "健康" },
  { value: "運動", label: "運動" },
  { value: "全公司", label: "全公司" },
];

export const auditRoleOptions: Option[] = [
  { value: "", label: "所有角色" },
  { value: "employee", label: "員工" },
  { value: "activity_admin", label: "活動主辦" },
  { value: "checkin_staff", label: "驗票人員" },
  { value: "hr_admin", label: "人資管理員" },
  { value: "system_admin", label: "系統管理員" },
];

export const auditActionOptions: Option[] = [
  { value: "", label: "所有操作" },
  { value: "event.created", label: "活動建立" },
  { value: "event.updated", label: "活動更新" },
  { value: "event.state_changed", label: "活動狀態變更" },
  { value: "registration.confirmed", label: "報名確認" },
  { value: "registration.waitlisted", label: "加入候補" },
  { value: "registration.cancelled", label: "報名取消" },
  { value: "waitlist.promoted", label: "候補提升" },
  { value: "ticket.issued", label: "票券發行" },
  { value: "ticket.redeemed", label: "票券核銷" },
  { value: "checkin.rejected", label: "驗票拒絕" },
  { value: "offline_checkin.conflict", label: "離線衝突" },
  { value: "report.export.requested", label: "報表匯出" },
  { value: "eligibility_impact.created", label: "資格影響" },
  { value: "hr_sync.completed", label: "人資同步" },
];

export const auditEntityTypeOptions: Option[] = [
  { value: "", label: "所有物件" },
  { value: "event", label: "活動" },
  { value: "registration", label: "報名" },
  { value: "ticket", label: "票券" },
  { value: "checkin", label: "驗票" },
  { value: "report_export", label: "報表匯出" },
];

export const auditLimitOptions: Option[] = [
  { value: "25", label: "25 筆" },
  { value: "50", label: "50 筆" },
  { value: "100", label: "100 筆" },
  { value: "200", label: "200 筆" },
];

export const deliveryStatusOptions: Option[] = [
  { value: "all", label: "全部" },
  { value: "pending", label: "待處理" },
  { value: "failed", label: "失敗" },
  { value: "dead_letter", label: "投遞終止" },
  { value: "sent", label: "已送達" },
  { value: "suppressed", label: "已抑制" },
];

export const hrReviewStatusOptions: Option[] = [
  { value: "open", label: "待處理" },
  { value: "resolved", label: "已處理" },
  { value: "all", label: "全部" },
];

export const cancellationReasonOptions: Option[] = [
  { value: "employee cancellation", label: "個人行程變更" },
  { value: "duplicate registration", label: "重複報名" },
  { value: "manager request", label: "主管要求" },
  { value: "other", label: "其他原因" },
];

export const revocationReasonOptions: Option[] = [
  { value: "ticket issued by mistake", label: "票券誤發" },
  { value: "eligibility changed", label: "資格異動" },
  { value: "security review", label: "安全審核" },
  { value: "manager request", label: "主管要求" },
];

export const resolutionReasonOptions: Option[] = [
  { value: "人資已人工處置影響項目。", label: "人資已人工處置" },
  { value: "票券與資格狀態已確認。", label: "票券與資格已確認" },
  { value: "已通知活動主辦追蹤。", label: "已通知主辦追蹤" },
  { value: "custom", label: "自訂處置原因" },
];

export const reportPresetOptions: Option[] = [
  { value: "participation", label: "參與彙總", helper: "報名、票券、入場率。" },
  { value: "waitlist", label: "候補壓力", helper: "優先查看候補量。" },
  { value: "capacity", label: "容量利用", helper: "剩餘名額與使用率。" },
  { value: "exceptions", label: "驗票例外", helper: "低到場與異常活動。" },
];

const eventStatuses: Record<string, StatusView> = {
  draft: { label: "草稿", tone: "neutral" },
  published: { label: "已發布", tone: "ok" },
  closed: { label: "已關閉", tone: "warn" },
  cancelled: { label: "已取消", tone: "fail" },
  archived: { label: "已封存", tone: "neutral" },
};

const registrationStatuses: Record<string, StatusView> = {
  confirmed: { label: "已報名", tone: "ok" },
  waitlisted: { label: "候補中", tone: "warn" },
  cancelled: { label: "已取消", tone: "fail" },
  rejected: { label: "已拒絕", tone: "fail" },
};

const deliveryStatuses: Record<string, StatusView> = {
  sent: { label: "已送達", tone: "ok" },
  pending: { label: "待處理", tone: "warn" },
  failed: { label: "失敗", tone: "fail" },
  dead_letter: { label: "投遞終止", tone: "fail" },
  suppressed: { label: "已抑制", tone: "info" },
};

const auditActions: Record<string, StatusView> = {
  "booking.confirmed": { label: "報名確認", tone: "ok" },
  "booking.waitlisted": { label: "加入候補", tone: "warn" },
  "event.created": { label: "活動建立", tone: "info" },
  "event.updated": { label: "活動更新", tone: "info" },
  "event.state_changed": { label: "活動狀態變更", tone: "warn" },
  "registration.confirmed": { label: "報名確認", tone: "ok" },
  "registration.waitlisted": { label: "加入候補", tone: "warn" },
  "registration.cancelled": { label: "報名取消", tone: "warn" },
  "waitlist.promoted": { label: "候補提升", tone: "ok" },
  "waitlist.promotion_noop": { label: "候補無可提升", tone: "neutral" },
  "ticket.issued": { label: "票券核發", tone: "ok" },
  "ticket.redeemed": { label: "票券核銷", tone: "ok" },
  "ticket.revoked": { label: "票券撤銷", tone: "fail" },
  "checkin.rejected": { label: "驗票拒絕", tone: "fail" },
  "offline_checkin.conflict": { label: "離線驗票衝突", tone: "warn" },
  "report.export.requested": { label: "報表匯出請求", tone: "info" },
  "eligibility_impact.created": { label: "資格影響建立", tone: "warn" },
  "hr_sync.completed": { label: "人資同步完成", tone: "ok" },
};

const entityTypes: Record<string, string> = {
  event: "活動",
  registration: "報名",
  ticket: "票券",
  checkin: "驗票",
  report_export: "報表匯出",
  offline_checkin_batch: "離線驗票批次",
};

const roleLabels: Record<string, string> = {
  employee: "員工",
  activity_admin: "活動主辦",
  checkin_staff: "驗票人員",
  hr_admin: "人資管理員",
  system_admin: "系統管理員",
};

const departmentLabels: Record<string, string> = {
  Engineering: "工程部",
  Operations: "營運部",
  Sales: "業務部",
};

const siteLabels: Record<string, string> = {
  HQ: "總部",
  Taipei: "台北",
  "Taipei HQ": "台北總部",
  "Taipei HQ Auditorium": "台北總部禮堂",
};

const employmentStatusLabels: Record<string, string> = {
  active: "在職",
  inactive: "離職",
};

const channelLabels: Record<string, string> = {
  email: "電子郵件",
  "in-app": "站內通知",
  in_app: "站內通知",
};

const reportExportStatuses: Record<string, StatusView> = {
  pending: { label: "準備中", tone: "warn" },
  ready: { label: "已完成", tone: "ok" },
  failed: { label: "失敗", tone: "fail" },
};

const reviewStatuses: Record<string, StatusView> = {
  open: { label: "待處理", tone: "warn" },
  resolved: { label: "已處理", tone: "ok" },
};

export function eventStatusView(status: string): StatusView {
  return eventStatuses[status] || { label: status || "未知", tone: "info" };
}

export function registrationStatusView(status: string): StatusView {
  return (
    registrationStatuses[status] || { label: status || "未知", tone: "info" }
  );
}

export function ticketStatusView(status: string): StatusView {
  if (status === "active") return { label: "可使用", tone: "ok" };
  if (status === "revoked") return { label: "已撤銷", tone: "fail" };
  if (status === "redeemed") return { label: "已核銷", tone: "neutral" };
  return { label: status || "未知", tone: "neutral" };
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
  if (siteMatch) {
    return `廠區 ${siteLabel(siteMatch[1])} 不符合資格`;
  }
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
