import { beforeEach, describe, expect, it } from "vitest";
import {
  debugChromePath,
  isDebugChromeEnabled,
  setDebugChromeQuery,
} from "./debug";

describe("debug chrome helpers", () => {
  beforeEach(() => {
    window.history.replaceState({}, "", "/user/events");
  });

  it("keeps debug chrome disabled without the debug query", () => {
    expect(isDebugChromeEnabled()).toBe(false);
  });

  it("toggles debug chrome through the URL query", () => {
    setDebugChromeQuery(true);

    expect(window.location.search).toBe("?debug=1");
    expect(isDebugChromeEnabled()).toBe(true);

    setDebugChromeQuery(false);
    expect(window.location.search).toBe("");
  });

  it("preserves debug mode across internal navigation paths", () => {
    setDebugChromeQuery(true);

    expect(debugChromePath("/user/tickets")).toBe("/user/tickets?debug=1");
    expect(debugChromePath("/user/events/detail?event_id=evt-1")).toBe(
      "/user/events/detail?event_id=evt-1&debug=1",
    );
  });
});
