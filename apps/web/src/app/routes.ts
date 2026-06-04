import type { Role } from "@/lib/api";
import { debugChromePath } from "@/lib/ui/debug";
import routeData from "./routes-data.json";

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
  | "admin-ops"
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
  | "calendarPlus"
  | "chart"
  | "chevronLeft"
  | "chevronRight"
  | "clipboard"
  | "copy"
  | "database"
  | "download"
  | "logout"
  | "mapPin"
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

type NavItemSeed = readonly [
  key: RouteKey,
  path: string,
  label: string,
  mobileLabel: string,
  mobileOrder: number,
  description: string,
  icon: IconName,
  signals: readonly string[],
  extras?: Partial<Pick<NavItem, "mobileOverflow" | "mobilePrimary">>,
];

type RoutePattern = {
  pattern: RegExp;
  key: RouteKey;
  eventIDMatch?: number;
};

type RouteMatch = {
  key: RouteKey;
  eventID?: string;
};

const routeConfig = routeData as unknown as {
  userRouteSeeds: NavItemSeed[];
  adminRouteSeeds: NavItemSeed[];
  routeAliases: Record<string, RouteKey>;
  roleRouteAccess: Record<Role, RouteKey[]>;
};

const toNavItem =
  (workspace: WorkspaceKey, eyebrow: string) =>
  ([
    key,
    path,
    label,
    mobileLabel,
    mobileOrder,
    description,
    icon,
    signals,
    extras,
  ]: NavItemSeed): NavItem => ({
    key,
    workspace,
    path,
    label,
    mobileLabel,
    mobileOrder,
    eyebrow,
    description,
    icon,
    signals: [...signals],
    ...extras,
  });

export const routes: NavItem[] = [
  ...routeConfig.userRouteSeeds.map(toNavItem("user", "員工工作區")),
  ...routeConfig.adminRouteSeeds.map(toNavItem("admin", "管理工作台")),
];

export const routeAliases = routeConfig.routeAliases;

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

export const roleRouteAccess = routeConfig.roleRouteAccess;

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
  const match = routeMatchForPath(globalThis.location.pathname);
  preserveEventIDQuery(match);
  return match.key;
}

export function navigate(path: string, state: unknown = {}) {
  globalThis.history.pushState(state, "", debugChromePath(path));
  globalThis.dispatchEvent(new PopStateEvent("popstate"));
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
  const params = new URLSearchParams(globalThis.location.search);
  if (params.has("event_id")) return;
  params.set("event_id", match.eventID);
  const query = params.toString();
  const querySuffix = query ? `?${query}` : "";
  globalThis.history.replaceState(
    {},
    "",
    `${globalThis.location.pathname}${querySuffix}${globalThis.location.hash}`,
  );
}

function decodePathSegment(value: string) {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}
