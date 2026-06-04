import type { StepState } from "@/app/routes";
import type { RegistrationDetail } from "@/lib/api";

export const demoSteps = [
  ["seed", "載入 demo 員工", "POST /api/v1/admin/seed-demo"],
  ["event", "建立 lottery 活動", "POST /api/v1/admin/events"],
  ["book1", "E1001 送出報名", "POST /api/v1/events/{event_id}/bookings"],
  ["book2", "E1002 送出報名", "POST /api/v1/events/{event_id}/bookings"],
  [
    "received",
    "檢查 received 名單",
    "GET /api/v1/admin/events/{event_id}/registrations",
  ],
  ["cutoff", "快進到 cutoff 後", "PUT /api/v1/debug/demo-clock"],
  [
    "lottery",
    "執行 deterministic lottery",
    "POST /api/v1/admin/events/{event_id}/lottery-runs",
  ],
  ["ticket", "目前票券 QR", "GET /api/v1/me/tickets"],
  ["checkin", "掃描即驗票", "POST /api/v1/checkins"],
  ["duplicate", "重複掃描拒絕", "POST /api/v1/checkins"],
  ["report", "查報表與稽核", "GET /api/v1/admin/reports + audit-logs"],
] as const;

export type DemoStepID = (typeof demoSteps)[number][0];
export type StepMap = Record<DemoStepID, { state: StepState; hint: string }>;

export function initialSteps(): StepMap {
  return Object.fromEntries(
    demoSteps.map(([id]) => [
      id,
      { state: "pending" as StepState, hint: "待執行" },
    ]),
  ) as StepMap;
}

export function registrationCounts(rows: RegistrationDetail[]) {
  return rows.reduce(
    (counts, row) => ({
      received: counts.received + (row.status === "received" ? 1 : 0),
      confirmed: counts.confirmed + (row.status === "confirmed" ? 1 : 0),
      waitlisted: counts.waitlisted + (row.status === "waitlisted" ? 1 : 0),
    }),
    { received: 0, confirmed: 0, waitlisted: 0 },
  );
}

export function addMinutesISO(value: string, minutes: number) {
  return new Date(new Date(value).getTime() + minutes * 60_000).toISOString();
}

export function toDatetimeLocal(value: string) {
  const date = new Date(value);
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

export function fromDatetimeLocal(value: string) {
  return value ? new Date(value).toISOString() : undefined;
}
