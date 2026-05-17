import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AuthSession } from "@/lib/api";
import App from "./App";
import { authBootstrap, me, readiness } from "@/lib/api";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    authBootstrap: vi.fn(),
    me: vi.fn(),
    readiness: vi.fn(),
    setApiObserver: vi.fn(),
  };
});

const mockMe = vi.mocked(me);
const mockAuthBootstrap = vi.mocked(authBootstrap);
const mockReadiness = vi.mocked(readiness);

describe("App", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.history.pushState({}, "", "/");
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

    window.history.pushState({}, "", "/admin/events");

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
});
