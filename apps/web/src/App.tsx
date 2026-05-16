import { useEffect, useState } from "react";
import type { MouseEvent } from "react";
import {
  authBootstrap,
  clearProviderToken,
  me,
  readiness,
  selectMockProfile,
  setApiObserver,
} from "@/lib/api";
import type { ApiLogEntry, AuthSession, MockProfile } from "@/lib/api";
import {
  adminRoutes,
  canAccessRoute,
  currentRoute,
  defaultRouteForRole,
  navigate,
  routePath,
  routes,
  userRoutes,
} from "@/app/routes";
import type { RouteKey } from "@/app/routes";
import { errorMessage } from "@/lib/formatting";
import { Alert, DebugChromeGate } from "@/components/shared";
import { LoadingScreen, StatusPanel } from "@/components/layout";
import { AuthenticatedShell } from "@/components/layout/shell";
import { MockProfileSelector } from "@/features/auth/MockProfileSelector";
import {
  AdminEventsPage,
  EmployeeEventDetailPage,
  EmployeeEventsPage,
} from "@/features/events/pages";
import { EmployeeTicketsPage } from "@/features/tickets/pages";
import { AdminRegistrationsPage } from "@/features/registrations/pages";
import {
  UserNotificationsPage,
  NotificationDeliveryPage,
} from "@/features/notifications/pages";
import {
  CheckinPage,
  OfflineCheckinBoundaryPage,
} from "@/features/checkin/pages";
import { HrReportsPage } from "@/features/reporting/pages";
import { HrSyncSettingsPage } from "@/features/hr-settings/pages";
import { AdminAuditPage } from "@/features/audit/pages";
import { DemoRunbookPage } from "@/features/demo-runbook/pages";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { isDebugChromeEnabled, setDebugChromeQuery } from "@/lib/ui/debug";

function App() {
  const [route, setRoute] = useState<RouteKey>(currentRoute);
  const [auth, setAuth] = useState<AuthSession | null>(null);
  const [authLoading, setAuthLoading] = useState(true);
  const [authMessage, setAuthMessage] = useState("");
  const [mockProfilesEnabled, setMockProfilesEnabled] = useState(false);
  const [mockProfiles, setMockProfiles] = useState<MockProfile[]>([]);
  const [apiLog, setApiLog] = useState<ApiLogEntry[]>([]);
  const [health, setHealth] = useState<"checking" | "ok" | "down">("checking");
  const [ready, setReady] = useState<"checking" | "ok" | "down">("checking");
  const [debugChrome, setDebugChrome] = useState(isDebugChromeEnabled);
  const canUseDemo = Boolean(auth && mockProfilesEnabled);
  const demoRouteBlocked = auth ? route === "admin-demo" && !canUseDemo : false;
  const unauthorizedRoute = auth
    ? !canAccessRoute(route, auth.actor.role) || demoRouteBlocked
    : false;
  const safeRoute =
    auth && unauthorizedRoute ? defaultRouteForRole(auth.actor.role) : route;
  const activeRoute =
    routes.find((candidate) => candidate.key === safeRoute) || routes[0];
  const activeWorkspace = activeRoute.workspace;
  const navRoutes = (
    activeWorkspace === "user" ? userRoutes : adminRoutes
  ).filter(
    (item) =>
      auth &&
      item.key !== "user-event-detail" &&
      canAccessRoute(item.key, auth.actor.role) &&
      (item.key !== "admin-demo" || canUseDemo),
  );

  useEffect(() => {
    const onRoute = () => {
      setRoute(currentRoute());
      setDebugChrome(isDebugChromeEnabled());
    };
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
    async function loadAuth() {
      try {
        const session = await me();
        if (!active) return;
        setAuth(session);
        try {
          const bootstrap = await authBootstrap();
          if (!active) return;
          setMockProfilesEnabled(bootstrap.mock_profiles_enabled);
          setMockProfiles(bootstrap.mock_profiles);
        } catch {
          if (!active) return;
          setMockProfilesEnabled(false);
          setMockProfiles([]);
        }
        setAuthMessage("");
      } catch {
        if (!active) return;
        setAuth(null);
        try {
          const bootstrap = await authBootstrap();
          if (!active) return;
          setMockProfilesEnabled(bootstrap.mock_profiles_enabled);
          setMockProfiles(bootstrap.mock_profiles);
        } catch (error) {
          if (!active) return;
          setMockProfilesEnabled(false);
          setMockProfiles([]);
          setAuthMessage(errorMessage(error));
        }
      } finally {
        if (active) setAuthLoading(false);
      }
    }
    void loadAuth();
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    let active = true;
    Promise.allSettled([readiness("/healthz"), readiness("/readyz")]).then(
      ([healthResult, readyResult]) => {
        if (!active) return;
        setHealth(healthResult.status === "fulfilled" ? "ok" : "down");
        setReady(readyResult.status === "fulfilled" ? "ok" : "down");
      },
    );
    return () => {
      active = false;
    };
  }, []);

  async function handleMockProfile(profileID: string) {
    setAuthMessage("");
    try {
      const session = await selectMockProfile(profileID);
      setAuth(session);
      navigate(routePath(defaultRouteForRole(session.actor.role)));
    } catch (error) {
      setAuthMessage(errorMessage(error));
    }
  }

  function handleSwitchProfile() {
    setAuthMessage("");
    clearProviderToken();
    setAuth(null);
    navigate(routePath("user-events"));
  }

  function handleDebugToggle(enabled: boolean) {
    setDebugChromeQuery(enabled);
    setDebugChrome(isDebugChromeEnabled());
  }

  if (authLoading) {
    return <LoadingScreen />;
  }

  if (!auth) {
    if (mockProfilesEnabled) {
      return (
        <MockProfileSelector
          debugChromeEnabled={debugChrome}
          health={health}
          ready={ready}
          message={authMessage}
          profiles={mockProfiles}
          onToggleDebugChrome={handleDebugToggle}
          onSelect={(profileID) => void handleMockProfile(profileID)}
        />
      );
    }
    return (
      <AuthRequiredState
        debugChrome={debugChrome}
        health={health}
        ready={ready}
        message={authMessage}
      />
    );
  }

  return (
    <AuthenticatedShell
      activeRoute={activeRoute}
      activeWorkspace={activeWorkspace}
      apiLog={apiLog}
      debugChromeEnabled={debugChrome}
      health={health}
      mockProfilesEnabled={mockProfilesEnabled}
      navRoutes={navRoutes}
      onClearApiLog={() => setApiLog([])}
      onSwitchProfile={
        debugChrome && mockProfilesEnabled ? handleSwitchProfile : undefined
      }
      onToggleDebugChrome={handleDebugToggle}
      ready={ready}
      safeRoute={safeRoute}
      session={auth}
    >
      {authMessage && <Alert tone="warn">{authMessage}</Alert>}
      {unauthorizedRoute ? (
        <UnauthorizedState
          requestedRoute={route}
          fallbackRoute={safeRoute}
          onReturn={() => navigate(routePath(safeRoute))}
        />
      ) : (
        <>
          {safeRoute === "user-events" && (
            <EmployeeEventsPage claims={auth.claims} />
          )}
          {safeRoute === "user-event-detail" && (
            <EmployeeEventDetailPage claims={auth.claims} />
          )}
          {safeRoute === "user-tickets" && (
            <EmployeeTicketsPage claims={auth.claims} />
          )}
          {safeRoute === "user-notifications" && <UserNotificationsPage />}
          {safeRoute === "admin-events" && <AdminEventsPage />}
          {safeRoute === "admin-registrations" && <AdminRegistrationsPage />}
          {safeRoute === "admin-notifications" && <NotificationDeliveryPage />}
          {safeRoute === "admin-checkin" && <CheckinPage />}
          {safeRoute === "admin-offline-checkin" && (
            <OfflineCheckinBoundaryPage />
          )}
          {safeRoute === "admin-reports" && <HrReportsPage />}
          {safeRoute === "admin-hr-settings" && <HrSyncSettingsPage />}
          {safeRoute === "admin-audit" && <AdminAuditPage />}
          {safeRoute === "admin-demo" && canUseDemo && (
            <DemoRunbookPage
              session={auth}
              onSessionChange={(next) => setAuth(next)}
            />
          )}
        </>
      )}
    </AuthenticatedShell>
  );
}

