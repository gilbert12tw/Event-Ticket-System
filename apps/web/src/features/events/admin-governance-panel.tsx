import type { FormEvent } from "react";
import { navigate } from "@/app/routes";
import { EmptyState, Field, Kpi, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import type { EventSummary } from "@/lib/api";
import { defaultEditEventForm, eventStatusTone } from "@/lib/formatting";

type AdminEditForm = ReturnType<typeof defaultEditEventForm>;
type EventStateForm = {
  status: string;
  reason: string;
};

export function AdminEventGovernancePanel({
  adminEvents,
  busy,
  editForm,
  onArchive,
  onChangeState,
  onDuplicate,
  onEditFormChange,
  onRefresh,
  onSave,
  onSelect,
  onStateFormChange,
  selectedEvent,
  stateForm,
}: {
  adminEvents: EventSummary[];
  busy: boolean;
  editForm: AdminEditForm;
  onArchive: () => void;
  onChangeState: () => void;
  onDuplicate: () => void;
  onEditFormChange: (next: AdminEditForm) => void;
  onRefresh: () => void;
  onSave: (event: FormEvent<HTMLFormElement>) => void | Promise<void>;
  onSelect: (event: EventSummary) => void;
  onStateFormChange: (next: EventStateForm) => void;
  selectedEvent?: EventSummary;
  stateForm: EventStateForm;
}) {
  return (
    <div className="panel span-12">
      <div className="section-heading">
        <div>
          <h2>活動治理</h2>
          <p>
            管理既有活動的可編輯欄位、狀態轉換、複製與封存，資格規則修改仍保留後端邊界。
          </p>
        </div>
        <button
          className="button secondary"
          type="button"
          onClick={onRefresh}
          disabled={busy}
        >
          <Icon name="refresh" />
          重新整理
        </button>
      </div>
      <div className="governance-grid">
        <div className="event-list compact-list">
          {adminEvents.length === 0 && (
            <EmptyState
              title="尚無管理活動"
              action="建立活動或執行 Demo Runbook 後會出現在這裡。"
            />
          )}
          {adminEvents.map((event) => (
            <button
              className={
                selectedEvent?.event_id === event.event_id
                  ? "event-row active"
                  : "event-row"
              }
              type="button"
              key={event.event_id}
              onClick={() => onSelect(event)}
            >
              <span>
                <strong>{event.title}</strong>
                <small>{event.event_id}</small>
              </span>
              <StatusBadge tone={eventStatusTone(event.status)}>
                {event.status}
              </StatusBadge>
            </button>
          ))}
        </div>
        <form
          className="governance-editor"
          onSubmit={(event) => void onSave(event)}
        >
          {!selectedEvent && (
            <EmptyState
              title="尚未選擇活動"
              action="選擇活動後即可編輯 Phase 1 可治理欄位。"
            />
          )}
          {selectedEvent && (
            <>
              <div className="readiness-grid">
                <Kpi label="Confirmed" value={selectedEvent.confirmed_count} />
                <Kpi label="Waitlist" value={selectedEvent.waitlist_count} />
                <Kpi
                  label="剩餘"
                  value={
                    selectedEvent.capacity_type === "unlimited"
                      ? "不限"
                      : (selectedEvent.remaining_capacity ?? 0)
                  }
                />
                <Kpi label="Version" value={selectedEvent.version || 1} />
              </div>
              <fieldset className="form-section full">
                <legend>可編輯欄位</legend>
                <Field
                  label="活動名稱"
                  value={editForm.title}
                  onChange={(value) =>
                    onEditFormChange({ ...editForm, title: value })
                  }
                  required
                />
                <Field
                  label="地點"
                  value={editForm.location}
                  onChange={(value) =>
                    onEditFormChange({ ...editForm, location: value })
                  }
                  required
                />
                <label className="field">
                  <span>活動城市</span>
                  <select
                    value={editForm.event_city}
                    onChange={(event) =>
                      onEditFormChange({
                        ...editForm,
                        event_city: event.target.value,
                      })
                    }
                  >
                    <option value="">（未設定）</option>
                    <option value="Taipei">台北 (Taipei)</option>
                    <option value="Hsinchu">新竹 (Hsinchu)</option>
                    <option value="Taichung">台中 (Taichung)</option>
                    <option value="Tainan">台南 (Tainan)</option>
                    <option value="Kaohsiung">高雄 (Kaohsiung)</option>
                  </select>
                  <small className="form-hint">
                    設定後用於比對員工所在城市，不同城市將顯示跨城市提示（不阻擋報名）。
                  </small>
                </label>
                <Field
                  label="活動開始"
                  type="datetime-local"
                  value={editForm.starts_at}
                  onChange={(value) =>
                    onEditFormChange({ ...editForm, starts_at: value })
                  }
                  required
                />
                <Field
                  label="報名開始"
                  type="datetime-local"
                  value={editForm.registration_start}
                  onChange={(value) =>
                    onEditFormChange({
                      ...editForm,
                      registration_start: value,
                    })
                  }
                  required
                />
                <Field
                  label="報名截止"
                  type="datetime-local"
                  value={editForm.registration_close}
                  onChange={(value) =>
                    onEditFormChange({
                      ...editForm,
                      registration_close: value,
                    })
                  }
                  required
                />
                <label className="field">
                  <span>票數類型</span>
                  <select
                    value={editForm.capacity_type}
                    onChange={(event) => {
                      const next = event.target.value as
                        | "limited"
                        | "unlimited";
                      onEditFormChange({
                        ...editForm,
                        capacity_type: next,
                        capacity:
                          next === "unlimited" ? "" : editForm.capacity || "1",
                      });
                    }}
                  >
                    <option value="limited">limited（本人單張票）</option>
                    <option value="unlimited">
                      unlimited（不扣庫存，可帶家屬）
                    </option>
                    <option value="unlimited">
                      unlimited（不扣庫存，可帶家屬）
                    </option>
                  </select>
                </label>
                {editForm.capacity_type === "limited" && (
                  <Field
                    label="容量"
                    type="number"
                    value={editForm.capacity}
                    onChange={(value) =>
                      onEditFormChange({ ...editForm, capacity: value })
                    }
                    required
                  />
                )}
                <Field
                  label="分類"
                  value={editForm.category}
                  onChange={(value) =>
                    onEditFormChange({ ...editForm, category: value })
                  }
                />
                <Field
                  label="Tags"
                  value={editForm.tags}
                  onChange={(value) =>
                    onEditFormChange({ ...editForm, tags: value })
                  }
                />
                <Field
                  label="入場方式"
                  value={editForm.entry_method}
                  onChange={(value) =>
                    onEditFormChange({ ...editForm, entry_method: value })
                  }
                />
                <Field
                  label="可見性"
                  value={editForm.visibility}
                  onChange={(value) =>
                    onEditFormChange({ ...editForm, visibility: value })
                  }
                />
                <label className="field full">
                  <span>描述</span>
                  <textarea
                    value={editForm.description}
                    onChange={(event) =>
                      onEditFormChange({
                        ...editForm,
                        description: event.target.value,
                      })
                    }
                    rows={3}
                  />
                </label>
              </fieldset>
              <div className="state-tools full">
                <label className="field">
                  <span>狀態</span>
                  <select
                    value={stateForm.status}
                    onChange={(event) =>
                      onStateFormChange({
                        ...stateForm,
                        status: event.target.value,
                      })
                    }
                  >
                    <option value="draft">draft</option>
                    <option value="published">published</option>
                    <option value="closed">closed</option>
                    <option value="cancelled">cancelled</option>
                    <option value="archived">archived</option>
                  </select>
                </label>
                <Field
                  label="狀態原因"
                  value={stateForm.reason}
                  onChange={(value) =>
                    onStateFormChange({ ...stateForm, reason: value })
                  }
                  required
                />
                <button
                  className="button secondary"
                  type="button"
                  onClick={onChangeState}
                  disabled={busy || !stateForm.reason.trim()}
                >
                  <Icon name="save" />
                  更新狀態
                </button>
              </div>
              <div className="form-actions full">
                <button className="button" type="submit" disabled={busy}>
                  <Icon name="save" />
                  儲存活動
                </button>
                <button
                  className="button secondary"
                  type="button"
                  onClick={onDuplicate}
                  disabled={busy}
                >
                  <Icon name="copy" />
                  複製
                </button>
                <button
                  className="button danger"
                  type="button"
                  onClick={onArchive}
                  disabled={busy}
                >
                  <Icon name="trash" />
                  封存
                </button>
                <button
                  className="button ghost"
                  type="button"
                  onClick={() => navigate("/admin/registrations")}
                >
                  <Icon name="users" />
                  報名治理
                </button>
              </div>
            </>
          )}
        </form>
      </div>
    </div>
  );
}
