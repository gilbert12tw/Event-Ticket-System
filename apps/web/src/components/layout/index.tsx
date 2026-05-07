import { useState } from "react";
import type { ApiLogEntry, AuthSession, Role } from "@/lib/api";
import type { RouteKey, WorkspaceKey } from "@/app/routes";
import { adminRoutes, canAccessRoute, defaultAdminRouteForRole, navigate, routePath, routes, userRoutes } from "@/app/routes";
import { principalLabel, roleLabel } from "@/lib/formatting";
import { Icon } from "@/components/shared/icon";
import { SkeletonRows, StatusBadge } from "@/components/shared";

export function WorkspaceSwitch({ active, role }: { active: WorkspaceKey; role: Role }) {
  const canUser = userRoutes.some((route) => canAccessRoute(route.key, role));
  const canAdmin = adminRoutes.some((route) => canAccessRoute(route.key, role));
  return (
    <div className="workspace-switch" aria-label="切換工作區">
      <button
        className={active === "user" ? "workspace-option active" : "workspace-option"}
        type="button"
        disabled={!canUser}
        onClick={() => navigate("/user/events")}
      >
        User
      </button>
      <button
        className={active === "admin" ? "workspace-option active" : "workspace-option"}
        type="button"
        disabled={!canAdmin}
        onClick={() => navigate(routePath(defaultAdminRouteForRole(role)))}
      >
        Admin
      </button>
    </div>
  );
}

export function Header({ route, session, onLogout }: { route: RouteKey; session: AuthSession; onLogout: () => void }) {
  const item = routes.find((candidate) => candidate.key === route) || routes[0];
  return (
    <header className={`page-header ${item.workspace}-header`}>
      <div className="page-title-block">
        <div className="page-kicker">
          <span className="page-icon" aria-hidden="true">
            <Icon name={item.icon} />
          </span>
          <span className="eyebrow">{item.eyebrow}</span>
        </div>
        <h1>{item.label}</h1>
        <p>{item.description}</p>
        <div className="signal-row" aria-label="頁面控制訊號">
          {item.signals.map((signal) => (
            <StatusBadge tone="neutral" key={signal}>
              {signal}
            </StatusBadge>
          ))}
        </div>
      </div>
      <div className="header-actions">
        <div className="session-pill" aria-label="目前登入身份">
          <strong>{principalLabel(session.actor.id)}</strong>
          <span>
            {session.actor.id} · {roleLabel(session.actor.role)}
          </span>
        </div>
        {canAccessRoute("admin-demo", session.actor.role) && (
          <button className="button ghost" type="button" onClick={() => navigate("/admin/demo")}>
            <Icon name="play" />
            跑完整 Demo
          </button>
        )}
        <button className="button secondary" type="button" onClick={onLogout}>
          <Icon name="logout" />
          登出
        </button>
      </div>
    </header>
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

export function StatusPanel({ health, ready }: { health: string; ready: string }) {
  return (
    <div className="status-panel" aria-label="系統狀態">
      <div className="status-line">
        <span>App</span>
        <StatusBadge tone={health === "ok" ? "ok" : health === "checking" ? "neutral" : "fail"}>
          {health === "checking" ? "checking" : health}
        </StatusBadge>
      </div>
      <div className="status-line">
        <span>PostgreSQL</span>
        <StatusBadge tone={ready === "ok" ? "ok" : ready === "checking" ? "neutral" : "fail"}>
          {ready === "checking" ? "checking" : ready}
        </StatusBadge>
      </div>
    </div>
  );
}

export function ApiActivity({ entries, onClear }: { entries: ApiLogEntry[]; onClear: () => void }) {
  const [collapsed, setCollapsed] = useState(true);
  return (
    <aside
      className={collapsed ? "api-panel collapsed" : "api-panel"}
      aria-label="API activity"
      data-state={collapsed ? "collapsed" : "expanded"}
    >
      <div className="api-header">
        <div>
          <strong>API Activity</strong>
          <small>tokens redacted</small>
        </div>
        <div className="api-actions">
          <button
            className="button secondary compact-button"
            type="button"
            aria-expanded={!collapsed}
            onClick={() => setCollapsed((current) => !current)}
          >
            {collapsed ? `顯示 ${entries.length}` : "收合"}
          </button>
          <button className="button icon-only ghost" type="button" onClick={onClear} aria-label="清除 API activity">
            <Icon name="x" />
          </button>
        </div>
      </div>
      {!collapsed && (
        <div className="api-log">
          {entries.length === 0 && <p className="form-hint">尚未呼叫 API。</p>}
          {entries.map((entry) => (
            <details className="api-entry" key={entry.id}>
              <summary>
                <StatusBadge tone={entry.ok ? "ok" : "fail"}>{entry.status}</StatusBadge>
                <span>{entry.label}</span>
              </summary>
              <pre>{JSON.stringify(entry.payload, null, 2)}</pre>
            </details>
          ))}
        </div>
      )}
    </aside>
  );
}
