const CHECKIN_HISTORY_TOKEN_KEY = "cetsCheckinToken";

let demoCheckinToken = "";

export type CheckinHistoryState = {
  [CHECKIN_HISTORY_TOKEN_KEY]: string;
};

export function checkinHistoryState(token: string): CheckinHistoryState {
  return { [CHECKIN_HISTORY_TOKEN_KEY]: token };
}

export function readHistoryCheckinToken(
  state: unknown = window.history.state,
): string {
  const payload =
    state && typeof state === "object" && !Array.isArray(state)
      ? (state as Record<string, unknown>)
      : null;
  if (!payload) {
    return "";
  }
  const token = payload[CHECKIN_HISTORY_TOKEN_KEY];
  return typeof token === "string" ? token : "";
}

export function setDemoCheckinToken(token: string) {
  demoCheckinToken = token.trim();
}

export function getDemoCheckinToken() {
  return demoCheckinToken;
}

export function hasDemoCheckinToken() {
  return Boolean(getDemoCheckinToken());
}
