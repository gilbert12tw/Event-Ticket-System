import type { NotificationDelivery } from "@/lib/api";
import { MetaList } from "@/components/shared";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export function RetryDeliveryDialog({
  busy,
  delivery,
  onClose,
  onConfirm,
}: Readonly<{
  busy: boolean;
  delivery: NotificationDelivery | null;
  onClose: () => void;
  onConfirm: (delivery: NotificationDelivery) => void;
}>) {
  return (
    <Dialog
      open={Boolean(delivery)}
      onOpenChange={(open) => !open && onClose()}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>確認重試通知</DialogTitle>
          <DialogDescription>
            系統會建立下一次投遞嘗試，並保留原本的投遞紀錄與稽核軌跡。
          </DialogDescription>
        </DialogHeader>
        {delivery && (
          <MetaList
            rows={[
              ["投遞編號", delivery.delivery_id],
              ["投遞批次", delivery.outbox_id],
              ["下一次嘗試", `第 ${delivery.attempts + 1} 次`],
            ]}
          />
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
            type="button"
            onClick={() => delivery && onConfirm(delivery)}
            disabled={busy || !delivery}
          >
            {busy ? "重試中" : "確認重試"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
