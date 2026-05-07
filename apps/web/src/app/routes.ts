import type { Role } from "@/lib/api";

export type WorkspaceKey = "user" | "admin";
export type RouteKey =
  | "user-events"
  | "user-event-detail"
  | "user-tickets"
  | "user-notifications"
  | "admin-events"
  | "admin-registrations"
  | "admin-notifications"
  | "admin-checkin"
  | "admin-offline-checkin"
  | "admin-reports"
  | "admin-hr-settings"
  | "admin-audit"
  | "admin-demo";
export type StepState = "pending" | "running" | "done" | "fail";
export type IconName =
  | "activity"
  | "audit"
  | "ban"
  | "bell"
  | "calendar"
  | "chart"
  | "clipboard"
  | "copy"
  | "database"
  | "logout"
  | "play"
  | "plus"
  | "refresh"
  | "save"
  | "scan"
  | "send"
  | "settings"
  | "ticket"
  | "trash"
  | "users"
  | "wifiOff"
  | "x";

export type NavItem = {
  key: RouteKey;
  workspace: WorkspaceKey;
  path: string;
  label: string;
  eyebrow: string;
  description: string;
  icon: IconName;
  signals: string[];
};

type RoutePattern = {
  pattern: RegExp;
  key: RouteKey;
  eventIDMatch?: number;
};

type RouteMatch = {
  key: RouteKey;
  eventID?: string;
};

export const routes: NavItem[] = [
  {
    key: "user-events",
    workspace: "user",
    path: "/user/events",
    label: "活動探索",
    eyebrow: "User Workspace",
    description: "員工瀏覽活動、確認資格、報名或加入候補。",
    icon: "calendar",
    signals: ["資格檢核", "名額狀態", "報名結果"]
  },
  {
    key: "user-event-detail",
    workspace: "user",
    path: "/user/events/detail",
    label: "活動詳情",
    eyebrow: "User Workspace",
    description: "員工檢視單一活動的完整時間、資格、容量與目前報名狀態。",
    icon: "audit",
    signals: ["單筆查詢", "資格原因", "容量證據"]
  },
  {
    key: "user-tickets",
    workspace: "user",
    path: "/user/tickets",
    label: "我的票券",
    eyebrow: "User Workspace",
    description: "員工查看電子票券與 QR 入場資訊。",
    icon: "ticket",
    signals: ["票券狀態", "QR 入場", "Token 保護"]
  },
  {
    key: "user-notifications",
    workspace: "user",
    path: "/user/notifications",
    label: "通知中心",
    eyebrow: "User Workspace",
    description: "員工查看報名、候補、票券與驗票相關通知的產品占位頁。",
    icon: "bell",
    signals: ["站內通知", "電子郵件", "重試狀態"]
  },
  {
    key: "admin-events",
    workspace: "admin",
    path: "/admin/events",
    label: "活動設定",
    eyebrow: "Admin Console",
    description: "活動主辦建立活動、設定容量、報名期間與資格規則。",
    icon: "activity",
    signals: ["發布檢查", "資格預覽", "Audit 寫入"]
  },
  {
    key: "admin-registrations",
    workspace: "admin",
    path: "/admin/registrations",
    label: "報名治理",
    eyebrow: "Admin Console",
    description: "管理活動報名清單、候補提升、取消報名與票券撤銷。",
    icon: "users",
    signals: ["候補提升", "取消報名", "撤銷票券"]
  },
  {
    key: "admin-notifications",
    workspace: "admin",
    path: "/admin/notifications",
    label: "通知投遞",
    eyebrow: "Admin Console",
    description: "保留通知模板、投遞、重試與失敗記錄的 Phase 1 邊界。",
    icon: "send",
    signals: ["Outbox", "Retry", "Delivery log"]
  },
  {
    key: "admin-checkin",
    workspace: "admin",
    path: "/admin/checkin",
    label: "現場驗票",
    eyebrow: "Admin Console",
    description: "驗票員以線上 token 核銷，清楚處理首次與重複掃描。",
    icon: "scan",
    signals: ["首次核銷", "重複阻擋", "裝置追蹤"]
  },
  {
    key: "admin-offline-checkin",
    workspace: "admin",
    path: "/admin/checkin/offline",
    label: "離線驗票邊界",
    eyebrow: "Admin Console",
    description: "說明離線名單、同步、first-commit-wins 衝突與 audit 保留邊界。",
    icon: "wifiOff",
    signals: ["名單快照", "同步衝突", "Audit 保留"]
  },
  {
    key: "admin-reports",
    workspace: "admin",
    path: "/admin/reports",
    label: "HR 報表",
    eyebrow: "Admin Console",
    description: "HR 檢視參與彙總、票券數、候補量與到場率。",
    icon: "chart",
    signals: ["彙總數據", "到場率", "最小個資"]
  },
  {
    key: "admin-hr-settings",
    workspace: "admin",
    path: "/admin/hr-settings",
    label: "HR 同步設定",
    eyebrow: "Admin Console",
    description: "保留 HR 屬性同步、欄位映射與手動匯入的設定邊界。",
    icon: "settings",
    signals: ["欄位映射", "同步狀態", "最小個資"]
  },
  {
    key: "admin-audit",
    workspace: "admin",
    path: "/admin/audit",
    label: "Audit 查詢",
    eyebrow: "Admin Console",
    description: "系統管理員追蹤敏感操作、衝突與稽核 metadata。",
    icon: "audit",
    signals: ["敏感操作", "Metadata", "衝突追蹤"]
  },
  {
    key: "admin-demo",
    workspace: "admin",
    path: "/admin/demo",
    label: "Demo Runbook",
    eyebrow: "Admin Console",
    description: "一鍵驗證 Phase 1 MVP 端到端流程。",
    icon: "play",
    signals: ["AC-9", "端到端", "可重跑"]
  }
];

