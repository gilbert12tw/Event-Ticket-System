import { describe, expect, it } from "vitest";
import type { AuditLog } from "@/lib/api";
import { auditTargetPath, sortAuditRowsByAttention } from "./audit-priority";

describe("audit priority", () => {
  it("puts conflict and failed audit rows first", () => {
    const rows = sortAuditRowsByAttention([
      audit({ audit_id: "normal", action: "event.updated" }),
      audit({
        audit_id: "ticket",
        action: "ticket.redeemed",
        entity_type: "ticket",
      }),
      audit({ audit_id: "fail", action: "notification.failed" }),
      audit({ audit_id: "conflict", action: "checkin.conflict" }),
    ]);

    expect(rows.map((row) => row.audit_id)).toEqual([
      "conflict",
      "fail",
      "ticket",
      "normal",
    ]);
  });

  it("maps audit entities to focused admin workspaces", () => {
    expect(
      auditTargetPath(audit({ entity_type: "event", entity_id: "evt-1" })),
    ).toBe("/admin/events/evt-1/edit");
    expect(
      auditTargetPath(audit({ entity_type: "ticket", entity_id: "tkt-1" })),
    ).toBe("/admin/checkin");
  });
});

function audit(overrides: Partial<AuditLog> = {}): AuditLog {
  return {
    audit_id: "aud-1",
    actor_id: "admin-1",
    role: "activity_admin",
    action: "event.updated",
    entity_type: "event",
    entity_id: "evt-1",
    metadata: "{}",
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}
