import { useState } from "react";
import type { MouseEvent } from "react";
import type { ApiLogEntry, AuthSession, Role } from "@/lib/api";
import type { RouteKey, WorkspaceKey } from "@/app/routes";
import {
  adminRoutes,
  canAccessRoute,
  defaultAdminRouteForRole,
  navigate,
  routePath,
  routes,
  userRoutes,
} from "@/app/routes";
import { roleLabel } from "@/lib/formatting";
import { Icon } from "@/components/shared/icon";
import {
  AppPageHeader,
  DebugToggle,
  SkeletonRows,
  StatusBadge,
} from "@/components/shared";
import { Button } from "@/components/ui/button";
import { isDebugChromeAvailable } from "@/lib/ui/debug";

export function WorkspaceSwitch({
  active,
  role,
}: {
  active: WorkspaceKey;
  role: Role;
}) {
  const visibleWorkspaces = [
    {
      key: "user" as const,
      label: "員工",
      path: "/user/events",
      visible: userRoutes.some((route) => canAccessRoute(route.key, role)),
    },
    {
      key: "admin" as const,
      label: "管理",
      path: routePath(defaultAdminRouteForRole(role)),
      visible: adminRoutes.some((route) => canAccessRoute(route.key, role)),
    },
  ].filter((workspace) => workspace.visible);

  if (visibleWorkspaces.length <= 1) return null;

  return (
    <div className="workspace-switch" aria-label="切換工作區">
      {visibleWorkspaces.map((workspace) => (
        <Button
          asChild
          className={
            active === workspace.key
              ? "workspace-option active"
              : "workspace-option"
          }
          aria-current={active === workspace.key ? "page" : undefined}
          key={workspace.key}
          variant={active === workspace.key ? "secondary" : "ghost"}
        >
          <a
            href={workspace.path}
            onClick={(event) => {
              if (shouldUseNativeNavigation(event)) return;
              event.preventDefault();
              navigate(workspace.path);
            }}
          >
            {workspace.label}
          </a>
        </Button>
      ))}
    </div>
  );
}

export function Header({
  debugChromeEnabled,
  route,
  session,
  mockProfilesEnabled,
  onSwitchProfile,
  onToggleDebugChrome,
}: {
  debugChromeEnabled: boolean;
  route: RouteKey;
  session: AuthSession;
  mockProfilesEnabled?: boolean;
  onSwitchProfile?: () => void;
  onToggleDebugChrome: (enabled: boolean) => void;
}) {
  const item = routes.find((candidate) => candidate.key === route) || routes[0];
  const displayName = session.claims.display_name || session.actor.id;
  return (
    <AppPageHeader
      eyebrow={item.eyebrow}
      icon={item.icon}
      title={item.label}
      session={
        <div className="session-pill" aria-label="目前登入身份">
          <strong>{displayName}</strong>
          <span>
            {session.actor.id} · {roleLabel(session.actor.role)}
          </span>
        </div>
      }
      utilities={
        <>
          {isDebugChromeAvailable() && (
            <DebugToggle
              enabled={debugChromeEnabled}
              onToggle={onToggleDebugChrome}
            />
          )}
          {debugChromeEnabled &&
            mockProfilesEnabled &&
            canAccessRoute("admin-demo", session.actor.role) && (
              <Button asChild variant="ghost" size="sm">
                <a
                  href={routePath("admin-demo")}
                  onClick={(event) => {
                    if (shouldUseNativeNavigation(event)) return;
                    event.preventDefault();
                    navigate(routePath("admin-demo"));
                  }}
                >
                  <Icon name="play" />
                  執行流程檢查
                </a>
              </Button>
            )}
          {debugChromeEnabled && mockProfilesEnabled && onSwitchProfile && (
            <Button
              variant="ghost"
              size="sm"
              type="button"
              onClick={onSwitchProfile}
            >
              <Icon name="logout" />
              切換身分
            </Button>
          )}
        </>
      }
    />
  );
}