export const routeAliases: Record<string, RouteKey> = {
  "/": "user-events",
  "/employee/events": "user-events",
  "/employee/tickets": "user-tickets",
  "/admin/events/new": "admin-events",
  "/checkin": "admin-checkin",
  "/admin/offline-checkin": "admin-offline-checkin",
  "/admin/checkin/offline": "admin-offline-checkin",
  "/hr/reports": "admin-reports",
  "/admin/hr-sync": "admin-hr-settings",
  "/admin/settings": "admin-hr-settings",
  "/demo": "admin-demo"
};

export const routeByPath = new Map<string, RouteKey>([...routes.map((route) => [route.path, route.key] as const), ...Object.entries(routeAliases)]);
const routePatterns: RoutePattern[] = [
  { pattern: /^\/user\/events\/([^/]+)$/, key: "user-event-detail", eventIDMatch: 1 },
  { pattern: /^\/admin\/events\/([^/]+)\/edit$/, key: "admin-events", eventIDMatch: 1 },
  { pattern: /^\/admin\/events\/([^/]+)\/eligibility$/, key: "admin-events", eventIDMatch: 1 },
  { pattern: /^\/admin\/events\/([^/]+)\/registrations$/, key: "admin-registrations", eventIDMatch: 1 }
];
export const userRoutes = routes.filter((route) => route.workspace === "user");
export const adminRoutes = routes.filter((route) => route.workspace === "admin");

export const roleRouteAccess: Record<Role, RouteKey[]> = {
  employee: ["user-events", "user-event-detail", "user-tickets", "user-notifications"],
  activity_admin: ["admin-events", "admin-registrations", "admin-notifications", "admin-demo"],
  checkin_staff: ["admin-checkin", "admin-offline-checkin"],
  hr_admin: ["admin-events", "admin-registrations", "admin-notifications", "admin-reports", "admin-hr-settings", "admin-audit", "admin-demo"],
  system_admin: ["admin-reports", "admin-hr-settings", "admin-audit", "admin-notifications"]
};

