import type { AuthSession } from "@/lib/api";

const AUTH_KEY = "cets-auth-session";

export function cacheAuthSession(session: AuthSession): void {
  try {
    localStorage.setItem(AUTH_KEY, JSON.stringify(session));
  } catch {
    /* quota exceeded or private mode — skip silently */
  }
}

export function loadCachedAuthSession(): AuthSession | null {
  try {
    const raw = localStorage.getItem(AUTH_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as AuthSession;
    if (!parsed?.actor?.id || !parsed?.actor?.role) return null;
    return parsed;
  } catch {
    return null;
  }
}

export function clearCachedAuthSession(): void {
  try {
    localStorage.removeItem(AUTH_KEY);
  } catch {
    /* ignore */
  }
}

export function isOffline(): boolean {
  return !navigator.onLine;
}
