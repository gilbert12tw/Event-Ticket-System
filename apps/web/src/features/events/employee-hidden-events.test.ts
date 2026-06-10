import { beforeEach, describe, expect, it } from "vitest";
import {
  loadHiddenEventIds,
  saveHiddenEventIds,
} from "./employee-hidden-events";

describe("employee-hidden-events", () => {
  beforeEach(() => localStorage.clear());

  it("round-trips hidden ids per principal", () => {
    saveHiddenEventIds("emp-1", ["evt-a", "evt-b"]);
    expect(loadHiddenEventIds("emp-1")).toEqual(["evt-a", "evt-b"]);
    expect(loadHiddenEventIds("emp-2")).toEqual([]);
  });

  it("returns empty list for blank principal or missing entry", () => {
    expect(loadHiddenEventIds("")).toEqual([]);
    expect(loadHiddenEventIds("nobody")).toEqual([]);
    saveHiddenEventIds("", ["evt-a"]);
    expect(loadHiddenEventIds("")).toEqual([]);
  });

  it("ignores malformed stored payloads", () => {
    localStorage.setItem("cets-hidden-events:emp-3", "{not-json");
    expect(loadHiddenEventIds("emp-3")).toEqual([]);
    localStorage.setItem("cets-hidden-events:emp-4", JSON.stringify({}));
    expect(loadHiddenEventIds("emp-4")).toEqual([]);
    localStorage.setItem(
      "cets-hidden-events:emp-5",
      JSON.stringify(["ok", 42, null]),
    );
    expect(loadHiddenEventIds("emp-5")).toEqual(["ok"]);
  });
});
