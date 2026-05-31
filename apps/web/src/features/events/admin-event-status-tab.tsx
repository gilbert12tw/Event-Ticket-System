import { useState } from "react";
import {
  CompactStatsBar,
  Field,
  MetaList,
  SelectField,
  StatusBadge,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { EventSummary } from "@/lib/api";
import { eventStatusOptions, eventStatusView } from "@/lib/ui/options";
import { SelectEventFirst } from "./admin-event-form-controls";
import type { EventStateForm } from "./admin-event-crud-types";

export function AdminEventStatusTab({
  busy,
  selectedEvent,
  stateForm,
  onChangeState,
  onStateFormChange,
}: Readonly<{
  busy: boolean;
  selectedEvent?: EventSummary;
  stateForm: EventStateForm;
  onChangeState: () => void;
  onStateFormChange: (next: EventStateForm) => void;
}>) {
  const [confirming, setConfirming] = useState(false);
  if (!selectedEvent) return <SelectEventFirst action="變更狀態" />;
  const statusUnchanged = stateForm.status === selectedEvent.status;
  const reasonReady = stateForm.reason.trim().length > 0;
  return (
    <div className="task-panel event-status-workspace">
      <div className="section-heading">
        <div>
          <h2>發布狀態</h2>
          <p>發布、關閉、取消與封存前的狀態意圖分開處理，並寫入稽核。</p>
        </div>
      </div>
      <CompactStatsBar
        items={[
          { label: "已報名", value: selectedEvent.confirmed_count },
          { label: "候補", value: selectedEvent.waitlist_count },
          { label: "剩餘", value: remainingCapacityLabel(selectedEvent) },
          { label: "版本", value: selectedEvent.version || 1 },
        ]}
        label="發布狀態摘要"
      />
      <div className="state-tools event-status-tools">
        <SelectField
          label="狀態"
          value={stateForm.status}
          options={eventStatusOptions}
          onChange={(status) => onStateFormChange({ ...stateForm, status })}
        />
        <Field
          label="狀態原因"
          name="event-state-reason"
          value={stateForm.reason}
          onChange={(reason) => onStateFormChange({ ...stateForm, reason })}
          required
        />
        <Button
          type="button"
          onClick={() => setConfirming(true)}
          disabled={busy || statusUnchanged || !reasonReady}
        >
          <Icon name="save" />
          更新狀態
        </Button>
      </div>
      <div className="helper-strip event-status-audit-strip">
        <StatusBadge tone="warn">將寫入稽核</StatusBadge>
        <span>
          {selectedEvent.title} · {eventStatusView(selectedEvent.status).label}{" "}
          → {eventStatusView(stateForm.status).label} · 影響已報名{" "}
          {selectedEvent.confirmed_count}、候補 {selectedEvent.waitlist_count}
        </span>
      </div>
      <Dialog open={confirming} onOpenChange={setConfirming}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>確認更新活動狀態</DialogTitle>
            <DialogDescription>
              狀態變更會影響員工能否報名、候補與入場，並寫入稽核紀錄。
            </DialogDescription>
          </DialogHeader>
          <MetaList
            rows={[
              ["活動", selectedEvent.title],
              [
                "狀態變更",
                <>
                  {eventStatusView(selectedEvent.status).label} →{" "}
                  {eventStatusView(stateForm.status).label}
                </>,
              ],
              [
                "影響名單",
                <>
                  已報名 {selectedEvent.confirmed_count}、候補{" "}
                  {selectedEvent.waitlist_count}
                </>,
              ],
              ["原因", stateForm.reason],
            ]}
          />
          <DialogFooter>
            <Button
              variant="outline"
              type="button"
              onClick={() => setConfirming(false)}
              disabled={busy}
            >
              返回
            </Button>
            <Button
              type="button"
              onClick={() => {
                setConfirming(false);
                onChangeState();
              }}
              disabled={busy || statusUnchanged || !reasonReady}
            >
              確認更新
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function remainingCapacityLabel(event: EventSummary) {
  if (event.capacity_type === "unlimited") return "不限";
  return event.remaining_capacity ?? 0;
}
