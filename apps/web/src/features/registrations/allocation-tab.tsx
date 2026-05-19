import { useEffect, useState } from "react";
import {
  Alert,
  EmptyState,
  Field,
  Kpi,
  StatusBadge,
} from "@/components/shared";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { runLottery } from "@/lib/api";
import type { EventSummary, LotteryRun } from "@/lib/api";
import { errorMessage } from "@/lib/formatting";

export function AllocationTab({
  busy,
  confirmedCount,
  event,
  onAllocated,
  waitlistCount,
}: {
  busy: boolean;
  confirmedCount: number;
  event?: EventSummary;
  onAllocated: () => void;
  waitlistCount: number;
}) {
  const [seed, setSeed] = useState("");
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [message, setMessage] = useState("");
  const [lotteryRun, setLotteryRun] = useState<LotteryRun | null>(null);
  const [running, setRunning] = useState(false);

  useEffect(() => {
    setSeed("");
    setConfirmOpen(false);
    setMessage("");
    setLotteryRun(null);
  }, [event?.event_id]);

  if (!event) {
    return (
      <EmptyState title="尚未選擇活動" action="先從上方活動選單選擇活動。" />
    );
  }

  if (event.allocation_mode !== "lottery") {
    return (
      <div className="task-panel">
        <div className="section-heading">
          <div>
            <h2>配票治理</h2>
            <p>此活動使用 FCFS，報名成功時即時配置名額，不需要抽籤。</p>
          </div>
          <StatusBadge tone="neutral">FCFS</StatusBadge>
        </div>
        <Alert tone="info">
          Lottery
          不適用於此活動。若要改成抽籤模式，請回到活動設定建立或調整活動。
        </Alert>
      </div>
    );
  }

  async function confirmLotteryRun() {
    if (!event || !seed.trim()) return;
    setRunning(true);
    setMessage("");
    try {
      const result = await runLottery(event.event_id, { seed: seed.trim() });
      setLotteryRun(result);
      setMessage(`抽籤已完成，run id ${result.run_id}。`);
      onAllocated();
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setRunning(false);
      setConfirmOpen(false);
    }
  }

  return (
    <div className="task-panel">
      <div className="section-heading">
        <div>
          <h2>配票抽籤</h2>
          <p>抽籤活動需要 seed 與顯式確認，方便事後 replay 驗證。</p>
        </div>
        <StatusBadge tone="warn">Lottery</StatusBadge>
      </div>
      {message && (
        <Alert tone={message.includes("完成") ? "ok" : "warn"}>{message}</Alert>
      )}
      <div className="kpi-row">
        <Kpi label="已確認" value={confirmedCount} />
        <Kpi label="候補" value={waitlistCount} />
        <Kpi label="剩餘容量" value={event.remaining_capacity ?? "不限量"} />
      </div>
      <Field
        autoComplete="off"
        label="抽籤 seed"
        name="lottery-seed"
        value={seed}
        onChange={setSeed}
        placeholder="例如 2026-summer-event"
        hint="相同名單與 seed 應可重播出一致結果。"
        required
      />
      <div className="toolbar">
        <Button
          type="button"
          onClick={() => setConfirmOpen(true)}
          disabled={busy || running || !seed.trim()}
        >
          執行抽籤
        </Button>
      </div>
      {lotteryRun && (
        <dl className="meta-list vertical">
          <div>
            <dt>Run ID</dt>
            <dd className="mono-cell">{lotteryRun.run_id}</dd>
          </div>
          <div>
            <dt>狀態</dt>
            <dd>{lotteryRun.status}</dd>
          </div>
          <div>
            <dt>中籤人數</dt>
            <dd>{lotteryRun.winner_count}</dd>
          </div>
        </dl>
      )}
      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>確認執行 deterministic lottery</DialogTitle>
            <DialogDescription>
              系統會用目前候補名單與 seed 進行可重播抽籤，結果會寫入治理紀錄。
            </DialogDescription>
          </DialogHeader>
          <Alert tone="warn">
            請確認 seed 已固定且活動名單已整理完成；送出後不要用不同 seed
            反覆嘗試。
          </Alert>
          <DialogFooter>
            <Button
              variant="outline"
              type="button"
              onClick={() => setConfirmOpen(false)}
              disabled={running}
            >
              返回
            </Button>
            <Button
              type="button"
              onClick={() => void confirmLotteryRun()}
              disabled={running || !seed.trim()}
            >
              {running ? "執行中" : "確認抽籤"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
