import type { Role } from "@/lib/api";
import { debugChromePath } from "@/lib/ui/debug";

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
  | "arrowRight"
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
  mobileLabel: string;
  mobileOrder: number;
  mobilePrimary?: boolean;
  mobileOverflow?: boolean;
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
    mobileLabel: "活動",
    mobileOrder: 10,
    eyebrow: "員工工作區",
    description: "員工瀏覽活動、確認資格、報名或加入候補。",
    icon: "calendar",
    signals: ["資格檢核", "名額狀態", "報名結果"],
  },
  {
    key: "user-event-detail",
    workspace: "user",
    path: "/user/events/detail",
    label: "活動詳情",
    mobileLabel: "詳情",
    mobileOrder: 11,
    mobilePrimary: false,
    eyebrow: "員工工作區",
    description: "員工檢視單一活動的完整時間、資格、容量與目前報名狀態。",
    icon: "audit",
    signals: ["單筆查詢", "資格原因", "容量證據"],
  },
  {
    key: "user-tickets",
    workspace: "user",
    path: "/user/tickets",
    label: "我的票券",
    mobileLabel: "票券",
    mobileOrder: 20,
    eyebrow: "員工工作區",
    description: "員工查看電子票券與二維碼入場資訊。",
    icon: "ticket",
    signals: ["票券狀態", "二維碼入場", "簽章碼保護"],
  },
  {
    key: "user-notifications",
    workspace: "user",
    path: "/user/notifications",
    label: "通知中心",
    mobileLabel: "通知",
    mobileOrder: 30,
    eyebrow: "員工工作區",
    description: "員工查看報名、候補、票券與驗票相關通知。",
    icon: "bell",
    signals: ["站內通知", "電子郵件", "重試狀態"],
  },
  {
    key: "admin-events",
    workspace: "admin",
    path: "/admin/events",
    label: "活動設定",
    mobileLabel: "活動",
    mobileOrder: 10,
    eyebrow: "管理工作台",
    description: "活動主辦建立活動、設定容量、報名期間與資格規則。",
    icon: "activity",
    signals: ["發布檢查", "資格預覽", "稽核寫入"],
  },
  {
    key: "admin-registrations",
    workspace: "admin",
    path: "/admin/registrations",
    label: "報名治理",
    mobileLabel: "報名",
    mobileOrder: 20,
    eyebrow: "管理工作台",
    description: "管理活動報名清單、候補提升、取消報名與票券撤銷。",
    icon: "users",
    signals: ["候補提升", "取消報名", "撤銷票券"],
  },
  {
    key: "admin-notifications",
    workspace: "admin",
    path: "/admin/notifications",
    label: "通知投遞",
    mobileLabel: "通知",
    mobileOrder: 50,
    eyebrow: "管理工作台",
    description: "管理通知投遞、重試與失敗記錄。",
    icon: "send",
    signals: ["待投遞佇列", "重試", "投遞紀錄"],
  },
  {
    key: "admin-checkin",
    workspace: "admin",
    path: "/admin/checkin",
    label: "現場驗票",
    mobileLabel: "驗票",
    mobileOrder: 30,
    eyebrow: "管理工作台",
    description: "驗票員以線上簽章碼核銷，清楚處理首次與重複掃描。",
    icon: "scan",
    signals: ["首次核銷", "重複阻擋", "裝置追蹤"],
  },
  {
    key: "admin-offline-checkin",
    workspace: "admin",
    path: "/admin/checkin/offline",
    label: "離線驗票同步",
    mobileLabel: "離線",
    mobileOrder: 31,
    mobileOverflow: true,
    eyebrow: "管理工作台",
    description: "下載離線名單、同步掃描結果，並保留衝突稽核資料。",
    icon: "wifiOff",
    signals: ["名單快照", "同步衝突", "稽核保留"],
  },
  {
    key: "admin-reports",
    workspace: "admin",
    path: "/admin/reports",
    label: "人資報表",
    mobileLabel: "報表",
    mobileOrder: 40,
    eyebrow: "管理工作台",
    description: "人資檢視參與彙總、票券數、候補量與到場率。",
    icon: "chart",
    signals: ["彙總數據", "到場率", "最小個資"],
  },
  {
    key: "admin-hr-settings",
    workspace: "admin",
    path: "/admin/hr-settings",
    label: "人資同步設定",
    mobileLabel: "同步",
    mobileOrder: 60,
    eyebrow: "管理工作台",
    description: "管理人資屬性同步、欄位映射與手動匯入設定。",
    icon: "settings",
    signals: ["欄位映射", "同步狀態", "最小個資"],
  },
  {
    key: "admin-audit",
    workspace: "admin",
    path: "/admin/audit",
    label: "稽核查詢",
    mobileLabel: "稽核",
    mobileOrder: 70,
    eyebrow: "管理工作台",
    description: "系統管理員追蹤敏感操作、衝突與稽核中繼資料。",
    icon: "audit",
    signals: ["敏感操作", "中繼資料", "衝突追蹤"],
  },
  {
    key: "admin-demo",
    workspace: "admin",
    path: "/admin/flow-check",
    label: "流程檢查",
    mobileLabel: "流程",
    mobileOrder: 90,
    mobileOverflow: true,
    eyebrow: "管理工作台",
    description: "一鍵檢查活動建立、報名、候補、驗票、報表與稽核流程。",
    icon: "play",
    signals: ["流程檢查", "端到端", "可重跑"],
  },
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
  "/demo": "admin-demo",
  "/admin/demo": "admin-demo",
};