function AuthRequiredState({
  debugChrome,
  health,
  ready,
  message,
}: {
  debugChrome: boolean;
  health: string;
  ready: string;
  message: string;
}) {
  return (
    <main className="login-shell">
      <section className="login-panel compact-login">
        <div className="brand-block login-brand">
          <div className="brand-mark" aria-hidden="true">
            C
          </div>
          <div>
            <div className="brand-title">企業活動票務</div>
            <div className="brand-subtitle">企業單一登入</div>
          </div>
        </div>
        <div>
          <div className="eyebrow">需要身分宣告</div>
          <h1>需要企業單一登入身分</h1>
          <p>
            請從企業身分提供者進入工作台，系統會使用身分宣告載入角色與員工屬性。
          </p>
        </div>
        <DebugChromeGate enabled={debugChrome}>
          <StatusPanel health={health} ready={ready} />
        </DebugChromeGate>
        {message && <Alert tone="warn">{message}</Alert>}
      </section>
    </main>
  );
}

function UnauthorizedState({
  requestedRoute,
  fallbackRoute,
  onReturn,
}: {
  requestedRoute: RouteKey;
  fallbackRoute: RouteKey;
  onReturn: () => void;
}) {
  const requested =
    routes.find((candidate) => candidate.key === requestedRoute) || routes[0];
  const fallback =
    routes.find((candidate) => candidate.key === fallbackRoute) || routes[0];

  return (
    <Card asChild className="panel span-12">
      <section>
        <div className="section-heading">
          <div>
            <h2>權限不足</h2>
            <p>目前登入角色無法進入「{requested.label}」。</p>
          </div>
        </div>
        <Alert tone="warn">
          請切換到你的可存取頁面，或使用對應角色的帳號重新登入。建議先回到「
          {fallback.label}」繼續操作。
        </Alert>
        <Button asChild>
          <a
            href={routePath(fallbackRoute)}
            onClick={(event) => {
              if (shouldUseNativeNavigation(event)) return;
              event.preventDefault();
              onReturn();
            }}
          >
            返回預設頁面
          </a>
        </Button>
      </section>
    </Card>
  );
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

export default App;
