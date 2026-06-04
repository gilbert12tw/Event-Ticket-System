import { useState } from "react";
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
import { runClientNavigation } from "@/lib/navigation";
import { Icon } from "@/components/shared/icon";
import { AppPageHeader, SkeletonRows, StatusBadge } from "@/components/shared";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";

export function WorkspaceSwitch({
  active,
  role,
}: Readonly<{
  active: WorkspaceKey;
  role: Role;
}>) {
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
            onClick={(event) =>
              runClientNavigation(event, () => navigate(workspace.path))
            }
          >
            {workspace.label}
          </a>
        </Button>
      ))}
    </div>
  );
}

export function Header({
  apiLog,
  debugChromeAvailable,
  debugChromeEnabled,
  health,
  route,
  session,
  mockProfilesEnabled,
  onClearApiLog,
  onSwitchProfile,
  onToggleDebugChrome,
  ready,
}: Readonly<{
  apiLog: ApiLogEntry[];
  debugChromeAvailable: boolean;
  debugChromeEnabled: boolean;
  health: string;
  route: RouteKey;
  session: AuthSession;
  mockProfilesEnabled?: boolean;
  onClearApiLog: () => void;
  onSwitchProfile?: () => void;
  onToggleDebugChrome: (enabled: boolean) => void;
  ready: string;
}>) {
  const item = routes.find((candidate) => candidate.key === route) || routes[0];
  const displayName = session.claims.display_name || session.actor.id;
  const userWorkspace = item.workspace === "user";
  return (
    <AppPageHeader
      eyebrow={item.eyebrow}
      icon={item.icon}
      title={item.label}
      session={
        <div className="session-pill" aria-label="目前登入身份">
          <strong>{displayName}</strong>
          <span>
            {userWorkspace
              ? roleLabel(session.actor.role)
              : `${session.actor.id} · ${roleLabel(session.actor.role)}`}
          </span>
        </div>
      }
      utilities={
        debugChromeAvailable &&
        !userWorkspace && (
          <DebugToolsSheet
            apiLog={apiLog}
            debugChromeEnabled={debugChromeEnabled}
            health={health}
            ready={ready}
            session={session}
            mockProfilesEnabled={mockProfilesEnabled}
            onClearApiLog={onClearApiLog}
            onSwitchProfile={onSwitchProfile}
            onToggleDebugChrome={onToggleDebugChrome}
          />
        )
      }
    />
  );
}

function DebugToolsSheet({
  apiLog,
  debugChromeEnabled,
  health,
  mockProfilesEnabled,
  onClearApiLog,
  onSwitchProfile,
  onToggleDebugChrome,
  ready,
  session,
}: Readonly<{
  apiLog: ApiLogEntry[];
  debugChromeEnabled: boolean;
  health: string;
  mockProfilesEnabled?: boolean;
  onClearApiLog: () => void;
  onSwitchProfile?: () => void;
  onToggleDebugChrome: (enabled: boolean) => void;
  ready: string;
  session: AuthSession;
}>) {
  return (
    <Sheet>
      <SheetTrigger asChild>
        <Button
          aria-pressed={debugChromeEnabled}
          className="debug-tools-trigger"
          size="sm"
          type="button"
          variant="ghost"
        >
          <Icon name="settings" />
          Debug
        </Button>
      </SheetTrigger>
      <SheetContent className="debug-tools-sheet">
        <SheetHeader>
          <SheetTitle>Debug tools</SheetTitle>
          <SheetDescription>
            本機身分、服務狀態與介接紀錄集中在此管理。
          </SheetDescription>
        </SheetHeader>
        <div className="debug-tools-body">
          <section className="debug-tools-card" aria-label="Debug 模式">
            <div>
              <strong>Debug chrome</strong>
              <span>
                {debugChromeEnabled
                  ? "已顯示服務狀態與介接紀錄。"
                  : "啟用後才顯示本機除錯資訊。"}
              </span>
            </div>
            <Button
              aria-pressed={debugChromeEnabled}
              size="sm"
              type="button"
              variant={debugChromeEnabled ? "outline" : "default"}
              onClick={() => onToggleDebugChrome(!debugChromeEnabled)}
            >
              {debugChromeEnabled ? "停用 Debug" : "啟用 Debug"}
            </Button>
          </section>

          {debugChromeEnabled &&
            mockProfilesEnabled &&
            canAccessRoute("admin-demo", session.actor.role) && (
              <Button asChild variant="outline">
                <a
                  href={routePath("admin-demo")}
                  onClick={(event) =>
                    runClientNavigation(event, () =>
                      navigate(routePath("admin-demo")),
                    )
                  }
                >
                  <Icon name="play" />
                  執行流程檢查
                </a>
              </Button>
            )}

          {debugChromeEnabled && mockProfilesEnabled && onSwitchProfile && (
            <Button variant="outline" type="button" onClick={onSwitchProfile}>
              <Icon name="logout" />
              切換身分
            </Button>
          )}

          {debugChromeEnabled ? (
            <>
              <StatusPanel health={health} ready={ready} />
              <ApiActivity
                entries={apiLog}
                mode="sheet"
                onClear={onClearApiLog}
              />
            </>
          ) : (
            <p className="form-hint">
              Debug 關閉時，正式工作區不顯示介接紀錄或服務狀態。
            </p>
          )}
        </div>
      </SheetContent>
    </Sheet>
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
}: Readonly<{
  health: string;
  ready: string;
}>) {
  return (
    <div className="status-panel" aria-label="系統狀態">
      <div className="status-line">
        <span>應用服務</span>
        <StatusBadge tone={statusPanelTone(health)}>
          {serviceStatusLabel(health)}
        </StatusBadge>
      </div>
      <div className="status-line">
        <span>資料庫</span>
        <StatusBadge tone={statusPanelTone(ready)}>
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
}: Readonly<{
  entries: ApiLogEntry[];
  mode?: "floating" | "sheet";
  onClear: () => void;
}>) {
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
    if (value === null || value === undefined) return "null";
    if (typeof value === "string") return value;
    const serialized = JSON.stringify(value, null, 2);
    return serialized ?? "[unserializable]";
  } catch {
    return "[unserializable]";
  }
}

function statusPanelTone(value: string): "ok" | "neutral" | "fail" {
  if (value === "ok") return "ok";
  if (value === "checking") return "neutral";
  return "fail";
}

function serviceStatusLabel(status: string) {
  if (status === "checking") return "檢查中";
  if (status === "ok") return "正常";
  if (status === "down") return "異常";
  return status;
}
