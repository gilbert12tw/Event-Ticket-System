import { useEffect, useState } from "react";
import { login, logout, me, readiness, setApiObserver } from "@/lib/api";
import type { ApiLogEntry, AuthSession } from "@/lib/api";
import { adminRoutes, canAccessRoute, currentRoute, defaultRouteForRole, navigate, routePath, routes, userRoutes } from "@/app/routes";
import type { RouteKey } from "@/app/routes";
import { errorMessage } from "@/lib/formatting";
import { Alert } from "@/components/shared";
import { ApiActivity, Header, LoadingScreen, StatusPanel, WorkspaceSwitch } from "@/components/layout";
import { LoginPage } from "@/features/auth/LoginPage";
import { AdminEventsPage, EmployeeEventDetailPage, EmployeeEventsPage } from "@/features/events/pages";
import { EmployeeTicketsPage } from "@/features/tickets/pages";
import { AdminRegistrationsPage } from "@/features/registrations/pages";
import { UserNotificationsPage, NotificationDeliveryPage } from "@/features/notifications/pages";
import { CheckinPage, OfflineCheckinBoundaryPage } from "@/features/checkin/pages";
import { HrReportsPage } from "@/features/reporting/pages";
import { HrSyncSettingsPage } from "@/features/hr-settings/pages";
import { AdminAuditPage } from "@/features/audit/pages";
import { DemoRunbookPage } from "@/features/demo-runbook/pages";
import { Icon } from "@/components/shared/icon";

