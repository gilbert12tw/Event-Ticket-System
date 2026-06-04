import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  captureProviderToken,
  downloadReportExport,
  eventPosterBlob,
  listEvents,
  restoreProviderToken,
  setApiObserver,
  setProviderToken,
  setProviderTokenProvider,
  uploadEventPoster,
} from "@/lib/api";

function response(body: unknown, init: ResponseInit = {}) {
  return new Response(typeof body === "string" ? body : JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

describe("api http helpers", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    setApiObserver(null);
    setProviderToken(null);
    setProviderTokenProvider(null);
    vi.unstubAllGlobals();
  });

  it("captures, restores, and falls back to provider token sources", async () => {
    setProviderTokenProvider(() => "provider-token");
    setProviderToken("memory-token");
    const snapshot = captureProviderToken();
    setProviderToken(null);
    restoreProviderToken(snapshot);
    fetchMock.mockResolvedValueOnce(
      response({ success: true, data: [], error: null }),
    );

    await listEvents();

    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      headers: expect.objectContaining({
        Authorization: "Bearer memory-token",
      }),
    });
  });

  it("logs network failures and throws non-API errors unchanged", async () => {
    const entries: Array<{ status: unknown; responseBody: unknown }> = [];
    setApiObserver((entry) => entries.push(entry));
    fetchMock.mockRejectedValueOnce(new Error("network down"));

    const caught = await listEvents().catch((error: unknown) => error);

    expect(caught).toBeInstanceOf(Error);
    expect(caught).not.toBeInstanceOf(ApiError);
    expect(entries[0]).toMatchObject({
      status: "ERR",
      responseBody: { error: "network down" },
    });
  });

  it("handles multipart errors from non-JSON responses", async () => {
    fetchMock.mockResolvedValueOnce(
      response("unsupported media", {
        status: 415,
        headers: { "Content-Type": "text/plain" },
      }),
    );

    const caught = await uploadEventPoster(
      "evt/1",
      new File(["poster"], "poster.png", { type: "image/png" }),
    ).catch((error: unknown) => error);

    expect(caught).toBeInstanceOf(ApiError);
    expect(caught).toMatchObject({
      status: 415,
      response: { error: "unsupported media" },
    });
  });

  it("returns null for missing posters and blobs for existing posters", async () => {
    fetchMock.mockResolvedValueOnce(response("missing", { status: 404 }));
    fetchMock.mockResolvedValueOnce(
      response("poster-bytes", {
        headers: { "Content-Type": "image/png" },
      }),
    );

    const missing = await eventPosterBlob("evt/1");
    const poster = await eventPosterBlob("evt/1");

    expect(missing).toBeNull();
    expect(poster).toBeInstanceOf(Blob);
    expect(await poster?.text()).toBe("poster-bytes");
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/events/evt%2F1/poster");
  });

  it("throws ApiError for non-JSON poster download errors", async () => {
    fetchMock.mockResolvedValueOnce(
      response("poster backend down", {
        status: 500,
        headers: { "Content-Type": "text/plain" },
      }),
    );

    const caught = await eventPosterBlob("evt/1").catch(
      (error: unknown) => error,
    );

    expect(caught).toBeInstanceOf(ApiError);
    expect(caught).toMatchObject({
      status: 500,
      response: { error: "poster backend down" },
    });
  });

  it("downloads report exports as blobs", async () => {
    fetchMock.mockResolvedValueOnce(
      response("csv-body", {
        headers: { "Content-Type": "text/csv" },
      }),
    );

    const report = await downloadReportExport("exp/1");

    expect(report).toBeInstanceOf(Blob);
    expect(await report.text()).toBe("csv-body");
    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/admin/reports/exports/exp%2F1/download",
    );
  });

  it("throws ApiError for non-JSON report download errors", async () => {
    fetchMock.mockResolvedValueOnce(
      response("not ready", {
        status: 409,
        headers: { "Content-Type": "text/plain" },
      }),
    );

    const caught = await downloadReportExport("exp/1").catch(
      (error: unknown) => error,
    );

    expect(caught).toBeInstanceOf(ApiError);
    expect(caught).toMatchObject({
      status: 409,
      response: { error: "not ready" },
    });
  });
});
