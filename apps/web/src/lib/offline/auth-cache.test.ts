import { beforeEach, describe, expect, it } from "vitest";
import {
  cacheAuthSession,
  cacheProviderToken,
  clearCachedAuthSession,
  clearCachedProviderToken,
  isOffline,
  loadCachedAuthSession,
  loadCachedProviderToken,
} from "./auth-cache";
import type { AuthSession } from "@/lib/api";

function fakeSession(overrides: Partial<AuthSession> = {}): AuthSession {
  return {
    actor: { id: "emp-1", role: "employee" },
    expires_at: "2026-12-31T00:00:00Z",
    claims: {
      employee_id: "emp-1",
      display_name: "Alice",
      role_claims: [],
      mapped_roles: ["employee"],
      department: "Eng",
      site: "HQ",
      city: "Taipei",
      grade: 5,
      employment_status: "active",
      claims_status: "complete",
    },
    source: "provider",
    ...overrides,
  };
}

describe("auth-cache", () => {
  beforeEach(() => localStorage.clear());

  it("round-trips a session through cache", () => {
    const session = fakeSession();
    cacheAuthSession(session);
    expect(loadCachedAuthSession()).toEqual(session);
  });

  it("returns null when nothing cached", () => {
    expect(loadCachedAuthSession()).toBeNull();
  });

  it("returns null for corrupt data", () => {
    localStorage.setItem("cets-auth-session", "not-json");
    expect(loadCachedAuthSession()).toBeNull();
  });

  it("returns null when actor is missing", () => {
    localStorage.setItem("cets-auth-session", JSON.stringify({ foo: 1 }));
    expect(loadCachedAuthSession()).toBeNull();
  });

  it("clears cached session", () => {
    cacheAuthSession(fakeSession());
    clearCachedAuthSession();
    expect(loadCachedAuthSession()).toBeNull();
  });

  it("round-trips a provider token that has not expired", () => {
    cacheProviderToken("tok-123", "2999-01-01T00:00:00Z");
    expect(loadCachedProviderToken()).toBe("tok-123");
  });

  it("returns null for an expired provider token and clears it", () => {
    cacheProviderToken("tok-expired", "2000-01-01T00:00:00Z");
    expect(loadCachedProviderToken()).toBeNull();
    expect(localStorage.getItem("cets-provider-token")).toBeNull();
  });

  it("returns null when no provider token cached", () => {
    expect(loadCachedProviderToken()).toBeNull();
  });

  it("ignores an empty provider token", () => {
    cacheProviderToken("", "2999-01-01T00:00:00Z");
    expect(loadCachedProviderToken()).toBeNull();
  });

  it("keeps a token with no expiry and clears on demand", () => {
    cacheProviderToken("tok-no-exp", "");
    expect(loadCachedProviderToken()).toBe("tok-no-exp");
    clearCachedProviderToken();
    expect(loadCachedProviderToken()).toBeNull();
  });

  it("isOffline reflects navigator.onLine", () => {
    Object.defineProperty(navigator, "onLine", {
      value: false,
      writable: true,
      configurable: true,
    });
    expect(isOffline()).toBe(true);
    Object.defineProperty(navigator, "onLine", {
      value: true,
      writable: true,
      configurable: true,
    });
    expect(isOffline()).toBe(false);
  });
});
