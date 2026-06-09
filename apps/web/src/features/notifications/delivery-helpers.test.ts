import { describe, expect, it } from "vitest";
import type { NotificationDelivery } from "@/lib/api";
import {
  canRetryDelivery,
  sortDeliveriesByAttention,
} from "./delivery-helpers";

describe("notification delivery helpers", () => {
  it("keeps retry eligibility limited to email failures", () => {
    expect(canRetryDelivery(delivery({ status: "failed" }))).toBe(true);
    expect(
      canRetryDelivery(delivery({ channel: "in_app", status: "failed" })),
    ).toBe(false);
    expect(canRetryDelivery(delivery({ status: "pending" }))).toBe(false);
  });

  it("sorts retryable failures and pending work before completed rows", () => {
    const rows = sortDeliveriesByAttention([
      delivery({
        delivery_id: "sent",
        status: "sent",
        updated_at: "2026-01-04T00:00:00Z",
      }),
      delivery({
        delivery_id: "pending",
        status: "pending",
        updated_at: "2026-01-03T00:00:00Z",
      }),
      delivery({
        delivery_id: "failed",
        status: "failed",
        updated_at: "2026-01-01T00:00:00Z",
      }),
      delivery({
        delivery_id: "dead",
        status: "dead_letter",
        updated_at: "2026-01-02T00:00:00Z",
      }),
    ]);

    expect(rows.map((row) => row.delivery_id)).toEqual([
      "dead",
      "failed",
      "pending",
      "sent",
    ]);
  });
});

function delivery(
  overrides: Partial<NotificationDelivery> = {},
): NotificationDelivery {
  return {
    delivery_id: "delivery",
    outbox_id: "outbox",
    employee_ref: "E100****",
    channel: "email",
    status: "sent",
    attempts: 1,
    last_error: "",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}
