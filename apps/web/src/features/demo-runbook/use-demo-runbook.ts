import { useEffect, useMemo, useState } from "react";
import {
  auditLogs,
  bookEvent,
  captureProviderToken,
  checkIn,
  createEvent,
  getDemoClock,
  listRegistrations,
  listTickets,
  reports,
  restoreProviderToken,
  runLottery,
  seedDemo,
  selectMockProfile,
  updateDemoClock,
} from "@/lib/api";
import type {
  AuditLog,
  AuthSession,
  DemoClockSnapshot,
  EventSummary,
  LotteryRun,
  RegistrationDetail,
  ReportRow,
  Ticket,
} from "@/lib/api";
import type { StepState } from "@/app/routes";
import { errorMessage, formatDate, futureISO } from "@/lib/formatting";
import { setDemoCheckinToken } from "@/features/checkin/checkin-token";
import {
  addMinutesISO,
  initialSteps,
  registrationCounts,
  toDatetimeLocal,
  type DemoStepID,
  type StepMap,
} from "./demo-flow";

type DemoBusyState = DemoStepID | "clock" | null;

type DemoRunbookParams = Readonly<{
  session: AuthSession;
  demoDebugAvailable: boolean;
  onSessionChange: (session: AuthSession | null) => void;
}>;

export function useDemoRunbook({
  session,
  demoDebugAvailable,
  onSessionChange,
}: DemoRunbookParams) {
  const [steps, setSteps] = useState<StepMap>(() => initialSteps());
  const [busyStep, setBusyStep] = useState<DemoBusyState>(null);
  const [clock, setClock] = useState<DemoClockSnapshot | null>(null);
  const [clockMessage, setClockMessage] = useState("");
  const [customClock, setCustomClock] = useState("");
  const [event, setEvent] = useState<EventSummary | null>(null);
  const [registrations, setRegistrations] = useState<RegistrationDetail[]>([]);
  const [lotteryRun, setLotteryRun] = useState<LotteryRun | null>(null);
  const [ticket, setTicket] = useState<Ticket | null>(null);
  const [reportRows, setReportRows] = useState<ReportRow[]>([]);
  const [auditRows, setAuditRows] = useState<AuditLog[]>([]);
  const [unique] = useState(() => Date.now());

  const counts = useMemo(
    () => registrationCounts(registrations),
    [registrations],
  );
  const currentReport = reportRows.find(
    (row) => row.event_id === event?.event_id,
  );

  useEffect(() => {
    if (!demoDebugAvailable) return;
    void refreshClock();
  }, [demoDebugAvailable]);

  function mark(id: DemoStepID, state: StepState, hint: string) {
    setSteps((current) => ({ ...current, [id]: { state, hint } }));
  }

  async function refreshClock() {
    const snapshot = await getDemoClock();
    setClock(snapshot);
    setClockMessage("");
    return snapshot;
  }

  async function withProfile<T>(profileID: string, action: () => Promise<T>) {
    const providerToken = captureProviderToken();
    try {
      await selectMockProfile(profileID);
      return await action();
    } finally {
      restoreProviderToken(providerToken);
      onSessionChange(session);
    }
  }

  async function runDemoStep(id: DemoStepID, action: () => Promise<string>) {
    setBusyStep(id);
    mark(id, "running", "執行中");
    try {
      const hint = await action();
      mark(id, "done", hint);
    } catch (error) {
      mark(id, "fail", errorMessage(error));
    } finally {
      setBusyStep(null);
    }
  }

  async function refreshRegistrations(eventID = event?.event_id) {
    if (!eventID) return [];
    const rows = await withProfile("admin-1", () => listRegistrations(eventID));
    setRegistrations(rows);
    return rows;
  }

  async function createLotteryEvent() {
    return withProfile("admin-1", async () => {
      const created = await createEvent({
        title: `Demo 抽籤活動 ${unique}`,
        description: "用於展示報名收到、快進 cutoff、抽籤配票與驗票。",
        location: "台北總部禮堂",
        event_city: "Taipei",
        event_site: "Taipei HQ",
        starts_at: futureISO(4),
        ends_at: futureISO(6),
        registration_start: futureISO(-1),
        registration_close: futureISO(1),
        capacity_type: "limited",
        capacity: 1,
        allows_family: false,
        allocation_mode: "lottery",
        status: "published",
        rule: {
          department: "Engineering",
          site: "Taipei HQ",
          min_grade: 5,
          employment_status: "active",
        },
      });
      setEvent(created);
      setRegistrations([]);
      setLotteryRun(null);
      setTicket(null);
      setReportRows([]);
      setAuditRows([]);
      setCustomClock(toDatetimeLocal(created.registration_close));
      return `created ${created.event_id}; allocation=${created.allocation_mode}`;
    });
  }

  async function book(profileID: "E1001" | "E1002") {
    if (!event) throw new Error("請先建立 lottery 活動");
    const booking = await withProfile(profileID, () =>
      bookEvent(event.event_id, `demo-${unique}-${profileID}`),
    );
    await refreshRegistrations(event.event_id);
    return `${profileID}: status=${booking.registration.status}; ticket=${booking.ticket?.ticket_id ?? "none"}`;
  }

  async function advanceToCutoff() {
    if (!demoDebugAvailable) throw new Error("DEMO_DEBUG_ENABLED 尚未開啟");
    if (!event) throw new Error("請先建立 lottery 活動");
    const now = addMinutesISO(event.registration_close, 1);
    const snapshot = await withProfile("admin-1", () =>
      updateDemoClock({
        mode: "fixed",
        now,
        reason: `demo cutoff for ${event.event_id}`,
      }),
    );
    setClock(snapshot);
    setClockMessage(`business time = ${formatDate(snapshot.now)}`);
    return `fixed now=${snapshot.now}`;
  }

  async function runAllocation() {
    if (!event) throw new Error("請先建立 lottery 活動");
    const result = await withProfile("admin-1", () =>
      runLottery(event.event_id, { seed: `demo-${unique}` }),
    );
    setLotteryRun(result);
    await refreshRegistrations(event.event_id);
    return `candidates=${result.candidate_count}; winners=${result.winner_count}`;
  }

  async function loadWinnerTicket() {
    if (!event) throw new Error("請先建立 lottery 活動");
    const winner = registrations.find((row) => row.status === "confirmed");
    if (!winner) throw new Error("尚未找到 confirmed winner");
    const tickets = await withProfile(winner.employee_id, () => listTickets());
    const activeTicket = tickets.find((row) => row.event_id === event.event_id);
    if (!activeTicket?.signed_token) throw new Error("winner ticket 尚未核發");
    setTicket(activeTicket);
    setDemoCheckinToken(activeTicket.signed_token);
    return `${winner.employee_id}: ticket=${activeTicket.ticket_id}`;
  }

  async function checkInWinner() {
    if (!event || !ticket?.signed_token)
      throw new Error("請先載入 winner 票券");
    const accepted = await withProfile("staff-1", () =>
      checkIn(ticket.signed_token || "", "demo-gate-1", event.event_id),
    );
    return `accepted at ${formatDate(accepted.scanned_at)}`;
  }

  async function rejectDuplicateScan() {
    if (!event || !ticket?.signed_token) throw new Error("請先完成首次驗票");
    return withProfile("staff-1", async () => {
      try {
        await checkIn(ticket.signed_token || "", "demo-gate-1", event.event_id);
      } catch (error) {
        return errorMessage(error);
      }
      throw new Error("預期的 duplicate rejection 沒有發生");
    });
  }

  async function loadReportsAndAudit() {
    if (!event) throw new Error("請先建立 lottery 活動");
    const [nextReports, nextAudits] = await withProfile("hr-1", () =>
      Promise.all([
        reports(),
        auditLogs({ entity_id: event.event_id, limit: "10" }),
      ]),
    );
    setReportRows(nextReports);
    setAuditRows(nextAudits);
    const row = nextReports.find(
      (candidate) => candidate.event_id === event.event_id,
    );
    return `reports=${nextReports.length}; event_confirmed=${row?.confirmed_count ?? 0}; audits=${nextAudits.length}`;
  }

  async function applyClock(mode: "real" | "fixed", now?: string) {
    if (!demoDebugAvailable) {
      setClockMessage("DEMO_DEBUG_ENABLED 尚未開啟");
      return;
    }
    setBusyStep("clock");
    try {
      const snapshot = await withProfile("admin-1", () =>
        updateDemoClock({
          mode,
          now,
          reason: mode === "real" ? "demo reset" : "manual demo time",
        }),
      );
      setClock(snapshot);
      setClockMessage(`business time = ${formatDate(snapshot.now)}`);
    } catch (error) {
      setClockMessage(errorMessage(error));
    } finally {
      setBusyStep(null);
    }
  }

  const stepActions: Record<DemoStepID, () => Promise<string>> = {
    seed: async () => {
      await withProfile("admin-1", () => seedDemo());
      return "seeded E1001, E1002, admin-1, staff-1, hr-1";
    },
    event: createLotteryEvent,
    book1: () => book("E1001"),
    book2: () => book("E1002"),
    received: async () => {
      const rows = await refreshRegistrations();
      return `received=${rows.filter((row) => row.status === "received").length}; total=${rows.length}`;
    },
    cutoff: advanceToCutoff,
    lottery: runAllocation,
    ticket: loadWinnerTicket,
    checkin: checkInWinner,
    duplicate: rejectDuplicateScan,
    report: loadReportsAndAudit,
  };

  return {
    auditRows,
    busyStep,
    clock,
    clockMessage,
    counts,
    currentReport,
    customClock,
    event,
    lotteryRun,
    refreshClock,
    runDemoStep,
    setCustomClock,
    stepActions,
    steps,
    ticket,
    applyClock,
  };
}
