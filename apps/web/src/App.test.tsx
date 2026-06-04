import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AuthSession } from "@/lib/api";
import App from "./App";
import {
  authBootstrap,
  clearProviderToken,
  getOpsDashboard,
  listEvents,
  me,
  readiness,
  reports,
  selectMockProfile,
} from "@/lib/api";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    authBootstrap: vi.fn(),
    clearProviderToken: vi.fn(),
    getOpsDashboard: vi.fn(),
    listEvents: vi.fn(),
    me: vi.fn(),
    readiness: vi.fn(),
    reports: vi.fn(),
    selectMockProfile: vi.fn(),
    setApiObserver: vi.fn(),
  };
});

vi.mock("@/features/demo-runbook/pages", () => ({
  DemoRunbookPage: ({
    session,
    onSessionChange,
  }: {
    session: AuthSession;
    onSessionChange: (session: AuthSession | null) => void;
  }) => (
    <button type="button" onClick={() => onSessionChange(session)}>
      restore demo session
    </button>
  ),
}));

const mockMe = vi.mocked(me);
const mockAuthBootstrap = vi.mocked(authBootstrap);
const mockReadiness = vi.mocked(readiness);
const mockSelectMockProfile = vi.mocked(selectMockProfile);
const mockClearProviderToken = vi.mocked(clearProviderToken);
const mockGetOpsDashboard = vi.mocked(getOpsDashboard);
const mockListEvents = vi.mocked(listEvents);
const mockReports = vi.mocked(reports);

