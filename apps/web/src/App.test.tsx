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
    setApiObserver: vi.fn()
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
        role: "checkin_staff"
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
        claims_status: "complete"
      },
      source: "provider"
    };

    mockMe.mockResolvedValueOnce(session);
    mockAuthBootstrap.mockResolvedValue({ mock_profiles_enabled: false, mock_profiles: [] });
    mockReadiness.mockResolvedValue({});

    window.history.pushState({}, "", "/admin/events");

    render(<App />);

    await waitFor(() => expect(screen.getByText("權限不足")).toBeInTheDocument());
    expect(screen.getByRole("heading", { name: "現場驗票" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "返回預設頁面" })).toBeInTheDocument();
  });

  it("shows SSO required state when provider auth fails and local demo is disabled", async () => {
    mockMe.mockRejectedValueOnce(new Error("authentication required"));
    mockAuthBootstrap.mockResolvedValueOnce({ mock_profiles_enabled: false, mock_profiles: [] });
    mockReadiness.mockResolvedValue({});

    render(<App />);

    await waitFor(() => expect(screen.getByRole("heading", { name: "需要企業 SSO 身分" })).toBeInTheDocument());
    expect(screen.queryByLabelText("Mock provider profiles")).not.toBeInTheDocument();
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
          city: "Taipei"
        }
      ]
    });
    mockReadiness.mockResolvedValue({});

    render(<App />);

    await waitFor(() => expect(screen.getByLabelText("Mock provider profiles")).toBeInTheDocument());
    expect(screen.getByRole("heading", { name: "選擇一個模擬 provider profile" })).toBeInTheDocument();
  });
});
