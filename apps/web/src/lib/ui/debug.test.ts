import { beforeEach, describe, expect, it } from "vitest";
import {
  debugChromePath,
  isDebugChromeEnabled,
  setDebugChromeQuery,
} from "./debug";

describe("debug chrome helpers", () => {
  beforeEach(() => {
    globalThis.history.replaceState({}, "", "/user/events");
  });

  it("keeps debug chrome disabled without the debug query when default is off", () => {
    expect(isDebugChromeEnabled(true, false)).toBe(false);
  });

  it("enables debug chrome by default for web dev when bootstrap allows it", () => {
    expect(isDebugChromeEnabled(true, true)).toBe(true);
    expect(isDebugChromeEnabled(false, true)).toBe(false);
  });

  it("toggles debug chrome through the URL query", () => {
    setDebugChromeQuery(true, false);

    expect(globalThis.location.search).toBe("?debug=1");
    expect(isDebugChromeEnabled(true, false)).toBe(true);
    expect(isDebugChromeEnabled(false, false)).toBe(false);

    setDebugChromeQuery(false, false);
    expect(globalThis.location.search).toBe("");
  });

  it("lets web dev users explicitly turn debug chrome off", () => {
    setDebugChromeQuery(false, true);

    expect(globalThis.location.search).toBe("?debug=0");
    expect(isDebugChromeEnabled(true, true)).toBe(false);

    setDebugChromeQuery(true, true);
    expect(globalThis.location.search).toBe("");
    expect(isDebugChromeEnabled(true, true)).toBe(true);
  });

  it("preserves debug mode across internal navigation paths", () => {
    setDebugChromeQuery(true, false);

    expect(debugChromePath("/user/tickets")).toBe("/user/tickets?debug=1");
    expect(debugChromePath("/user/events/detail?event_id=evt-1")).toBe(
      "/user/events/detail?event_id=evt-1&debug=1",
    );
  });

  it("preserves an explicit web dev debug disable across internal navigation paths", () => {
    setDebugChromeQuery(false, true);

    expect(debugChromePath("/user/tickets")).toBe("/user/tickets?debug=0");
  });
});
