import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { MetaList, SelectField, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import type { EventSummary } from "@/lib/api";
import {
  cancellationReasonOptions,
  registrationStatusView,
} from "@/lib/ui/options";

export function CancellationControl({
  busy = false,
  event,
  onCancel,
  onReasonChange,
  reason,
}: {
  busy?: boolean;
  event: EventSummary;
  onCancel: () => void;
  onReasonChange: (value: string) => void;
  reason: string;
}) {
  const registrationID = registrationIDFor(event);
  const booked =
    event.current_user_status === "confirmed" ||
    event.current_user_status === "waitlisted";
  if (!booked) return <p className="form-hint">沒有可取消的有效報名。</p>;
  const open = cancellationOpen(event);
  const reasonValue = reason || "employee cancellation";
  return (
    <div className="cancel-box">
      <SelectField
        label="取消原因"
        value={reasonValue}
        options={cancellationReasonOptions}
        onChange={onReasonChange}
      />
      <AlertDialog>
        <AlertDialogTrigger asChild>
          <Button
            variant="outline"
            aria-label="取消報名"
            type="button"
            disabled={busy || !open || !registrationID}
          >
            <Icon name="x" />
            {busy ? "取消中" : "取消報名"}
          </Button>
        </AlertDialogTrigger>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>確認取消報名</AlertDialogTitle>
            <AlertDialogDescription>
              取消後會立即更新報名狀態，並寫入活動報名紀錄。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <MetaList
            className="vertical cancellation-confirmation-list"
            rows={[
              ["活動", event.title],
              [
                "目前狀態",
                <StatusBadge
                  tone={registrationStatusView(event.current_user_status).tone}
                >
                  {registrationStatusView(event.current_user_status).label}
                </StatusBadge>,
              ],
              ["取消原因", cancelReasonLabel(reasonValue)],
              ["票券影響", cancellationTicketConsequence(event)],
              ["名額與候補", cancellationCapacityConsequence(event)],
              [
                "重新報名",
                "取消後不能自行用前端重新報名；若需恢復或重新報名，請聯絡活動主辦。",
              ],
            ]}
          />
          <AlertDialogFooter>
            <AlertDialogCancel>保留報名</AlertDialogCancel>
            <AlertDialogAction type="button" disabled={busy} onClick={onCancel}>
              {busy ? "取消中" : "確認取消報名"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <p className="form-hint">
        {open
          ? "報名期間內可自助取消，送出前會再次確認影響。"
          : "自助取消已關閉，請聯絡活動主辦。"}
      </p>
    </div>
  );
}

function registrationIDFor(event: EventSummary) {
  return (
    event.current_user_registration_id ||
    event.current_user_ticket?.registration_id ||
    ""
  );
}

function cancellationOpen(event: EventSummary) {
  const close = Date.parse(event.registration_close);
  return Number.isFinite(close) && close > Date.now();
}

function cancelReasonLabel(reason: string) {
  return (
    cancellationReasonOptions.find((option) => option.value === reason)
      ?.label || reason
  );
}

function cancellationTicketConsequence(event: EventSummary) {
  if (event.current_user_ticket) {
    return "已核發票券會同步失效，不能再用於入場。";
  }
  return "目前沒有已核發票券。";
}

function cancellationCapacityConsequence(event: EventSummary) {
  if (event.current_user_status === "waitlisted") {
    return "候補紀錄會移除，候補順序可能更新。";
  }
  return "名額會釋出，候補或剩餘名額可能更新。";
}
