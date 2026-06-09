import { useEffect, useState } from "react";
import {
  authBootstrap,
  clearProviderToken,
  me,
  readiness,
  selectMockProfile,
  setApiObserver,
} from "@/lib/api";
import type {
  ApiLogEntry,
  AuthBootstrap,
  AuthSession,
  MockProfile,
} from "@/lib/api";
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
import {
  cacheAuthSession,
  isOffline,
  loadCachedAuthSession,
} from "@/lib/offline/auth-cache";
import { Alert, DebugChromeGate, DebugToggle } from "@/components/shared";
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
import { OpsControlPlanePage } from "@/features/ops/pages";
import { HrReportsPage } from "@/features/reporting/pages";
import { HrSyncSettingsPage } from "@/features/hr-settings/pages";
import { AdminAuditPage } from "@/features/audit/pages";
import { DemoRunbookPage } from "@/features/demo-runbook/pages";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { runClientNavigation } from "@/lib/navigation";
import { isDebugChromeEnabled, setDebugChromeQuery } from "@/lib/ui/debug";

async function settle<T>(
  promise: Promise<T>,
): Promise<[T | null, Error | null]> {
  try {
    return [await promise, null];
  } catch (error) {
    return [null, error instanceof Error ? error : new Error(String(error))];
  }
}

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
  const [debugChromeAvailable, setDebugChromeAvailable] = useState(false);
  const [debugChrome, setDebugChrome] = useState(isDebugChromeEnabled);
  const [demoDebugAvailable, setDemoDebugAvailable] = useState(false);
  const [opsAPIAvailable, setOpsAPIAvailable] = useState(false);
  const canUseDemo = Boolean(auth && mockProfilesEnabled);
  const demoRouteBlocked = Boolean(
    auth && route === "admin-demo" && !canUseDemo,
  );
  const opsRouteBlocked = Boolean(
    auth && route === "admin-ops" && !opsAPIAvailable,
  );
  const unauthorizedRoute = auth
    ? !canAccessRoute(route, auth.actor.role) ||
      demoRouteBlocked ||
      opsRouteBlocked
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
      (item.key !== "admin-demo" || canUseDemo) &&
      (item.key !== "admin-ops" || opsAPIAvailable),
  );

  useEffect(() => {
    const onRoute = () => {
      setRoute(currentRoute());
      setDebugChrome(isDebugChromeEnabled(debugChromeAvailable));
    };
    globalThis.addEventListener("popstate", onRoute);
    return () => globalThis.removeEventListener("popstate", onRoute);
  }, [debugChromeAvailable]);

  useEffect(() => {
    setApiObserver((entry) => {
      setApiLog((entries) => [entry, ...entries].slice(0, 20));
    });
    return () => setApiObserver(null);
  }, []);

  useEffect(() => {
    let active = true;

    const resetDebugSessionState = () => {
      setMockProfilesEnabled(false);
      setMockProfiles([]);
      setDebugChromeAvailable(false);
      setDemoDebugAvailable(false);
      setOpsAPIAvailable(false);
      setDebugChrome(false);
      setDebugChromeQuery(false);
    };

    async function loadAuth() {
      try {
        const [session, sessionError] = await settle(me());
        if (!active) return;

        if (session) {
          cacheAuthSession(session);
        } else if (isOffline()) {
          const cached = loadCachedAuthSession();
          if (cached) {
            setAuth(cached);
            setAuthMessage("離線模式：使用已儲存的身分");
            return;
          }
        }

        setAuth(session ?? null);

        const [bootstrap, bootstrapError] = await settle(authBootstrap());
        if (!active) return;

        if (bootstrap) {
          applyBootstrap(bootstrap);
          setAuthMessage("");
          return;
        }

        resetDebugSessionState();
        setAuthMessage(
          errorMessage(
            bootstrapError ||
              sessionError ||
              new Error("bootstrap features are unavailable"),
          ),
        );
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
    const nextEnabled = debugChromeAvailable && enabled;
    setDebugChromeQuery(nextEnabled);
    setDebugChrome(isDebugChromeEnabled(debugChromeAvailable));
  }

  function applyBootstrap(bootstrap: AuthBootstrap) {
    const available = Boolean(bootstrap.debug_chrome_enabled);
    setMockProfilesEnabled(bootstrap.mock_profiles_enabled);
    setMockProfiles(bootstrap.mock_profiles);
    setDebugChromeAvailable(available);
    setDemoDebugAvailable(Boolean(bootstrap.demo_debug_enabled));
    setOpsAPIAvailable(Boolean(bootstrap.ops_api_enabled));
    if (!available) setDebugChromeQuery(false);
    setDebugChrome(isDebugChromeEnabled(available));
  }

  if (authLoading) {
    return <LoadingScreen />;
  }

  if (!auth) {
    if (mockProfilesEnabled) {
      return (
        <MockProfileSelector
          debugChromeAvailable={debugChromeAvailable}
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
        debugChromeAvailable={debugChromeAvailable}
        health={health}
        ready={ready}
        message={authMessage}
        onToggleDebugChrome={handleDebugToggle}
      />
    );
  }

  return (
    <AuthenticatedShell
      activeRoute={activeRoute}
      activeWorkspace={activeWorkspace}
      apiLog={apiLog}
      debugChromeAvailable={debugChromeAvailable}
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
          {safeRoute === "admin-ops" && <OpsControlPlanePage />}
          {safeRoute === "admin-reports" && <HrReportsPage />}
          {safeRoute === "admin-hr-settings" && <HrSyncSettingsPage />}
          {safeRoute === "admin-audit" && <AdminAuditPage />}
          {safeRoute === "admin-demo" && canUseDemo && (
            <DemoRunbookPage
              session={auth}
              demoDebugAvailable={demoDebugAvailable}
              onSessionChange={(next) => setAuth(next)}
            />
          )}
        </>
      )}
    </AuthenticatedShell>
  );
}

type AuthRequiredStateProps = {
  readonly debugChrome: boolean;
  readonly debugChromeAvailable: boolean;
  readonly health: string;
  readonly ready: string;
  readonly message: string;
  readonly onToggleDebugChrome: (enabled: boolean) => void;
};

function AuthRequiredState({
  debugChrome,
  debugChromeAvailable,
  health,
  ready,
  message,
  onToggleDebugChrome,
}: AuthRequiredStateProps) {
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
        {debugChromeAvailable && (
          <DebugToggle enabled={debugChrome} onToggle={onToggleDebugChrome} />
        )}
        <DebugChromeGate enabled={debugChrome}>
          <StatusPanel health={health} ready={ready} />
        </DebugChromeGate>
        {message && <Alert tone="warn">{message}</Alert>}
      </section>
    </main>
  );
}

type UnauthorizedStateProps = {
  readonly requestedRoute: RouteKey;
  readonly fallbackRoute: RouteKey;
  readonly onReturn: () => void;
};

function UnauthorizedState({
  requestedRoute,
  fallbackRoute,
  onReturn,
}: UnauthorizedStateProps) {
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
            onClick={(event) => runClientNavigation(event, onReturn)}
          >
            返回預設頁面
          </a>
        </Button>
      </section>
    </Card>
  );
}

export default App;