describe("App", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    globalThis.history.pushState({}, "", "/");
    mockListEvents.mockResolvedValue([]);
    mockReports.mockResolvedValue([]);
    mockGetOpsDashboard.mockResolvedValue({
      capacity_pressure: { events: [] },
      queues: { queues: [] },
      reports_freshness: { projections: [] },
      dead_letter_recent: [],
    });
  });

  it("renders unauthorized state when an invalid role enters a forbidden route", async () => {
    const session: AuthSession = {
      actor: {
        id: "staff-1",
        role: "checkin_staff",
      },
      expires_at: "2026-05-06T10:00:00Z",
      claims: {
        employee_id: "staff-1",
        display_name: "Staff One",
        role_claims: ["checkin_staff"],
        mapped_roles: ["checkin_staff"],
        department: "Operations",
        site: "Taipei HQ",
        city: "Taipei",
        grade: 5,
        employment_status: "active",
        claims_status: "complete",
      },
      source: "provider",
    };

    mockMe.mockResolvedValueOnce(session);
    mockAuthBootstrap.mockResolvedValue({
      mock_profiles_enabled: false,
      mock_profiles: [],
      debug_chrome_enabled: false,
    });
    mockReadiness.mockResolvedValue({});

    globalThis.history.pushState({}, "", "/admin/events");

    render(<App />);

    await waitFor(() =>
      expect(screen.getByText("權限不足")).toBeInTheDocument(),
    );
    expect(
      screen.getAllByRole("heading", { name: "現場驗票" }).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getByRole("link", { name: "返回預設頁面" }),
    ).toBeInTheDocument();
  });

  it("shows single sign-on required state when provider auth fails and local demo is disabled", async () => {
    mockMe.mockRejectedValueOnce(new Error("authentication required"));
    mockAuthBootstrap.mockResolvedValueOnce({
      mock_profiles_enabled: false,
      mock_profiles: [],
      debug_chrome_enabled: false,
    });
    mockReadiness.mockResolvedValue({});

    render(<App />);

    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "需要企業單一登入身分" }),
      ).toBeInTheDocument(),
    );
    expect(screen.queryByLabelText("本機身分清單")).not.toBeInTheDocument();
  });

  it("shows mock profile selector when bootstrap allows it", async () => {
    mockMe.mockRejectedValueOnce(new Error("authentication required"));
    mockAuthBootstrap.mockResolvedValueOnce({
      mock_profiles_enabled: true,
      mock_profiles: [
        {
          profile_id: "E1001",
          display_name: "Ariel Chen",
          role_claims: ["employee"],
          mapped_roles: ["employee"],
          department: "Engineering",
          site: "Taipei HQ",
          city: "Taipei",
          grade: 6,
          employment_status: "active",
        },
      ],
      debug_chrome_enabled: true,
    });
    mockReadiness.mockResolvedValue({});

    render(<App />);

    await waitFor(() =>
      expect(screen.getByLabelText("本機身分清單")).toBeInTheDocument(),
    );
    expect(
      screen.getByRole("heading", { name: "選擇一個本機身分" }),
    ).toBeInTheDocument();
  });

  it("selects local mock profiles into the employee app chrome", async () => {
    const session: AuthSession = {
      actor: {
        id: "E1001",
        role: "employee",
      },
      expires_at: "2026-05-06T10:00:00Z",
      claims: {
        employee_id: "E1001",
        display_name: "Ariel Chen",
        role_claims: ["employee"],
        mapped_roles: ["employee"],
        department: "Engineering",
        site: "Taipei HQ",
        city: "Taipei",
        grade: 6,
        employment_status: "active",
        claims_status: "complete",
      },
      source: "mock",
    };

    mockMe.mockRejectedValueOnce(new Error("authentication required"));
    mockAuthBootstrap.mockResolvedValueOnce({
      mock_profiles_enabled: true,
      mock_profiles: [
        {
          profile_id: "E1001",
          display_name: "Ariel Chen",
          role_claims: ["employee"],
          mapped_roles: ["employee"],
          department: "Engineering",
          site: "Taipei HQ",
          city: "Taipei",
          grade: 6,
          employment_status: "active",
        },
      ],
      debug_chrome_enabled: true,
    });
    mockReadiness.mockResolvedValue({});
    mockSelectMockProfile.mockResolvedValueOnce(session);

    render(<App />);

    await userEvent.click(
      await screen.findByRole("button", { name: /Ariel Chen/ }),
    );

    await waitFor(() =>
      expect(mockSelectMockProfile).toHaveBeenCalledWith("E1001"),
    );
    expect(
      await screen.findAllByRole("heading", { name: "活動探索" }),
    ).not.toHaveLength(0);
    expect(screen.getByLabelText("活動行事曆")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Debug/ })).toBeInTheDocument();
    expect(document.body).not.toHaveTextContent("E1001");
    expect(mockClearProviderToken).not.toHaveBeenCalled();
    expect(globalThis.location.pathname).toBe("/user/events");
  });

  it("shows ops navigation only when the ops API is registered", async () => {
    const session: AuthSession = {
      actor: {
        id: "system-1",
        role: "system_admin",
      },
      expires_at: "2026-05-31T08:00:00Z",
      claims: {
        employee_id: "system-1",
        display_name: "System One",
        role_claims: ["system_admin"],
        mapped_roles: ["system_admin"],
        department: "IT",
        site: "Taipei HQ",
        city: "Taipei",
        grade: 8,
        employment_status: "active",
        claims_status: "complete",
      },
      source: "provider",
    };
    mockMe.mockResolvedValueOnce(session);
    mockAuthBootstrap.mockResolvedValueOnce({
      mock_profiles_enabled: false,
      mock_profiles: [],
      debug_chrome_enabled: false,
      ops_api_enabled: true,
    });
    mockReadiness.mockResolvedValue({});
    globalThis.history.pushState({}, "", "/admin/reports");

    render(<App />);

    await waitFor(() =>
      expect(screen.getAllByRole("link", { name: /營運監控/ }).length).toBe(1),
    );
  });

  it("surfaces bootstrap failures for signed-in users", async () => {
    const session: AuthSession = {
      actor: {
        id: "admin-1",
        role: "activity_admin",
      },
      expires_at: "2026-05-31T08:00:00Z",
      claims: {
        employee_id: "admin-1",
        display_name: "Admin One",
        role_claims: ["activity_admin"],
        mapped_roles: ["activity_admin"],
        department: "Welfare Committee",
        site: "Taipei HQ",
        city: "Taipei",
        grade: 7,
        employment_status: "active",
        claims_status: "complete",
      },
      source: "provider",
    };
    mockMe.mockResolvedValueOnce(session);
    mockAuthBootstrap.mockRejectedValueOnce(new Error("bootstrap unavailable"));
    mockReadiness.mockResolvedValue({});
    globalThis.history.pushState({}, "", "/admin/demo");

    render(<App />);

    await waitFor(() =>
      expect(screen.getByText("權限不足")).toBeInTheDocument(),
    );
    expect(screen.getByText("bootstrap unavailable")).toBeInTheDocument();
  });

  it("keeps admin demo available for provider sessions during restoration", async () => {
    const session: AuthSession = {
      actor: {
        id: "external-admin",
        role: "activity_admin",
      },
      expires_at: "2026-05-31T08:00:00Z",
      claims: {
        employee_id: "external-admin",
        display_name: "External Admin",
        role_claims: ["activity_admin"],
        mapped_roles: ["activity_admin"],
        department: "Welfare Committee",
        site: "Taipei HQ",
        city: "Taipei",
        grade: 7,
        employment_status: "active",
        claims_status: "complete",
      },
      source: "provider",
    };
    mockMe.mockResolvedValueOnce(session);
    mockAuthBootstrap.mockResolvedValueOnce({
      mock_profiles_enabled: true,
      mock_profiles: [
        {
          profile_id: "admin-1",
          display_name: "Admin One",
          role_claims: ["activity_admin"],
          mapped_roles: ["activity_admin"],
          department: "Welfare Committee",
          site: "Taipei HQ",
          city: "Taipei",
          grade: 7,
          employment_status: "active",
        },
      ],
      debug_chrome_enabled: true,
      demo_debug_enabled: true,
    });
    mockReadiness.mockResolvedValue({});
    globalThis.history.pushState({}, "", "/admin/demo");

    render(<App />);

    const restore = await screen.findByRole("button", {
      name: "restore demo session",
    });
    await userEvent.click(restore);

    expect(
      screen.getByRole("button", { name: "restore demo session" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("權限不足")).not.toBeInTheDocument();
  });

  it("blocks ops deep links when the ops API is not registered", async () => {
    const session: AuthSession = {
      actor: {
        id: "system-1",
        role: "system_admin",
      },
      expires_at: "2026-05-31T08:00:00Z",
      claims: {
        employee_id: "system-1",
        display_name: "System One",
        role_claims: ["system_admin"],
        mapped_roles: ["system_admin"],
        department: "IT",
        site: "Taipei HQ",
        city: "Taipei",
        grade: 8,
        employment_status: "active",
        claims_status: "complete",
      },
      source: "provider",
    };
    mockMe.mockResolvedValueOnce(session);
    mockAuthBootstrap.mockResolvedValueOnce({
      mock_profiles_enabled: false,
      mock_profiles: [],
      debug_chrome_enabled: false,
      ops_api_enabled: false,
    });
    mockReadiness.mockResolvedValue({});
    globalThis.history.pushState({}, "", "/admin/ops");

    render(<App />);

    await waitFor(() =>
      expect(screen.getByText("權限不足")).toBeInTheDocument(),
    );
    expect(mockGetOpsDashboard).not.toHaveBeenCalled();
  });
});