export const localSSOPrincipals = [
  {
    id: "E1001",
    role: "employee" as Role,
    label: "Ariel Chen",
    group: "員工入口",
    description: "Engineering / Taipei / G6"
  },
  {
    id: "E1002",
    role: "employee" as Role,
    label: "Ben Lin",
    group: "員工入口",
    description: "Engineering / Taipei / G5"
  },
  {
    id: "E2001",
    role: "employee" as Role,
    label: "Carla Wu",
    group: "員工入口",
    description: "Sales / Taipei / G4"
  },
  {
    id: "admin-1",
    role: "activity_admin" as Role,
    label: "Activity Admin",
    group: "管理入口",
    description: "建立活動、設定容量與資格"
  },
  {
    id: "staff-1",
    role: "checkin_staff" as Role,
    label: "Check-in Staff",
    group: "管理入口",
    description: "現場驗票與重複掃描處理"
  },
  {
    id: "hr-1",
    role: "hr_admin" as Role,
    label: "HR Admin",
    group: "管理入口",
    description: "報表、audit 與參與彙總"
  },
  {
    id: "system-1",
    role: "system_admin" as Role,
    label: "System Admin",
    group: "管理入口",
    description: "Phase 1 HR reporting and audit alias"
  }
];

export const demoSteps = [
  ["seed", "建立 HR 示範員工"],
  ["event", "建立已發布活動"],
  ["browse", "員工瀏覽資格"],
  ["book", "第一位合格員工報名"],
  ["waitlist", "第二位合格員工候補"],
  ["reject", "不合格員工被拒絕"],
  ["ticket", "顯示電子票券"],
  ["checkin", "完成首次驗票"],
  ["duplicate", "拒絕重複掃描"],
  ["report", "檢視報表與 audit"]
] as const;

export function routeMatchForPath(pathname: string): RouteMatch {
  const exact = routeByPath.get(pathname);
  if (exact) return { key: exact };

  for (const routePattern of routePatterns) {
    const match = routePattern.pattern.exec(pathname);
    if (!match) continue;
    const rawEventID = routePattern.eventIDMatch ? match[routePattern.eventIDMatch] : undefined;
    return {
      key: routePattern.key,
      eventID: rawEventID ? decodePathSegment(rawEventID) : undefined
    };
  }

  return { key: "user-events" };
}

export function routeKeyForPath(pathname: string): RouteKey {
  return routeMatchForPath(pathname).key;
}

export function currentRoute(): RouteKey {
  const match = routeMatchForPath(window.location.pathname);
  preserveEventIDQuery(match);
  return match.key;
}

export function navigate(path: string, state: unknown = {}) {
  window.history.pushState(state, "", path);
  window.dispatchEvent(new PopStateEvent("popstate"));
}

export function canAccessRoute(route: RouteKey, role: Role) {
  return roleRouteAccess[role].includes(route);
}

export function defaultRouteForRole(role: Role): RouteKey {
  return roleRouteAccess[role][0];
}

export function defaultAdminRouteForRole(role: Role): RouteKey {
  const adminRoute = roleRouteAccess[role].find((route) => routes.find((item) => item.key === route)?.workspace === "admin");
  return adminRoute || defaultRouteForRole(role);
}

export function routePath(route: RouteKey) {
  return routes.find((item) => item.key === route)?.path || "/user/events";
}

function preserveEventIDQuery(match: RouteMatch) {
  if (!match.eventID) return;
  const params = new URLSearchParams(window.location.search);
  if (params.has("event_id")) return;
  params.set("event_id", match.eventID);
  const query = params.toString();
  window.history.replaceState({}, "", `${window.location.pathname}${query ? `?${query}` : ""}${window.location.hash}`);
}

function decodePathSegment(value: string) {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}
