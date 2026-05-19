import { useState } from "react";
import {
  auditLogs,
  bookEvent,
  checkIn,
  createEvent,
  listEvents,
  listTickets,
  reports,
  seedDemo,
  selectMockProfile,
} from "@/lib/api";
import type { AuthSession, ReportRow, Ticket } from "@/lib/api";
import type { StepState } from "@/app/routes";
import { errorMessage, formatDate, futureISO } from "@/lib/formatting";
import { Kpi } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { TicketPanel } from "@/features/tickets/pages";
import { setDemoCheckinToken } from "@/features/checkin/checkin-token";
import { localizedMessage } from "@/lib/ui/options";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";

const demoSteps = [
  ["seed", "載入起始員工"],
  ["event", "建立已發布活動"],
  ["browse", "員工瀏覽資格"],
  ["book", "第一位合格員工報名"],
  ["waitlist", "第二位合格員工候補"],
  ["reject", "不合格員工被拒絕"],
  ["ticket", "顯示電子票券"],
  ["checkin", "完成首次驗票"],
  ["duplicate", "拒絕重複掃描"],
  ["report", "檢視報表與稽核"],
] as const;

export function DemoRunbookPage({
  session,
  onSessionChange,
}: {
  session: AuthSession;
  onSessionChange: (session: AuthSession | null) => void;
}) {
  const [steps, setSteps] = useState<
    Record<string, { state: StepState; hint: string }>
  >(() => initialSteps());
  const [ticket, setTicket] = useState<Ticket | null>(null);
  const [reportRows, setReportRows] = useState<ReportRow[]>([]);
  const [busy, setBusy] = useState(false);

  function mark(id: string, state: StepState, hint: string) {
    setSteps((current) => ({ ...current, [id]: { state, hint } }));
  }

  async function runAs(principalID: string) {
    return selectMockProfile(principalID);
  }

  async function runDemo() {
    setBusy(true);
    setSteps(initialSteps());
    setTicket(null);
    setReportRows([]);
    const unique = Date.now();
    let currentStep: (typeof demoSteps)[number][0] | null = null;
    const startStep = (id: (typeof demoSteps)[number][0], hint: string) => {
      currentStep = id;
      mark(id, "running", hint);
    };
    try {
      startStep("seed", "執行中");
      await runAs("admin-1");
      await seedDemo();
      mark("seed", "done", "E1001、E1002、E2001 已建立。");

      startStep("event", "建立容量 1 的檢查活動");
      await runAs("admin-1");
      const event = await createEvent({
        title: `台北家庭電影夜 ${unique}`,
        description: "先搶先得、候補與驗票流程檢查。",
        location: "台北總部禮堂",
        starts_at: futureISO(72),
        registration_start: futureISO(-1),
        registration_close: futureISO(48),
        capacity_type: "limited",
        capacity: 1,
        allows_family: false,
        status: "published",
        rule: {
          department: "Engineering",
          site: "Taipei",
          min_grade: 5,
          employment_status: "active",
        },
      });
      mark("event", "done", event.title);

      startStep("browse", "查詢 E1001 活動列表");
      await runAs("E1001");
      const eventRows = await listEvents();
      const current = eventRows.find((row) => row.event_id === event.event_id);
      mark(
        "browse",
        current?.eligible ? "done" : "fail",
        current?.eligibility_reason || "未找到新活動",
      );

      startStep("book", "送出第一筆報名");
      await runAs("E1001");
      const booking = await bookEvent(event.event_id, `book-${unique}-E1001`);
      mark("book", "done", localizedMessage(booking.message));

      startStep("waitlist", "送出第二筆報名");
      await runAs("E1002");
      const waitlist = await bookEvent(event.event_id, `book-${unique}-E1002`);
      mark("waitlist", "done", localizedMessage(waitlist.message));

      startStep("reject", "確認不合格員工被拒絕");
      await runAs("E2001");
      try {
        await bookEvent(event.event_id, `book-${unique}-E2001`);
        mark("reject", "fail", "預期的拒絕沒有發生。");
      } catch (error) {
        mark("reject", "done", errorMessage(error));
      }

      startStep("ticket", "載入 E1001 票券");
      await runAs("E1001");
      const tickets = await listTickets();
      const activeTicket = tickets[0] || booking.ticket;
      if (!activeTicket?.signed_token) throw new Error("ticket was not issued");
      setTicket(activeTicket);
      setDemoCheckinToken(activeTicket.signed_token);
      mark("ticket", "done", `${activeTicket.ticket_id} 已可驗票。`);

      startStep("checkin", "首次核銷");
      await runAs("staff-1");
      const accepted = await checkIn(activeTicket.signed_token, "gate-1");
      mark("checkin", "done", `驗票成功：${formatDate(accepted.scanned_at)}`);

      startStep("duplicate", "重複掃描");
      await runAs("staff-1");
      try {
        await checkIn(activeTicket.signed_token, "gate-1");
        mark("duplicate", "fail", "預期的重複掃描拒絕沒有發生。");
      } catch (error) {
        mark("duplicate", "done", errorMessage(error));
      }

      startStep("report", "載入報表與 audit");
      await runAs("hr-1");
      const [nextReports, nextAudits] = await Promise.all([
        reports(),
        auditLogs(),
      ]);
      setReportRows(nextReports);
      mark(
        "report",
        "done",
        `${nextReports.length} 筆報表，${nextAudits.length} 筆稽核紀錄。`,
      );
    } catch (error) {
      if (currentStep) mark(currentStep, "fail", errorMessage(error));
    } finally {
      try {
        onSessionChange(await selectMockProfile(session.actor.id));
      } catch {
        onSessionChange(null);
      }
      setBusy(false);
    }
  }

  return (
    <section className="content-grid">
      <Card className="panel span-5">
        <div className="section-heading">
          <div>
            <h2>驗證流程</h2>
            <p>一鍵跑完端到端檢查流程，確認主要操作仍可完成。</p>
          </div>
          <Button type="button" onClick={() => void runDemo()} disabled={busy}>
            <Icon name="play" />
            {busy ? "執行中" : "執行流程檢查"}
          </Button>
        </div>
        <div className="step-list">
          {demoSteps.map(([id, label], index) => (
            <div className={`step-item ${steps[id].state}`} key={id}>
              <span className="step-number">{index + 1}</span>
              <span>
                <strong>{label}</strong>
                <small>{steps[id].hint}</small>
              </span>
            </div>
          ))}
        </div>
      </Card>
      <Card className="panel span-7">
        <TicketPanel ticket={ticket || undefined} />
        {reportRows.length > 0 && (
          <div className="report-strip">
            <Kpi label="已報名" value={reportRows[0].confirmed_count} />
            <Kpi label="候補" value={reportRows[0].waitlist_count} />
            <Kpi label="已入場" value={reportRows[0].checkin_count} />
          </div>
        )}
      </Card>
    </section>
  );
}

function initialSteps() {
  return Object.fromEntries(
    demoSteps.map(([id]) => [
      id,
      { state: "pending" as StepState, hint: "待執行" },
    ]),
  );
}
