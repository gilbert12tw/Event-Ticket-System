const CHECKIN_HISTORY_TOKEN_KEY = "cetsCheckinToken";
const DEMO_CHECKIN_TOKEN_KEY = "cets:lastTicketToken";
const DEMO_CHECKIN_MODE_KEY = "cets:lastTicketToken:demo-mode";

export type CheckinHistoryState = {
  [CHECKIN_HISTORY_TOKEN_KEY]: string;
};

export function checkinHistoryState(token: string): CheckinHistoryState {
  return { [CHECKIN_HISTORY_TOKEN_KEY]: token };
}

export function readHistoryCheckinToken(state: unknown = window.history.state): string {
  const payload = (state && typeof state === "object" && !Array.isArray(state) ? (state as Record<string, unknown>) : null);
  if (!payload) {
    return "";
  }
  const token = payload[CHECKIN_HISTORY_TOKEN_KEY];
  return typeof token === "string" ? token : "";
}

export function setDemoCheckinToken(token: string) {
  if (typeof window === "undefined") {
    return;
  }
  if (!token) {
    return;
  }
  window.localStorage.setItem(DEMO_CHECKIN_TOKEN_KEY, token);
  window.localStorage.setItem(DEMO_CHECKIN_MODE_KEY, "enabled");
}

export function getDemoCheckinToken() {
  if (typeof window === "undefined") return "";
  if (window.localStorage.getItem(DEMO_CHECKIN_MODE_KEY) !== "enabled") {
    return "";
  }
  return window.localStorage.getItem(DEMO_CHECKIN_TOKEN_KEY) || "";
}

export function hasDemoCheckinToken() {
  return Boolean(getDemoCheckinToken());
}
