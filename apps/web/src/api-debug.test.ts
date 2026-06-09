import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  getDemoClock,
  setProviderToken,
  setProviderTokenProvider,
  updateDemoClock,
} from "@/lib/api";

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function parseRequestBody(body: BodyInit | null | undefined) {
  if (body === undefined || body === null) return undefined;
  if (typeof body !== "string") {
    throw new TypeError("expected request body to be a JSON string");
  }
  return JSON.parse(body) as unknown;
}

describe("debug API client", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    setProviderToken(null);
    setProviderTokenProvider(null);
    vi.unstubAllGlobals();
  });

  function mockSuccess(data: unknown = {}) {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ success: true, data, error: null }),
    );
  }

  function fetchCall(index: number) {
    const [path, init] = fetchMock.mock.calls[index] as [
      string,
      RequestInit | undefined,
    ];
    const body = parseRequestBody(init?.body);
    return { path, init, body };
  }

  it("calls demo debug clock endpoints", async () => {
    mockSuccess({
      enabled: true,
      mode: "real",
      now: "2026-05-31T08:00:00Z",
      real_now: "2026-05-31T08:00:00Z",
    });
    await getDemoClock();

    mockSuccess({
      enabled: true,
      mode: "fixed",
      now: "2026-06-01T01:30:00Z",
      real_now: "2026-05-31T08:00:00Z",
    });
    await updateDemoClock({
      mode: "fixed",
      now: "2026-06-01T01:30:00Z",
      reason: "cutoff demo",
    });

    expect(fetchCall(0).path).toBe("/api/v1/debug/demo-clock");
    expect(fetchCall(1)).toMatchObject({
      path: "/api/v1/debug/demo-clock",
      init: { method: "PUT" },
      body: {
        mode: "fixed",
        now: "2026-06-01T01:30:00Z",
        reason: "cutoff demo",
      },
    });
  });
});