function App() {
  const [route, setRoute] = useState<RouteKey>(currentRoute);
  const [auth, setAuth] = useState<AuthSession | null>(null);
  const [authLoading, setAuthLoading] = useState(true);
  const [authMessage, setAuthMessage] = useState("");
  const [apiLog, setApiLog] = useState<ApiLogEntry[]>([]);
  const [health, setHealth] = useState<"checking" | "ok" | "down">("checking");
  const [ready, setReady] = useState<"checking" | "ok" | "down">("checking");
  const unauthorizedRoute = auth ? !canAccessRoute(route, auth.actor.role) : false;
  const safeRoute = auth && unauthorizedRoute ? defaultRouteForRole(auth.actor.role) : route;
  const activeRoute = routes.find((candidate) => candidate.key === safeRoute) || routes[0];
  const activeWorkspace = activeRoute.workspace;
  const navRoutes = (activeWorkspace === "user" ? userRoutes : adminRoutes).filter((item) => auth && canAccessRoute(item.key, auth.actor.role));

  useEffect(() => {
    const onRoute = () => setRoute(currentRoute());
    window.addEventListener("popstate", onRoute);
    return () => window.removeEventListener("popstate", onRoute);
  }, []);

  useEffect(() => {
    setApiObserver((entry) => {
      setApiLog((entries) => [entry, ...entries].slice(0, 20));
    });
    return () => setApiObserver(null);
  }, []);

  useEffect(() => {
    let active = true;
    me()
      .then((session) => {
        if (!active) return;
        setAuth(session);
        setAuthMessage("");
      })
      .catch(() => {
        if (!active) return;
        setAuth(null);
      })
      .finally(() => {
        if (active) setAuthLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    let active = true;
    Promise.allSettled([readiness("/healthz"), readiness("/readyz")]).then(([healthResult, readyResult]) => {
      if (!active) return;
      setHealth(healthResult.status === "fulfilled" ? "ok" : "down");
      setReady(readyResult.status === "fulfilled" ? "ok" : "down");
    });
    return () => {
      active = false;
    };
  }, []);

  async function handleLogin(principalID: string) {
    setAuthMessage("");
    try {
      const session = await login(principalID);
      setAuth(session);
      navigate(routePath(defaultRouteForRole(session.actor.role)));
    } catch (error) {
      setAuthMessage(errorMessage(error));
    }
  }

  async function handleLogout() {
    setAuthMessage("");
    try {
      await logout();
    } catch (error) {
      setAuthMessage(errorMessage(error));
    } finally {
      setAuth(null);
    }
  }

  if (authLoading) {
    return <LoadingScreen />;
  }

  if (!auth) {
    return <LoginPage health={health} ready={ready} message={authMessage} onLogin={(principalID) => void handleLogin(principalID)} />;
  }

  return (
    <div className={`app-shell ${activeWorkspace}-workspace`}>
      <aside className="sidebar" aria-label="主要導覽">
        <div className="brand-block">
          <div className="brand-mark" aria-hidden="true">
            C
          </div>
          <div>
            <div className="brand-title">企業活動票務</div>
            <div className="brand-subtitle">Phase 1 MVP</div>
          </div>
        </div>
        <WorkspaceSwitch active={activeWorkspace} role={auth.actor.role} />
        <nav className="nav-list">
          <div className="nav-group-label">{activeWorkspace === "user" ? "User Workspace" : "Admin Console"}</div>
          {navRoutes.map((item) => (
            <a
              className={item.key === safeRoute ? "nav-link active" : "nav-link"}
              href={item.path}
              key={item.key}
              onClick={(event) => {
                event.preventDefault();
                navigate(item.path);
              }}
            >
              <span className="nav-icon" aria-hidden="true">
                <Icon name={item.icon} />
              </span>
              <span className="nav-copy">
                <strong>{item.label}</strong>
                <small>{item.eyebrow}</small>
              </span>
            </a>
          ))}
        </nav>
        <StatusPanel health={health} ready={ready} />
      </aside>

      <main className="workspace">
        <Header route={safeRoute} session={auth} onLogout={() => void handleLogout()} />
        {authMessage && <Alert tone="warn">{authMessage}</Alert>}
        {unauthorizedRoute ? (
          <UnauthorizedState
            requestedRoute={route}
            fallbackRoute={safeRoute}
            onReturn={() => navigate(routePath(safeRoute))}
          />
        ) : (
          <>
            {safeRoute === "user-events" && <EmployeeEventsPage employeeID={auth.actor.id} />}
            {safeRoute === "user-event-detail" && <EmployeeEventDetailPage employeeID={auth.actor.id} />}
            {safeRoute === "user-tickets" && <EmployeeTicketsPage employeeID={auth.actor.id} />}
            {safeRoute === "user-notifications" && <UserNotificationsPage employeeID={auth.actor.id} />}
            {safeRoute === "admin-events" && <AdminEventsPage />}
            {safeRoute === "admin-registrations" && <AdminRegistrationsPage />}
            {safeRoute === "admin-notifications" && <NotificationDeliveryPage />}
            {safeRoute === "admin-checkin" && <CheckinPage />}
            {safeRoute === "admin-offline-checkin" && <OfflineCheckinBoundaryPage />}
            {safeRoute === "admin-reports" && <HrReportsPage />}
            {safeRoute === "admin-hr-settings" && <HrSyncSettingsPage />}
            {safeRoute === "admin-audit" && <AdminAuditPage />}
            {safeRoute === "admin-demo" && <DemoRunbookPage session={auth} onSessionChange={(next) => setAuth(next)} />}
          </>
        )}
      </main>

      <ApiActivity entries={apiLog} onClear={() => setApiLog([])} />
    </div>
  );
}

function UnauthorizedState({
  requestedRoute,
  fallbackRoute,
  onReturn
}: {
  requestedRoute: RouteKey;
  fallbackRoute: RouteKey;
  onReturn: () => void;
}) {
  const requested = routes.find((candidate) => candidate.key === requestedRoute) || routes[0];
  const fallback = routes.find((candidate) => candidate.key === fallbackRoute) || routes[0];

  return (
    <section className="panel span-12">
      <div className="section-heading">
        <div>
          <h2>權限不足</h2>
          <p>目前登入角色無法進入「{requested.label}」。</p>
        </div>
      </div>
      <Alert tone="warn">
        請切換到你的可存取頁面，或使用對應角色的帳號重新登入。建議先回到「{fallback.label}」繼續操作。
      </Alert>
      <button className="button" type="button" onClick={onReturn}>
        返回預設頁面
      </button>
    </section>
  );
}

export default App;
