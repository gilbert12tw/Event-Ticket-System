import { describe, expect, it } from "vitest";
import messages from "./messages-data.json";
import options from "./options-data.json";
import {
  deliveryStatusView,
  eventStatusView,
  localizedMessage,
  roleViewLabel,
  ticketStatusView,
} from "./options";

const optionGroups = [
  "capacityTypes",
  "eligibilityDepartments",
  "eligibilitySites",
  "employmentStatusFilters",
  "entryMethods",
  "visibilities",
  "categories",
  "eventTemplates",
  "schedulePresets",
  "venues",
  "devicePresets",
  "tagSuggestions",
  "roles",
  "entities",
  "departments",
  "sites",
  "employmentStatuses",
  "channels",
  "auditActions",
  "auditLimits",
  "hrReviewStatuses",
  "cancellationReasons",
  "revocationReasons",
  "resolutionReasons",
  "reportPresets",
] as const;
const viewGroups = [
  "eventStatuses",
  "registrationStatuses",
  "ticketStatuses",
  "deliveryStatuses",
  "reviewStatuses",
  "reportExportStatuses",
  "checkinStatuses",
  "auditActionViews",
] as const;
const tones = new Set(["ok", "warn", "fail", "info", "neutral"]);
const optionData = options as unknown as Record<string, readonly unknown[][]>;
const messageData = messages as Record<string, string>;

describe("ui data contracts", () => {
  it("keeps option and status groups available to options.ts", () => {
    expect(Object.keys(optionData).sort()).toEqual(
      [...optionGroups, ...viewGroups].sort(),
    );
  });

  it("keeps option rows in the expected tuple shape", () => {
    for (const group of optionGroups) {
      expect(optionData[group]?.length, group).toBeGreaterThan(0);
      for (const row of optionData[group]) {
        expect(row.length, group).toBeGreaterThanOrEqual(2);
        expect(row.length, group).toBeLessThanOrEqual(3);
        expect(typeof row[0], group).toBe("string");
        expect(typeof row[1], group).toBe("string");
        if (row[2] !== undefined) expect(typeof row[2], group).toBe("string");
      }
    }
  });

  it("keeps status rows in the expected tuple shape and tone set", () => {
    for (const group of viewGroups) {
      expect(optionData[group]?.length, group).toBeGreaterThan(0);
      for (const row of optionData[group]) {
        expect(row.length, group).toBeGreaterThanOrEqual(3);
        expect(row.length, group).toBeLessThanOrEqual(4);
        expect(typeof row[0], group).toBe("string");
        expect(typeof row[1], group).toBe("string");
        expect(tones.has(String(row[2])), group).toBe(true);
        if (row[3] !== undefined) expect(typeof row[3], group).toBe("string");
      }
    }
    expect(eventStatusView("published")).toEqual({
      label: "已發布",
      tone: "ok",
    });
    expect(ticketStatusView("revoked")).toEqual({
      label: "已撤銷",
      tone: "fail",
    });
    expect(deliveryStatusView("dead_letter")).toEqual({
      label: "投遞終止",
      tone: "fail",
    });
    expect(roleViewLabel("system_admin")).toBe("系統管理員");
  });

  it("keeps known backend messages localized", () => {
    for (const [source, localized] of [
      ["ticket has already been redeemed", "此票券已核銷，不能重複入場。"],
      ["offline check-in batch event mismatch", "離線批次的活動不一致。"],
      ["invalid offline package signature", "離線名單簽章無效。"],
    ] as const) {
      expect(messageData[source]).toBe(localized);
      expect(localizedMessage(source)).toBe(localized);
    }
  });
});
