import userEvent from "@testing-library/user-event";
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ApiLogEntry, AuthSession } from "@/lib/api";
import { routes } from "@/app/routes";
import { ApiActivity, WorkspaceSwitch } from ".";
import { AuthenticatedShell } from "./shell";

describe("layout components", () => {
  it("keeps desktop debug tools in one header entry", () => {
    const activeRoute = routes.find((route) => route.key === "admin-reports");
    const session: AuthSession = {
      actor: {
        id: "hr-1",
        role: "hr_admin",
      },
      claims: {
        employee_id: "hr-1",
        display_name: "HR One",
        role_claims: ["hr_admin"],
        mapped_roles: ["hr_admin"],
        department: "Human Resources",
        site: "Taipei HQ",
        city: "Taipei",
        grade: 6,
        employment_status: "active",
        claims_status: "complete",
      },
      expires_at: "2026-05-17T12:00:00Z",
      source: "provider",
    };

    if (!activeRoute) throw new Error("missing admin reports route");

    render(
      <AuthenticatedShell
        activeRoute={activeRoute}
        activeWorkspace="admin"
        apiLog={[]}
        debugChromeAvailable
        debugChromeEnabled
        health="ok"
        mockProfilesEnabled
        navRoutes={[activeRoute]}
        onClearApiLog={vi.fn()}
        onSwitchProfile={vi.fn()}
        onToggleDebugChrome={vi.fn()}
        ready="ok"
        safeRoute="admin-reports"
        session={session}
      >
        <div>Report content</div>
      </AuthenticatedShell>,
    );

    expect(screen.getAllByRole("button", { name: "Debug" })).toHaveLength(1);
    expect(
      within(screen.getByLabelText("主要導覽")).queryByRole("button", {
        name: "Debug",
      }),
    ).not.toBeInTheDocument();
  });

  it("hides the workspace switch when the role has only one workspace", () => {
    const { rerender } = render(
      <WorkspaceSwitch active="user" role="employee" />,
    );

    expect(screen.queryByLabelText("切換工作區")).not.toBeInTheDocument();

    rerender(<WorkspaceSwitch active="admin" role="activity_admin" />);

    expect(screen.queryByLabelText("切換工作區")).not.toBeInTheDocument();
  });

  it("keeps employee workspace debug available without exposing employee IDs", () => {
    const activeRoute = routes.find((route) => route.key === "user-events");
    const session: AuthSession = {
      actor: {
        id: "E1001",
        role: "employee",
      },
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
      expires_at: "2026-05-17T12:00:00Z",
      source: "provider",
    };

    if (!activeRoute) throw new Error("missing user events route");

    const { container } = render(
      <AuthenticatedShell
        activeRoute={activeRoute}
        activeWorkspace="user"
        apiLog={[]}
        debugChromeAvailable
        debugChromeEnabled
        health="ok"
        mockProfilesEnabled
        navRoutes={[activeRoute]}
        onClearApiLog={vi.fn()}
        onSwitchProfile={vi.fn()}
        onToggleDebugChrome={vi.fn()}
        ready="ok"
        safeRoute="user-events"
        session={session}
      >
        <div>Events content</div>
      </AuthenticatedShell>,
    );

    expect(screen.getByRole("button", { name: "Debug" })).toBeInTheDocument();
    expect(
      screen.getAllByRole("heading", { level: 1, name: "活動探索" }),
    ).toHaveLength(1);
    expect(container).not.toHaveTextContent("E1001");
    expect(container).toHaveTextContent("員工");
  });

  it("shows request and response bodies when an API activity entry expands", async () => {
    const entry: ApiLogEntry = {
      id: "log-1",
      label: "GET /api/v1/events",
      status: 200,
      ok: true,
      requestBody: null,
      responseBody: {
        success: true,
        data: [{ event_id: "evt-1", title: "家庭電影夜" }],
        error: null,
      },
      createdAt: "2026-05-16T10:00:00Z",
    };

    render(<ApiActivity entries={[entry]} onClear={vi.fn()} />);

    await userEvent.click(screen.getByRole("button", { name: "顯示 1" }));
    const summary = screen.getByText("GET /api/v1/events").closest("summary");
    const details = summary?.closest("details");
    expect(summary).not.toBeNull();
    expect(details).not.toHaveAttribute("open");

    await userEvent.click(summary as HTMLElement);

    expect(details).toHaveAttribute("open");
    expect(
      screen.getByLabelText("GET /api/v1/events request body"),
    ).toHaveTextContent("null");
    expect(
      screen.getByLabelText("GET /api/v1/events response body"),
    ).toHaveTextContent('"title": "家庭電影夜"');
  });
});
