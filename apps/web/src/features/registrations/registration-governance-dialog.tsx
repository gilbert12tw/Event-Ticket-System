import type { RegistrationDetail, Ticket } from "@/lib/api";
import {
  Alert,
  MetaList,
  SelectField,
  type MetaListRow,
} from "@/components/shared";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import {
  cancellationReasonOptions,
  revocationReasonOptions,
} from "@/lib/ui/options";

export type GovernanceAction =
  | { kind: "cancel-registration" | "cancel-waitlist"; row: RegistrationDetail }
  | { kind: "revoke-ticket"; row: RegistrationDetail; ticket: Ticket };

const governanceActionCopies = {
  "revoke-ticket": {
    title: "確認撤銷票券",
    buttonLabel: "確認撤銷",
    consequence:
      "票券撤銷後不可入場，驗票端會改為拒絕，員工需要由主辦重新處理。",
  },
  "cancel-waitlist": {
    title: "確認取消候補",
    buttonLabel: "確認取消候補",
    consequence: "候補取消後會離開候補名單，不會再自動遞補名額。",
  },
  "cancel-registration": {
    title: "確認取消報名",
    buttonLabel: "確認取消報名",
    consequence: "報名取消後會釋出名額；若已有票券，票券治理需同步確認。",
  },
};

export function GovernanceConfirmationDialog({
  action,
  busy,
  onChangeReason,
  onClose,
  onConfirm,
  reason,
}: Readonly<{
  action: GovernanceAction | null;
  busy: boolean;
  onChangeReason: (reason: string) => void;
  onClose: () => void;
  onConfirm: () => void;
  reason: string;
}>) {
  const copy = action ? governanceActionCopies[action.kind] : null;
  const options =
    action?.kind === "revoke-ticket"
      ? revocationReasonOptions
      : cancellationReasonOptions;
  const details: MetaListRow[] = action
    ? [
        [
          "員工",
          <>
            {action.row.employee_name || action.row.employee_id}
            <span className="table-muted">{action.row.employee_id}</span>
          </>,
        ],
        ["報名編號", action.row.registration_id],
        ...(action.kind === "revoke-ticket"
          ? ([["票券編號", action.ticket.ticket_id]] satisfies MetaListRow[])
          : []),
      ]
    : [];

  return (
    <Dialog open={Boolean(action)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{copy?.title || "確認操作"}</DialogTitle>
          <DialogDescription>
            送出後會立即影響報名或票券狀態，並寫入稽核紀錄。
          </DialogDescription>
        </DialogHeader>
        {action && copy && (
          <>
            <MetaList rows={details} />
            <Alert tone="warn">{copy.consequence}</Alert>
            <SelectField
              label="處置原因"
              value={reason}
              options={[{ value: "", label: "請選擇原因" }, ...options]}
              onChange={onChangeReason}
              required
              invalid={!reason.trim()}
              hint="必須選擇明確原因，不能用預設原因直接送出。"
            />
          </>
        )}
        <DialogFooter>
          <Button
            variant="outline"
            type="button"
            onClick={onClose}
            disabled={busy}
          >
            返回
          </Button>
          <Button
            variant="destructive"
            type="button"
            onClick={onConfirm}
            disabled={busy || !reason.trim()}
          >
            {busy ? "處理中" : copy?.buttonLabel || "確認"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