export function LoadingScreen() {
  return (
    <main className="login-shell">
      <section className="login-panel compact-login" aria-busy="true">
        <div className="brand-block login-brand">
          <div className="brand-mark" aria-hidden="true">
            C
          </div>
          <div>
            <div className="brand-title">企業活動票務</div>
            <div className="brand-subtitle">正在確認登入狀態</div>
          </div>
        </div>
        <SkeletonRows rows={2} />
      </section>
    </main>
  );
}

export function StatusPanel({
  health,
  ready,
}: {
  health: string;
  ready: string;
}) {
  return (
    <div className="status-panel" aria-label="系統狀態">
      <div className="status-line">
        <span>應用服務</span>
        <StatusBadge
          tone={
            health === "ok" ? "ok" : health === "checking" ? "neutral" : "fail"
          }
        >
          {serviceStatusLabel(health)}
        </StatusBadge>
      </div>
      <div className="status-line">
        <span>資料庫</span>
        <StatusBadge
          tone={
            ready === "ok" ? "ok" : ready === "checking" ? "neutral" : "fail"
          }
        >
          {serviceStatusLabel(ready)}
        </StatusBadge>
      </div>
    </div>
  );
}

export function ApiActivity({
  entries,
  mode = "floating",
  onClear,
}: {
  entries: ApiLogEntry[];
  mode?: "floating" | "sheet";
  onClear: () => void;
}) {
  const [collapsed, setCollapsed] = useState(mode !== "sheet");
  return (
    <aside
      className={
        collapsed ? `api-panel ${mode} collapsed` : `api-panel ${mode}`
      }
      aria-label="介接紀錄"
      data-state={collapsed ? "collapsed" : "expanded"}
      data-mode={mode}
    >
      <div className="api-header">
        <div>
          <strong>介接紀錄</strong>
          <small>簽章碼已遮蔽</small>
        </div>
        <div className="api-actions">
          <Button
            variant="outline"
            size="sm"
            type="button"
            aria-expanded={!collapsed}
            onClick={() => setCollapsed((current) => !current)}
          >
            {collapsed ? `顯示 ${entries.length}` : "收合"}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            type="button"
            onClick={onClear}
            aria-label="清除介接紀錄"
          >
            <Icon name="x" />
          </Button>
        </div>
      </div>
      {!collapsed && (
        <div className="api-log">
          {entries.length === 0 && <p className="form-hint">尚未呼叫介接。</p>}
          {entries.map((entry) => (
            <details className="api-entry" key={entry.id}>
              <summary>
                <StatusBadge tone={entry.ok ? "ok" : "fail"}>
                  {entry.status}
                </StatusBadge>
                <span>{entry.label}</span>
              </summary>
              <div className="api-body-grid">
                <section
                  className="api-body-block"
                  aria-label={`${entry.label} request body`}
                >
                  <span className="api-body-label">Request body</span>
                  <pre>{formatApiBody(entry.requestBody)}</pre>
                </section>
                <section
                  className="api-body-block"
                  aria-label={`${entry.label} response body`}
                >
                  <span className="api-body-label">Response body</span>
                  <pre>{formatApiBody(entry.responseBody)}</pre>
                </section>
              </div>
            </details>
          ))}
        </div>
      )}
    </aside>
  );
}

function formatApiBody(value: unknown) {
  try {
    return JSON.stringify(value, null, 2) ?? "null";
  } catch {
    return String(value);
  }
}

function serviceStatusLabel(status: string) {
  if (status === "checking") return "檢查中";
  if (status === "ok") return "正常";
  if (status === "down") return "異常";
  return status;
}

function shouldUseNativeNavigation(event: MouseEvent<HTMLAnchorElement>) {
  return (
    event.button !== 0 ||
    event.metaKey ||
    event.ctrlKey ||
    event.altKey ||
    event.shiftKey
  );
}
