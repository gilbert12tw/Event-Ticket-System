import userEvent from "@testing-library/user-event";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ApiLogEntry } from "@/lib/api";
import { ApiActivity, WorkspaceSwitch } from ".";

describe("layout components", () => {
  it("hides the workspace switch when the role has only one workspace", () => {
    const { rerender } = render(
      <WorkspaceSwitch active="user" role="employee" />,
    );

    expect(screen.queryByLabelText("切換工作區")).not.toBeInTheDocument();

    rerender(<WorkspaceSwitch active="admin" role="activity_admin" />);

    expect(screen.queryByLabelText("切換工作區")).not.toBeInTheDocument();
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