export const routeByPath = new Map<string, RouteKey>([
  ...routes.map((route) => [route.path, route.key] as const),
  ...Object.entries(routeAliases),
]);
const routePatterns: RoutePattern[] = [
  {
    pattern: /^\/user\/events\/([^/]+)$/,
    key: "user-event-detail",
    eventIDMatch: 1,
  },
  {
    pattern: /^\/admin\/events\/([^/]+)\/edit$/,
    key: "admin-events",
    eventIDMatch: 1,
  },
  {
    pattern: /^\/admin\/events\/([^/]+)\/eligibility$/,
    key: "admin-events",
    eventIDMatch: 1,
  },
  {
    pattern: /^\/admin\/events\/([^/]+)\/registrations$/,
    key: "admin-registrations",
    eventIDMatch: 1,
  },
];
export const userRoutes = routes.filter((route) => route.workspace === "user");
export const adminRoutes = routes.filter(
  (route) => route.workspace === "admin",
);

export const roleRouteAccess: Record<Role, RouteKey[]> = {
  employee: [
    "user-events",
    "user-event-detail",
    "user-tickets",
    "user-notifications",
  ],
  activity_admin: [
    "admin-events",
    "admin-registrations",
    "admin-notifications",
    "admin-demo",
  ],
  checkin_staff: ["admin-checkin", "admin-offline-checkin"],
  hr_admin: ["admin-reports", "admin-hr-settings", "admin-audit"],
  system_admin: [
    "admin-reports",
    "admin-hr-settings",
    "admin-audit",
    "admin-notifications",
  ],
};

export const demoSteps = [
  ["seed", "載入起始員工"],
  ["event", "建立已發布活動"],
  ["browse", "員工瀏覽資格"],
  ["book", "第一位合格員工報名"],
  ["waitlist", "第二位合格員工候補"],
  ["reject", "不合格員工被拒絕"],
  ["ticket", "顯示電子票券"],
  ["checkin", "完成首次驗票"],
  ["duplicate", "拒絕重複掃描"],
  ["report", "檢視報表與稽核"],
] as const;

export function routeMatchForPath(pathname: string): RouteMatch {
  const exact = routeByPath.get(pathname);
  if (exact) return { key: exact };

  for (const routePattern of routePatterns) {
    const match = routePattern.pattern.exec(pathname);
    if (!match) continue;
    const rawEventID = routePattern.eventIDMatch
      ? match[routePattern.eventIDMatch]
      : undefined;
    return {
      key: routePattern.key,
      eventID: rawEventID ? decodePathSegment(rawEventID) : undefined,
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
  window.history.pushState(state, "", debugChromePath(path));
  window.dispatchEvent(new PopStateEvent("popstate"));
}

export function canAccessRoute(route: RouteKey, role: Role) {
  return roleRouteAccess[role].includes(route);
}

export function defaultRouteForRole(role: Role): RouteKey {
  return roleRouteAccess[role][0];
}

export function defaultAdminRouteForRole(role: Role): RouteKey {
  const adminRoute = roleRouteAccess[role].find(
    (route) => routes.find((item) => item.key === route)?.workspace === "admin",
  );
  return adminRoute || defaultRouteForRole(role);
}

export function routePath(route: RouteKey) {
  return routes.find((item) => item.key === route)?.path || "/user/events";
}

export function ticketDetailPath(ticketID: string) {
  const params = new URLSearchParams();
  params.set("ticket_id", ticketID);
  return `/user/tickets?${params.toString()}`;
}

function preserveEventIDQuery(match: RouteMatch) {
  if (!match.eventID) return;
  const params = new URLSearchParams(window.location.search);
  if (params.has("event_id")) return;
  params.set("event_id", match.eventID);
  const query = params.toString();
  window.history.replaceState(
    {},
    "",
    `${window.location.pathname}${query ? `?${query}` : ""}${window.location.hash}`,
  );
}

function decodePathSegment(value: string) {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}
