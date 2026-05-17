import userEvent from "@testing-library/user-event";
import { render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { auditLogs } from "@/lib/api";
import { AdminAuditPage } from "./pages";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    auditLogs: vi.fn(),
  };
});

describe("AdminAuditPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.history.replaceState({}, "", "/admin/audit");
  });

  it("selects audit rows directly without desktop row action buttons", async () => {
    vi.mocked(auditLogs).mockResolvedValue([
      {
        audit_id: "aud-1",
        actor_id: "staff-1",
        role: "checkin_staff",
        action: "checkin.conflict",
        entity_type: "ticket",
        entity_id: "tkt-1",
        metadata: JSON.stringify({ event_id: "evt-1" }),
        created_at: "2026-05-16T10:32:00Z",
      },
      {
        audit_id: "aud-2",
        actor_id: "staff-2",
        role: "checkin_staff",
        action: "ticket.redeemed",
        entity_type: "ticket",
        entity_id: "tkt-2",
        metadata: JSON.stringify({ event_id: "evt-2" }),
        created_at: "2026-05-16T10:34:00Z",
      },
    ]);

    const { container } = render(<AdminAuditPage />);

    const table = await screen.findByRole("table", { name: "稽核紀錄" });
    expect(
      within(table).queryByRole("button", { name: "檢視" }),
    ).not.toBeInTheDocument();
    expect(container.querySelectorAll(".audit-mobile-card")).toHaveLength(2);
    expect(screen.getByText("aud-1")).toBeInTheDocument();

    const secondRow = within(table).getByText("staff-2").closest("tr");
    expect(secondRow).not.toBeNull();
    await userEvent.click(secondRow as HTMLTableRowElement);

    await waitFor(() => expect(screen.getByText("aud-2")).toBeInTheDocument());
  });

  it("keeps audit cursor paging available when a preset page has no visible rows", async () => {
    window.history.replaceState({}, "", "/admin/audit?tab=ticket");
    vi.mocked(auditLogs).mockResolvedValue([
      {
        audit_id: "aud-event-only",
        actor_id: "admin-1",
        role: "activity_admin",
        action: "event.updated",
        entity_type: "event",
        entity_id: "evt-1",
        metadata: JSON.stringify({ event_id: "evt-1" }),
        created_at: "2026-05-16T10:32:00Z",
      },
    ]);

    render(<AdminAuditPage />);

    expect(await screen.findAllByText("沒有稽核紀錄")).not.toHaveLength(0);
    await userEvent.click(screen.getByRole("button", { name: "下一頁" }));

    await waitFor(() =>
      expect(vi.mocked(auditLogs)).toHaveBeenLastCalledWith(
        expect.objectContaining({
          cursor: "2026-05-16T10:32:00Z|aud-event-only",
        }),
      ),
    );
  });
});
