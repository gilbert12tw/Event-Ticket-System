import { beforeEach, describe, expect, it } from "vitest";
import {
  cacheAuthSession,
  clearCachedAuthSession,
  isOffline,
  loadCachedAuthSession,
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
