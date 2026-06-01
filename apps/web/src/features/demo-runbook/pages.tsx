import type { AuthSession } from "@/lib/api";
import { formatDate } from "@/lib/formatting";
import { Alert, Field, Kpi, MetaList, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { TicketPanel } from "@/features/tickets/pages";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { addMinutesISO, demoSteps, fromDatetimeLocal } from "./demo-flow";
import { useDemoRunbook } from "./use-demo-runbook";

export function DemoRunbookPage({
  session,
  demoDebugAvailable,
  onSessionChange,
}: Readonly<{
  session: AuthSession;
  demoDebugAvailable: boolean;
  onSessionChange: (session: AuthSession | null) => void;
}>) {
  const {
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
  } = useDemoRunbook({ session, demoDebugAvailable, onSessionChange });

  return (
    <section className="content-grid">
      <Card className="panel span-5">
        <div className="section-heading">
          <div>
            <h2>Demo 控制台</h2>
            <p>逐步觸發真實 API，右側同步顯示資料狀態。</p>
          </div>
          <StatusBadge tone={demoDebugAvailable ? "ok" : "warn"}>
            {demoDebugAvailable ? "Debug clock" : "Real time"}
          </StatusBadge>
        </div>
        {!demoDebugAvailable && (
          <Alert tone="warn">
            尚未啟用 DEMO_DEBUG_ENABLED；流程仍可操作，但不能快進 cutoff。
          </Alert>
        )}
        <div className="step-list">
          {demoSteps.map(([id, label, request], index) => (
            <div
              className={`step-item demo-step-item ${steps[id].state}`}
              key={id}
            >
              <span className="step-number">{index + 1}</span>
              <span>
                <strong>{label}</strong>
                <small>{request}</small>
                <small>{steps[id].hint}</small>
              </span>
              <Button
                type="button"
                size="sm"
                variant={steps[id].state === "done" ? "outline" : "default"}
                onClick={() => void runDemoStep(id, stepActions[id])}
                disabled={busyStep !== null}
              >
                <Icon name={steps[id].state === "done" ? "refresh" : "play"} />
                {busyStep === id ? "執行中" : "執行"}
              </Button>
            </div>
          ))}
        </div>
      </Card>

      <Card className="panel span-7">
        <div className="section-heading">
          <div>
            <h2>時間控制</h2>
            <p>
              改變 backend business clock，直接影響 cutoff 與 lottery 判斷。
            </p>
          </div>
          <Button
            type="button"
            variant="outline"
            onClick={() => void refreshClock()}
            disabled={!demoDebugAvailable || busyStep !== null}
          >
            <Icon name="refresh" />
            重新讀取
          </Button>
        </div>
        {clock && (
          <MetaList
            rows={[
              ["模式", clock.mode],
              ["Business time", formatDate(clock.now)],
              ["Real write time", formatDate(clock.real_now)],
              ["更新原因", clock.reason || "未設定"],
            ]}
          />
        )}
        {clockMessage && <Alert tone="info">{clockMessage}</Alert>}
        <div className="toolbar">
          <Button
            type="button"
            variant="outline"
            onClick={() => void applyClock("real")}
            disabled={!demoDebugAvailable || busyStep !== null}
          >
            現在
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() =>
              event &&
              void applyClock(
                "fixed",
                addMinutesISO(event.registration_start, 1),
              )
            }
            disabled={!demoDebugAvailable || !event || busyStep !== null}
          >
            報名期間
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() =>
              event &&
              void applyClock(
                "fixed",
                addMinutesISO(event.registration_close, 1),
              )
            }
            disabled={!demoDebugAvailable || !event || busyStep !== null}
          >
            Cutoff +1m
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() =>
              event &&
              void applyClock("fixed", addMinutesISO(event.starts_at, 1))
            }
            disabled={!demoDebugAvailable || !event || busyStep !== null}
          >
            活動開始後
          </Button>
        </div>
        <div className="toolbar">
          <Field
            label="自訂時間"
            type="datetime-local"
            value={customClock}
            onChange={setCustomClock}
          />
          <Button
            type="button"
            onClick={() =>
              void applyClock("fixed", fromDatetimeLocal(customClock))
            }
            disabled={!demoDebugAvailable || !customClock || busyStep !== null}
          >
            套用
          </Button>
        </div>

        <div className="kpi-row">
          <Kpi label="Received" value={counts.received} />
          <Kpi label="Confirmed" value={counts.confirmed} />
          <Kpi label="Waitlisted" value={counts.waitlisted} />
        </div>

        {event && (
          <MetaList
            rows={[
              ["Event ID", event.event_id, "mono-cell"],
              ["活動", event.title],
              ["Allocation", event.allocation_mode],
              ["報名截止", formatDate(event.registration_close)],
              ["活動開始", formatDate(event.starts_at)],
            ]}
          />
        )}

        {lotteryRun && (
          <MetaList
            rows={[
              ["Lottery run", lotteryRun.run_id, "mono-cell"],
              ["Candidates", lotteryRun.candidate_count],
              ["Winners", lotteryRun.winner_count],
              ["Algorithm", lotteryRun.algorithm_version],
            ]}
          />
        )}

        <TicketPanel ticket={ticket || undefined} />

        {currentReport && (
          <div className="report-strip">
            <Kpi label="報表 confirmed" value={currentReport.confirmed_count} />
            <Kpi label="報表 waitlist" value={currentReport.waitlist_count} />
            <Kpi label="已入場" value={currentReport.checkin_count} />
          </div>
        )}

        {auditRows.length > 0 && (
          <div className="event-list">
            {auditRows.slice(0, 3).map((row) => (
              <div className="event-row" key={row.audit_id}>
                <span>
                  <strong>{row.action}</strong>
                  <small>{formatDate(row.created_at)}</small>
                </span>
                <StatusBadge tone="neutral">{row.role}</StatusBadge>
              </div>
            ))}
          </div>
        )}
      </Card>
    </section>
  );
}
