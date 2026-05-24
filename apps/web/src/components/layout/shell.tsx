import { useMemo, useState } from "react";
import type { MouseEvent, ReactNode } from "react";
import type { ApiLogEntry, AuthSession } from "@/lib/api";
import type { NavItem, RouteKey, WorkspaceKey } from "@/app/routes";
import { navigate } from "@/app/routes";
import { roleLabel } from "@/lib/formatting";
import { runClientNavigation } from "@/lib/navigation";
import { DebugToggle } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import {
  ApiActivity,
  Header,
  StatusPanel,
  WorkspaceSwitch,
} from "@/components/layout";

type AuthenticatedShellProps = {
  activeRoute: NavItem;
  activeWorkspace: WorkspaceKey;
  apiLog: ApiLogEntry[];
  children: ReactNode;
  debugChromeAvailable: boolean;
  debugChromeEnabled: boolean;
  health: string;
  mockProfilesEnabled: boolean;
  navRoutes: NavItem[];
  onClearApiLog: () => void;
  onSwitchProfile?: () => void;
  onToggleDebugChrome: (enabled: boolean) => void;
  safeRoute: RouteKey;
  session: AuthSession;
  ready: string;
};

type NavLinkProps = {
  item: NavItem;
  route: RouteKey;
  compact?: boolean;
};

export function AuthenticatedShell({
  activeRoute,
  activeWorkspace,
  apiLog,
  children,
  debugChromeAvailable,
  debugChromeEnabled,
  health,
  mockProfilesEnabled,
  navRoutes,
  onClearApiLog,
  onSwitchProfile,
  onToggleDebugChrome,
  safeRoute,
  session,
  ready,
}: AuthenticatedShellProps) {
  return (
    <div
      className={`app-shell ${activeWorkspace}-workspace${
        debugChromeEnabled ? " debug-chrome" : ""
      }`}
    >
      <a className="skip-link" href="#main-workspace">
        跳到主要內容
      </a>
      <DesktopSidebar
        activeWorkspace={activeWorkspace}
        navRoutes={navRoutes}
        route={safeRoute}
        session={session}
      />
      <MobileTopBar
        activeRoute={activeRoute}
        apiLog={apiLog}
        debugChromeAvailable={debugChromeAvailable}
        debugChromeEnabled={debugChromeEnabled}
        health={health}
        mockProfilesEnabled={mockProfilesEnabled}
        onClearApiLog={onClearApiLog}
        onSwitchProfile={onSwitchProfile}
        onToggleDebugChrome={onToggleDebugChrome}
        ready={ready}
        session={session}
      />

      <main
        className="workspace"
        data-route={safeRoute}
        id="main-workspace"
        tabIndex={-1}
      >
        <Header
          apiLog={apiLog}
          route={safeRoute}
          session={session}
          debugChromeAvailable={debugChromeAvailable}
          debugChromeEnabled={debugChromeEnabled}
          health={health}
          mockProfilesEnabled={mockProfilesEnabled}
          onClearApiLog={onClearApiLog}
          onToggleDebugChrome={onToggleDebugChrome}
          onSwitchProfile={onSwitchProfile}
          ready={ready}
        />
        {children}
      </main>

      <MobileTabBar navRoutes={navRoutes} route={safeRoute} />
    </div>
  );
}

function DesktopSidebar({
  activeWorkspace,
  navRoutes,
  route,
  session,
}: {
  activeWorkspace: WorkspaceKey;
  navRoutes: NavItem[];
  route: RouteKey;
  session: AuthSession;
}) {
  return (
    <aside className="sidebar desktop-sidebar" aria-label="主要導覽">
      <BrandBlock />
      <WorkspaceSwitch active={activeWorkspace} role={session.actor.role} />
      <nav className="nav-list">
        <div className="nav-group-label">
          {activeWorkspace === "user" ? "員工工作區" : "管理工作台"}
        </div>
        {navRoutes.map((item) => (
          <DesktopNavLink item={item} key={item.key} route={route} />
        ))}
      </nav>
    </aside>
  );
}

function MobileTopBar({
  activeRoute,
  apiLog,
  debugChromeAvailable,
  debugChromeEnabled,
  health,
  mockProfilesEnabled,
  onClearApiLog,
  onSwitchProfile,
  onToggleDebugChrome,
  ready,
  session,
}: {
  activeRoute: NavItem;
  apiLog: ApiLogEntry[];
  debugChromeAvailable: boolean;
  debugChromeEnabled: boolean;
  health: string;
  mockProfilesEnabled: boolean;
  onClearApiLog: () => void;
  onSwitchProfile?: () => void;
  onToggleDebugChrome: (enabled: boolean) => void;
  ready: string;
  session: AuthSession;
}) {
  const displayName = session.claims.display_name || session.actor.id;
  return (
    <header className="mobile-topbar">
      <div className="mobile-topbar-title">
        <span className="brand-mark mobile-brand-mark" aria-hidden="true">
          C
        </span>
        <div>
          <span className="eyebrow">{activeRoute.eyebrow}</span>
          <h1>{activeRoute.label}</h1>
        </div>
      </div>
      <Sheet>
        <SheetTrigger asChild>
          <Button
            aria-label="開啟工具"
            className="mobile-tool-trigger"
            size="icon"
            type="button"
            variant="outline"
          >
            <Icon name="settings" />
          </Button>
        </SheetTrigger>
        <SheetContent className="mobile-utility-sheet" side="bottom">
          <SheetHeader>
            <SheetTitle>工作區工具</SheetTitle>
            <SheetDescription>
              身分、Debug、服務狀態與介接紀錄。
            </SheetDescription>
          </SheetHeader>
          <div className="mobile-identity-card" aria-label="目前登入身份">
            <strong>{displayName}</strong>
            <span>
              {session.actor.id} · {roleLabel(session.actor.role)}
            </span>
          </div>
          {debugChromeAvailable && (
            <DebugToggle
              enabled={debugChromeEnabled}
              onToggle={onToggleDebugChrome}
            />
          )}
          {debugChromeEnabled && mockProfilesEnabled && onSwitchProfile && (
            <Button variant="outline" type="button" onClick={onSwitchProfile}>
              <Icon name="logout" />
              切換身分
            </Button>
          )}
          {debugChromeEnabled && (
            <>
              <StatusPanel health={health} ready={ready} />
              <ApiActivity
                entries={apiLog}
                mode="sheet"
                onClear={onClearApiLog}
              />
            </>
          )}
        </SheetContent>
      </Sheet>
    </header>
  );
}

