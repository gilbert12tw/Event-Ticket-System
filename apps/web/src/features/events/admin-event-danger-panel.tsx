import { useState } from "react";
import { navigate } from "@/app/routes";
import { Alert, DangerZonePanel, Field } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import type { EventSummary } from "@/lib/api";
import { SelectEventFirst } from "./admin-event-form-controls";
import { Button } from "@/components/ui/button";

export function AdminEventDangerTab({
  busy,
  selectedEvent,
  onArchive,
  onDuplicate,
}: {
  busy: boolean;
  selectedEvent?: EventSummary;
  onArchive: () => void;
  onDuplicate: () => void;
}) {
  const [archiveConfirmation, setArchiveConfirmation] = useState("");
  if (!selectedEvent) return <SelectEventFirst action="使用危險操作" />;
  const archiveConfirmed = archiveConfirmation.trim() === selectedEvent.title;
  return (
    <DangerZonePanel>
      <div className="section-heading">
        <div>
          <h2>危險操作</h2>
          <p>複製、封存與報名治理入口集中在此，避免和一般儲存混在一起。</p>
        </div>
      </div>
      <Alert tone="warn">
        這些操作會影響活動生命週期或後續治理，送出前請確認目前選取：
        {selectedEvent.title}。
      </Alert>
      <Field
        autoComplete="off"
        id="archive-confirmation"
        label="輸入活動名稱才能封存"
        name="archive-confirmation"
        value={archiveConfirmation}
        onChange={setArchiveConfirmation}
        hint={`需要輸入：${selectedEvent.title}`}
      />
      <p className="form-hint">
        封存會寫入稽核，並影響已報名 {selectedEvent.confirmed_count}、候補{" "}
        {selectedEvent.waitlist_count} 的後續治理。
        {!archiveConfirmed && " 按鈕會在活動名稱完全相符後開啟。"}
      </p>
      <div className="danger-action-list">
        <Button
          variant="outline"
          type="button"
          onClick={onDuplicate}
          disabled={busy}
        >
          <Icon name="copy" />
          複製活動
        </Button>
        <Button
          variant="destructive"
          type="button"
          onClick={onArchive}
          disabled={busy || !archiveConfirmed}
        >
          <Icon name="trash" />
          封存活動
        </Button>
        <Button
          variant="ghost"
          type="button"
          onClick={() => navigate("/admin/registrations")}
        >
          <Icon name="users" />
          前往報名治理
        </Button>
      </div>
    </DangerZonePanel>
  );
}
