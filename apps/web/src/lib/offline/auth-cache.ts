import type { AuthSession } from "@/lib/api";

const AUTH_KEY = "cets-auth-session";
const PROVIDER_TOKEN_KEY = "cets-provider-token";

type CachedProviderToken = {
  provider_token: string;
  expires_at: string;
};

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

// Persist the provider token so a PWA restart / offline reload can still
// authenticate the reconnect sync. Stored with its expiry; reads drop it once
// expired. The token is a mock provider token in this demo and is redacted in
// API logs (see lib/api/redaction.ts).
export function cacheProviderToken(token: string, expiresAt: string): void {
  if (!token) return;
  try {
    const entry: CachedProviderToken = {
      provider_token: token,
      expires_at: expiresAt,
    };
    localStorage.setItem(PROVIDER_TOKEN_KEY, JSON.stringify(entry));
  } catch {
    /* quota exceeded or private mode — skip silently */
  }
}

export function loadCachedProviderToken(now: Date = new Date()): string | null {
  try {
    const raw = localStorage.getItem(PROVIDER_TOKEN_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as CachedProviderToken;
    if (!parsed?.provider_token) return null;
    if (parsed.expires_at) {
      const expiry = new Date(parsed.expires_at);
      if (!Number.isNaN(expiry.getTime()) && expiry <= now) {
        clearCachedProviderToken();
        return null;
      }
    }
    return parsed.provider_token;
  } catch {
    return null;
  }
}

export function clearCachedProviderToken(): void {
  try {
    localStorage.removeItem(PROVIDER_TOKEN_KEY);
  } catch {
    /* ignore */
  }
}

export function isOffline(): boolean {
  return !navigator.onLine;
}