function MobileTabBar({
  navRoutes,
  route,
}: {
  navRoutes: NavItem[];
  route: RouteKey;
}) {
  const { directRoutes, overflowRoutes } = useMemo(
    () => mobileRouteGroups(navRoutes),
    [navRoutes],
  );
  const overflowActive = overflowRoutes.some((item) =>
    navItemActive(item.key, route),
  );
  const [routeSheetOpen, setRouteSheetOpen] = useState(false);

  return (
    <nav className="mobile-tabbar" aria-label="手機主要導覽">
      {directRoutes.map((item) => (
        <MobileNavLink item={item} key={item.key} route={route} compact />
      ))}
      {overflowRoutes.length > 0 && (
        <Sheet open={routeSheetOpen} onOpenChange={setRouteSheetOpen}>
          <SheetTrigger asChild>
            <Button
              aria-current={overflowActive ? "page" : undefined}
              aria-label="更多頁面"
              className={
                overflowActive
                  ? "mobile-tab-link mobile-tab-more active"
                  : "mobile-tab-link mobile-tab-more"
              }
              type="button"
              variant="ghost"
            >
              <Icon name="plus" />
              <span>更多</span>
            </Button>
          </SheetTrigger>
          <SheetContent className="mobile-route-sheet" side="bottom">
            <SheetHeader>
              <SheetTitle>更多頁面</SheetTitle>
              <SheetDescription>
                選擇此角色可存取的其他工作頁。
              </SheetDescription>
            </SheetHeader>
            <div className="mobile-route-list">
              {overflowRoutes.map((item) => (
                <MobileRouteSheetLink
                  item={item}
                  key={item.key}
                  route={route}
                  onNavigate={() => setRouteSheetOpen(false)}
                />
              ))}
            </div>
          </SheetContent>
        </Sheet>
      )}
    </nav>
  );
}

function BrandBlock() {
  return (
    <div className="brand-block">
      <div className="brand-mark" aria-hidden="true">
        C
      </div>
      <div>
        <div className="brand-title">企業活動票務</div>
        <div className="brand-subtitle">內部活動服務</div>
      </div>
    </div>
  );
}

function DesktopNavLink({ item, route }: NavLinkProps) {
  const active = navItemActive(item.key, route);
  return (
    <a
      className={active ? "nav-link active" : "nav-link"}
      href={item.path}
      aria-current={active ? "page" : undefined}
      onClick={handleNavClick(item.path)}
    >
      <span className="nav-icon" aria-hidden="true">
        <Icon name={item.icon} />
      </span>
      <span className="nav-copy">
        <strong>{item.label}</strong>
        <small>{item.eyebrow}</small>
      </span>
    </a>
  );
}

function MobileNavLink({ item, route }: NavLinkProps) {
  const active = navItemActive(item.key, route);
  return (
    <a
      className={active ? "mobile-tab-link active" : "mobile-tab-link"}
      href={item.path}
      aria-current={active ? "page" : undefined}
      onClick={handleNavClick(item.path)}
    >
      <Icon name={item.icon} />
      <span>{item.mobileLabel}</span>
    </a>
  );
}

function MobileRouteSheetLink({
  item,
  onNavigate,
  route,
}: NavLinkProps & { onNavigate: () => void }) {
  const active = navItemActive(item.key, route);
  return (
    <a
      className={active ? "mobile-route-link active" : "mobile-route-link"}
      href={item.path}
      aria-current={active ? "page" : undefined}
      onClick={(event) => {
        handleNavClick(item.path)(event);
        if (!event.defaultPrevented) return;
        onNavigate();
      }}
    >
      <span className="nav-icon" aria-hidden="true">
        <Icon name={item.icon} />
      </span>
      <span className="nav-copy">
        <strong>{item.label}</strong>
        <small>{item.description}</small>
      </span>
    </a>
  );
}

function mobileRouteGroups(navRoutes: NavItem[]) {
  const sortedRoutes = [...navRoutes]
    .filter((item) => item.mobilePrimary !== false)
    .sort((left, right) => left.mobileOrder - right.mobileOrder);
  const directCandidates = sortedRoutes.filter((item) => !item.mobileOverflow);
  const directRoutes =
    sortedRoutes.length <= 4 ? sortedRoutes : directCandidates.slice(0, 4);
  const directKeys = new Set(directRoutes.map((item) => item.key));
  const overflowRoutes = sortedRoutes.filter(
    (item) => !directKeys.has(item.key),
  );
  return { directRoutes, overflowRoutes };
}

function handleNavClick(path: string) {
  return (event: MouseEvent<HTMLAnchorElement>) =>
    runClientNavigation(event, () => navigate(path));
}

function navItemActive(item: RouteKey, route: RouteKey) {
  return (
    item === route || (item === "user-events" && route === "user-event-detail")
  );
}
