import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { AuthSession } from "@/lib/api";
import App from "./App";
import { me, readiness } from "@/lib/api";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    login: vi.fn(),
    logout: vi.fn(),
    me: vi.fn(),
    readiness: vi.fn(),
    setApiObserver: vi.fn()
  };
});

const mockMe = vi.mocked(me);
const mockReadiness = vi.mocked(readiness);

describe("App", () => {
  it("renders unauthorized state when an invalid role enters a forbidden route", async () => {
    const session: AuthSession = {
      actor: {
        id: "staff-1",
        role: "checkin_staff"
      },
      expires_at: "2026-05-06T10:00:00Z"
    };

    mockMe.mockResolvedValueOnce(session);
    mockReadiness.mockResolvedValue({});

    window.history.pushState({}, "", "/admin/events");

    render(<App />);

    await waitFor(() => expect(screen.getByText("權限不足")).toBeInTheDocument());
    expect(screen.getByRole("heading", { name: "現場驗票" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "返回預設頁面" })).toBeInTheDocument();
  });
});
