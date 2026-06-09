import type { EventSummary } from "@/lib/api";
import { selectCurrentEventID } from "@/lib/current-event";

const duplicateScanWindowMs = 1500;

export type RecentScan = {
  token: string;
  scannedAtMs: number;
};

export function selectedCheckinEventID(
  events: EventSummary[],
  current: string,
  now: Date = new Date(),
) {
  return selectCurrentEventID(events, current, now);
}

export function isCheckinReady({
  deviceID,
  eventID,
  token,
}: Readonly<{
  deviceID: string;
  eventID: string;
  token: string;
}>) {
  return (
    token.trim().length > 0 &&
    deviceID.trim().length > 0 &&
    eventID.trim().length > 0
  );
}

export function shouldSubmitDetectedToken({
  detectedToken,
  lastScan,
  nowMs,
}: Readonly<{
  detectedToken: string;
  lastScan: RecentScan | null;
  nowMs: number;
}>) {
  const token = detectedToken.trim();
  if (!token) return false;
  if (!lastScan) return true;
  return (
    lastScan.token !== token ||
    nowMs - lastScan.scannedAtMs > duplicateScanWindowMs
  );
}
